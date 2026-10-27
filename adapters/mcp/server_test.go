package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	transport "ns-exploration-graph/adapters/mcp"
	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/store/memory"
	"ns-exploration-graph/toolkit"
)

func fixture(t *testing.T, role string, authorize toolkit.Authorizer) (*graph.Service, *toolkit.Adapter) {
	t.Helper()
	ctx := context.Background()
	s := graph.NewService(memory.New())
	b := toolkit.Binding{Namespace: "fixture", GraphID: graph.NewID(), ActorID: "host-test", Role: role}
	_, e := s.CreateGraph(ctx, graph.CommandContext{Namespace: b.Namespace, GraphID: b.GraphID, ActorID: b.ActorID, RequestID: graph.NewID()}, graph.CreateGraphInput{Title: "Parser", Request: "Investigate rejection"})
	if e != nil {
		t.Fatal(e)
	}
	a, e := toolkit.New(s, toolkit.Config{Binding: b, Authorize: authorize})
	if e != nil {
		t.Fatal(e)
	}
	return s, a
}
func connect(t *testing.T, a *toolkit.Adapter, o transport.Options) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	s, e := transport.NewServer(ctx, a, o)
	if e != nil {
		t.Fatal(e)
	}
	ct, st := sdk.NewInMemoryTransports()
	serverSession, e := s.Connect(ctx, st, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { serverSession.Close() })
	client := sdk.NewClient(&sdk.Implementation{Name: "conformance-client", Version: "1"}, nil)
	session, e := client.Connect(ctx, ct, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
func invoke(t *testing.T, s *sdk.ClientSession, name string, args any) toolkit.Envelope {
	t.Helper()
	r, e := s.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(r.StructuredContent)
	if e != nil {
		t.Fatal(e)
	}
	var out toolkit.Envelope
	if e = json.Unmarshal(b, &out); e != nil {
		t.Fatal(e)
	}
	if r.IsError == out.OK {
		t.Fatalf("MCP tool-error flag disagrees with envelope: %+v", r)
	}
	if len(r.Content) != 1 {
		t.Fatal("missing JSON text result")
	}
	if text, ok := r.Content[0].(*sdk.TextContent); !ok || !jsonEqual(b, []byte(text.Text)) {
		t.Fatal("structured/text parity failure")
	}
	if e = toolkit.ValidateOutput(name, b); e != nil {
		t.Fatal(e)
	}
	return out
}
func jsonEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return bytes.Equal(xb, yb)
}
func TestProtocolDiscoveryValidationAndParity(t *testing.T) {
	_, a := fixture(t, "planner", nil)
	const invocation = "durably-persisted-host-invocation"
	s := connect(t, a, transport.Options{ResolveInvocation: func(context.Context, *sdk.CallToolRequest) (string, error) { return invocation, nil }})
	ds, e := s.ListTools(context.Background(), &sdk.ListToolsParams{})
	if e != nil {
		t.Fatal(e)
	}
	if len(ds.Tools) != 6 {
		t.Fatalf("planner discovery tools %d", len(ds.Tools))
	}
	for _, d := range ds.Tools {
		if d.OutputSchema == nil {
			t.Fatal("missing result schema")
		}
		if d.Name == "graph_accept_goal" || d.Name == "graph_record_observation" {
			t.Fatal("planner gained worker/acceptance grant")
		}
	}
	args := map[string]any{"title": "Reproduce parser", "instruction": "Run malformed fixture", "priority": 5}
	encoded, _ := json.Marshal(args)
	direct := a.Invoke(context.Background(), "graph_propose_work", encoded, invocation)
	if !direct.OK {
		t.Fatal(direct.Error)
	}
	remote := invoke(t, s, "graph_propose_work", args)
	db, _ := json.Marshal(direct)
	rb, _ := json.Marshal(remote)
	if !jsonEqual(db, rb) {
		t.Fatalf("direct/MCP normalized envelope mismatch\n%s\n%s", db, rb)
	}
	for _, bad := range []map[string]any{{"title": "t", "instruction": "i", "priority": 0, "graph_id": graph.NewID()}, {"title": "t", "instruction": "i", "priority": 0, "parents": []any{map[string]any{"id": graph.NewID(), "revision": 1, "actor_id": "spoof"}}}} {
		out := invoke(t, s, "graph_propose_work", bad)
		if out.Error == nil || out.Error.Code != "invalid_argument" {
			t.Fatalf("spoof accepted %+v", out)
		}
	}
	no := invoke(t, s, "graph_record_observation", map[string]any{})
	if no.Error == nil || no.Error.Code != "permission_denied" {
		t.Fatal("hidden tool direct invocation was not denied")
	}
	read := invoke(t, s, "graph_list_nodes", map[string]any{})
	if !read.OK {
		t.Fatal(read.Error)
	}
}
func TestReconnectIdentityAndGenericAmbiguity(t *testing.T) {
	s, a := fixture(t, "planner", nil)
	o := transport.Options{ResolveInvocation: func(context.Context, *sdk.CallToolRequest) (string, error) { return "host-ledger-operation-1", nil }}
	args := map[string]any{"title": "Retry fixture", "instruction": "Investigate", "priority": 1}
	one := connect(t, a, o)
	first := invoke(t, one, "graph_propose_work", args)
	one.Close()
	two := connect(t, a, o)
	retry := invoke(t, two, "graph_propose_work", args)
	fb, _ := json.Marshal(first)
	rb, _ := json.Marshal(retry)
	if !jsonEqual(fb, rb) {
		t.Fatal("trusted durable identity did not survive reconnect")
	}
	b := a.Binding()
	events, e := s.EventsAfter(context.Background(), b.Namespace, b.GraphID, 0, 100)
	if e != nil {
		t.Fatal(e)
	}
	if len(events.Events) != 2 {
		t.Fatal("retry created duplicate mutation")
	}
	generic := connect(t, a, transport.Options{})
	g1 := invoke(t, generic, "graph_propose_work", args)
	generic.Close()
	g2 := invoke(t, connect(t, a, transport.Options{}), "graph_propose_work", args)
	if g1.GraphRevision == g2.GraphRevision {
		t.Fatal("generic server falsely treated reconnect as durable retry")
	}
}

// unavailable fails one mutation and one read after the real service is set up.
type unavailable struct {
	*graph.Service
	err error
}

func (u unavailable) ProposeWork(context.Context, graph.CommandContext, graph.ProposeWorkInput) (graph.Result, error) {
	return graph.Result{}, u.err
}
func (u unavailable) ListNodes(context.Context, string, string, graph.ListOptions) (graph.NodePage, error) {
	return graph.NodePage{}, u.err
}
func TestGeneratedIdentityStorageFailureIsAmbiguous(t *testing.T) {
	s, real := fixture(t, "planner", nil)
	stand := unavailable{s, graph.Err("storage_unavailable", "pq: connection reset")}
	a, e := toolkit.New(stand, toolkit.Config{Binding: real.Binding()})
	if e != nil {
		t.Fatal(e)
	}
	args := map[string]any{"title": "t", "instruction": "i", "priority": 1}
	out := invoke(t, connect(t, a, transport.Options{}), "graph_propose_work", args)
	if out.Error == nil || out.Error.Code != "storage_unavailable" || out.Error.Retryable || out.Error.Details["outcome"] != "ambiguous" {
		t.Fatalf("generated identity failure %+v", out.Error)
	}
	stable := transport.Options{ResolveInvocation: func(context.Context, *sdk.CallToolRequest) (string, error) { return "host-ledger-operation-2", nil }}
	out = invoke(t, connect(t, a, stable), "graph_propose_work", args)
	if out.Error == nil || !out.Error.Retryable || out.Error.Details != nil {
		t.Fatalf("resolved identity lost retryability %+v", out.Error)
	}
	out = invoke(t, connect(t, a, transport.Options{}), "graph_list_nodes", map[string]any{})
	if out.Error == nil || out.Error.Code != "storage_unavailable" || !out.Error.Retryable || out.Error.Details != nil {
		t.Fatalf("read failure changed %+v", out.Error)
	}
	// Coded errors pass through by pointer; the stored error must stay intact.
	coded := &graph.Error{Code: "revision_conflict", Message: "Node revision changed; read it again.", Retryable: true, Details: map[string]any{"current_revision": 3}}
	stand.err = coded
	a, e = toolkit.New(stand, toolkit.Config{Binding: real.Binding()})
	if e != nil {
		t.Fatal(e)
	}
	out = invoke(t, connect(t, a, transport.Options{}), "graph_propose_work", args)
	if out.Error.Retryable || out.Error.Details["outcome"] != "ambiguous" || out.Error.Details["current_revision"] != float64(3) {
		t.Fatalf("details dropped %+v", out.Error)
	}
	if !coded.Retryable || len(coded.Details) != 1 {
		t.Fatalf("stored error mutated %+v", coded)
	}
}
func TestWorkerDiscoveryAndLiveAuthorization(t *testing.T) {
	allowed := true
	_, a := fixture(t, "worker", func(context.Context, toolkit.Binding, string) error {
		if !allowed {
			return graph.Err("permission_denied", "revoked")
		}
		return nil
	})
	s := connect(t, a, transport.Options{})
	ds, e := s.ListTools(context.Background(), &sdk.ListToolsParams{})
	if e != nil || len(ds.Tools) != 5 {
		t.Fatalf("worker discovery %v %v", ds, e)
	}
	out := invoke(t, s, "graph_propose_work", map[string]any{})
	if out.Error == nil || out.Error.Code != "permission_denied" {
		t.Fatal("worker direct planner invocation")
	}
	allowed = false
	out = invoke(t, s, "graph_overview", map[string]any{})
	if out.Error == nil || out.Error.Code != "permission_denied" {
		t.Fatal("authorization did not run on read")
	}
}
