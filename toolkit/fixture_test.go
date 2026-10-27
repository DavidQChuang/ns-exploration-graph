package toolkit_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/store/memory"
	"ns-exploration-graph/graph/store/postgres"
	"ns-exploration-graph/toolkit"
)

// eachStore runs fn against the in-memory reference store and real PostgreSQL.
// PostgreSQL is mandatory under CI/GRAPH_REQUIRE_POSTGRES and skipped only on a bare local run.
func eachStore(t *testing.T, fn func(*testing.T, *graph.Service)) {
	t.Run("memory", func(t *testing.T) { fn(t, graph.NewService(memory.New())) })
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("GRAPH_TEST_DATABASE_URL")
		if dsn == "" {
			if os.Getenv("CI") != "" || os.Getenv("GRAPH_REQUIRE_POSTGRES") == "1" {
				t.Fatal("GRAPH_TEST_DATABASE_URL is required; PostgreSQL toolkit tests cannot be skipped in CI")
			}
			t.Skip("set GRAPH_TEST_DATABASE_URL to run toolkit tests against PostgreSQL")
		}
		db, e := postgres.Open(context.Background(), dsn)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { db.Close() })
		if e = db.Migrate(context.Background()); e != nil {
			t.Fatal(e)
		}
		fn(t, graph.NewService(db))
	})
}

func memoryStore() graph.Store { return memory.New() }

// world is a fresh graph (origin plus one goal) with host-side helpers. The host
// calls the service directly; agents only use adapters.
type world struct {
	t    *testing.T
	ctx  context.Context
	svc  *graph.Service
	b    toolkit.Binding
	goal graph.Ref
}

func newWorld(t *testing.T, svc *graph.Service) *world {
	t.Helper()
	w := &world{t: t, ctx: context.Background(), svc: svc, b: toolkit.Binding{Namespace: "toolkit-tests", GraphID: graph.NewID(), ActorID: "host-tester", Role: "planner"}}
	_, e := svc.CreateGraph(w.ctx, w.host(), graph.CreateGraphInput{Title: "Parser investigation", Request: "Verify the integer parser rejects malformed input."})
	w.must(e)
	g, e := svc.AddGoal(w.ctx, w.host(), graph.AddGoalInput{Title: "Malformed input rejected", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Rejection evidence retained."}}})
	w.must(e)
	w.goal = graph.Ref{NodeID: g.Data["goal_id"].(string), Revision: 1}
	return w
}
func (w *world) must(e error) {
	w.t.Helper()
	if e != nil {
		w.t.Fatal(e)
	}
}
func (w *world) host() graph.CommandContext {
	return graph.CommandContext{Namespace: w.b.Namespace, GraphID: w.b.GraphID, ActorID: w.b.ActorID, RequestID: graph.NewID()}
}

// adapter builds a tool adapter from the world's binding with optional edits.
func (w *world) adapter(edit func(*toolkit.Binding), cfg ...func(*toolkit.Config)) *toolkit.Adapter {
	w.t.Helper()
	b := w.b
	if edit != nil {
		edit(&b)
	}
	c := toolkit.Config{Binding: b}
	for _, f := range cfg {
		f(&c)
	}
	a, e := toolkit.New(w.svc, c)
	w.must(e)
	return a
}

// work proposes a work item through the host API and returns its ID.
func (w *world) work(title string) string {
	return w.workWith(title, "Run the fixture and record the result.")
}
func (w *world) workWith(title, instruction string) string {
	r, e := w.svc.ProposeWork(w.ctx, w.host(), graph.ProposeWorkInput{Title: title, WorkPayload: graph.WorkPayload{Instruction: instruction, Priority: 10}})
	w.must(e)
	return r.Data["work_item_id"].(string)
}

// claim moves open work (at revision rev) to running under a fresh execution.
func (w *world) claim(workID string, rev int) *graph.Execution {
	x := graph.NewID()
	_, e := w.svc.TransitionWork(w.ctx, w.host(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: workID, Revision: rev}, State: "running", ExecutionID: x})
	w.must(e)
	return &graph.Execution{WorkItemID: workID, ExecutionID: x}
}
func (w *world) transition(workID string, rev int, state string) {
	_, e := w.svc.TransitionWork(w.ctx, w.host(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: workID, Revision: rev}, State: state})
	w.must(e)
}
func (w *world) worker(x *graph.Execution) *toolkit.Adapter {
	return w.adapter(func(b *toolkit.Binding) { b.Role, b.Execution = "worker", x })
}
func (w *world) revision() int64 {
	s, e := w.svc.Snapshot(w.ctx, w.b.Namespace, w.b.GraphID)
	w.must(e)
	return s.Graph.Revision
}
func (w *world) snapshot() graph.Snapshot {
	s, e := w.svc.Snapshot(w.ctx, w.b.Namespace, w.b.GraphID)
	w.must(e)
	return s
}

// obs is a minimal valid observation argument set.
func obs(title string) map[string]any {
	return map[string]any{"title": title, "statement": "The parser rejected twelve.", "evidence": "exit status 2: invalid syntax", "epistemic_status": "observed", "polarity": "negative"}
}
func data(t *testing.T, out toolkit.Envelope) map[string]any {
	t.Helper()
	if !out.OK {
		t.Fatalf("unexpected tool error %+v", out.Error)
	}
	var m map[string]any
	if e := json.Unmarshal(mustJSON(out.Data), &m); e != nil {
		t.Fatal(e)
	}
	return m
}
func wantCode(t *testing.T, out toolkit.Envelope, code string) {
	t.Helper()
	if out.OK || out.Error == nil || out.Error.Code != code {
		t.Fatalf("want error %s, got %s", code, mustJSON(out))
	}
	if out.Data != nil || out.GraphRevision != 0 {
		t.Fatalf("error envelope carries success fields: %s", mustJSON(out))
	}
	if out.Error.Retryable != (code == "storage_unavailable") {
		t.Fatalf("%s retryability %v", code, out.Error.Retryable)
	}
	if strings.TrimSpace(out.Error.Message) == "" {
		t.Fatal("error lacks a message")
	}
}
