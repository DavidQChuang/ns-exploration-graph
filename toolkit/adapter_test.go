package toolkit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/store/memory"
	"ns-exploration-graph/toolkit"
)

func setup(t *testing.T) (*graph.Service, toolkit.Binding, graph.CommandContext) {
	t.Helper()
	s := graph.NewService(memory.New())
	b := toolkit.Binding{Namespace: "test", GraphID: graph.NewID(), ActorID: "tester", Role: "planner"}
	c := graph.CommandContext{Namespace: b.Namespace, GraphID: b.GraphID, ActorID: b.ActorID, RequestID: graph.NewID()}
	_, e := s.CreateGraph(context.Background(), c, graph.CreateGraphInput{Title: "Parser investigation", Request: "Check parser rejection"})
	if e != nil {
		t.Fatal(e)
	}
	return s, b, c
}
func call(a *toolkit.Adapter, name string, args any, id string) toolkit.Envelope {
	b, _ := json.Marshal(args)
	return a.Invoke(context.Background(), name, b, id)
}
func TestManifestParityAndPrompts(t *testing.T) {
	source, e := os.ReadFile("../spec/tool-contracts.json")
	if e != nil {
		t.Fatal(e)
	}
	embedded, e := os.ReadFile("contracts.json")
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(source, embedded) {
		t.Fatal("embedded contract drift: run python toolkit/generate_contracts.py")
	}
	ds, e := toolkit.Definitions()
	if e != nil || len(ds) != 8 {
		t.Fatalf("definitions %d %v", len(ds), e)
	}
	for _, role := range []string{"planner", "worker", "solo"} {
		p, e := toolkit.Prompt(role)
		if e != nil {
			t.Fatal(e)
		}
		for _, required := range []string{"untrusted", "exact returned IDs", "stale_evidence", "host owns work completion"} {
			if !strings.Contains(p, required) {
				t.Fatalf("%s lacks %q", role, required)
			}
		}
	}
}
func TestStrictNestedInputSchema(t *testing.T) {
	id := graph.NewID()
	tests := []struct{ name, input string }{
		{"graph_overview", `{"actor_id":"spoof"}`},
		{"graph_get_node", `{"node_id":"not-a-uuid"}`},
		{"graph_list_nodes", `{"limit":1.5}`},
		{"graph_propose_work", `{"title":"t","instruction":"i","priority":0,"parents":[{"id":"` + id + `","revision":1,"execution_id":"spoof"}]}`},
		{"graph_propose_work", `{"title":"t","instruction":"i","priority":0,"parents":[{"id":"` + id + `"}]}`},
		{"graph_record_observation", `{"title":"t","statement":"s","evidence":"e","epistemic_status":"observed","resource_refs":[{"resource_type":"log","uri":"file:///x","host_actor":"spoof"}]}`},
		{"graph_record_observation", `{"title":"t","statement":"s","evidence":"e","epistemic_status":"observed","resource_refs":[{"resource_type":"log","uri":"relative/path"}]}`},
		{"graph_record_conclusion", `{"title":"t","statement":"s","epistemic_status":"verified","supports":[]}`},
		{"graph_add_support", `{"source":{"id":"` + id + `","revision":0},"target":{"id":"` + id + `","revision":1}}`},
		{"graph_accept_goal", `{"goal":{"id":"` + id + `","revision":1},"support_edge_ids":["` + id + `","` + id + `"],"rationale":"r"}`},
	}
	for _, tt := range tests {
		if e := toolkit.ValidateInput(tt.name, []byte(tt.input)); e == nil {
			t.Fatalf("accepted %s %s", tt.name, tt.input)
		}
	}
	good := `{"title":"t","instruction":"i","priority":0,"parents":[{"id":"` + id + `","revision":1}]}`
	if e := toolkit.ValidateInput("graph_propose_work", []byte(good)); e != nil {
		t.Fatal(e)
	}
}
func TestAuthorizationEveryRetryAndNoSpoofing(t *testing.T) {
	s, b, _ := setup(t)
	count := 0
	allow := true
	a, e := toolkit.New(s, toolkit.Config{Binding: b, Authorize: func(context.Context, toolkit.Binding, string) error {
		count++
		if !allow {
			return graph.Err("permission_denied", "no")
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	args := map[string]any{"title": "reproduce", "instruction": "test parser", "priority": 1}
	first := call(a, "graph_propose_work", args, "durable-1")
	if !first.OK {
		t.Fatal(first.Error)
	}
	retry := call(a, "graph_propose_work", args, "durable-1")
	if !retry.OK {
		t.Fatal(retry.Error)
	}
	x, _ := json.Marshal(first)
	y, _ := json.Marshal(retry)
	if !bytes.Equal(x, y) {
		t.Fatal("retry did not preserve exact original result")
	}
	allow = false
	denied := call(a, "graph_propose_work", args, "durable-1")
	if denied.Error == nil || denied.Error.Code != "permission_denied" || count != 3 {
		t.Fatalf("authorization missing on retry: %+v count %d", denied, count)
	}
	b.Role = "worker"
	worker, e := toolkit.New(s, toolkit.Config{Binding: b})
	if e != nil {
		t.Fatal(e)
	}
	for _, n := range []string{"graph_propose_work", "graph_add_support", "graph_accept_goal"} {
		out := call(worker, n, map[string]any{}, "x")
		if out.Error == nil || out.Error.Code != "permission_denied" {
			t.Fatalf("worker called %s", n)
		}
	}
	out := call(worker, "graph_record_observation", map[string]any{"title": "t", "statement": "s", "evidence": "e", "epistemic_status": "observed"}, "x")
	if out.Error == nil || out.Error.Code != "permission_denied" {
		t.Fatal("unbound worker observation")
	}
}
func TestBoundWorkerAndAcceptanceContract(t *testing.T) {
	ctx := context.Background()
	s, b, c := setup(t)
	next := func() graph.CommandContext { c.RequestID = graph.NewID(); return c }
	goal, e := s.AddGoal(ctx, next(), graph.AddGoalInput{Title: "Reject malformed input", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"failure reproduced"}}})
	if e != nil {
		t.Fatal(e)
	}
	work, e := s.ProposeWork(ctx, next(), graph.ProposeWorkInput{Title: "reproduce", WorkPayload: graph.WorkPayload{Instruction: "test", Priority: 1}})
	if e != nil {
		t.Fatal(e)
	}
	wid := work.Data["work_item_id"].(string)
	x := graph.NewID()
	_, e = s.TransitionWork(ctx, next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 1}, State: "running", ExecutionID: x})
	if e != nil {
		t.Fatal(e)
	}
	worker := b
	worker.Role = "worker"
	worker.Execution = &graph.Execution{WorkItemID: wid, ExecutionID: x}
	wa, e := toolkit.New(s, toolkit.Config{Binding: worker})
	if e != nil {
		t.Fatal(e)
	}
	ob := call(wa, "graph_record_observation", map[string]any{"title": "negative", "statement": "Malformed input rejected", "evidence": "exit 2", "epistemic_status": "observed", "polarity": "negative"}, "ob-1")
	if !ob.OK {
		t.Fatal(ob.Error)
	}
	if e := toolkit.ValidateOutput("graph_record_observation", mustJSON(ob)); e != nil {
		t.Fatal(e)
	}
	oid := ob.Data.(map[string]any)["observation_id"].(string)
	pa, e := toolkit.New(s, toolkit.Config{Binding: b})
	if e != nil {
		t.Fatal(e)
	}
	co := call(pa, "graph_record_conclusion", map[string]any{"title": "Rejection confirmed", "statement": "Reproduced rejection", "epistemic_status": "inferred", "supports": []any{map[string]any{"id": oid, "revision": 1}}}, "co-1")
	if !co.OK {
		t.Fatal(co.Error)
	}
	cid := co.Data.(map[string]any)["conclusion_id"].(string)
	gid := goal.Data["goal_id"].(string)
	edge := call(pa, "graph_add_support", map[string]any{"source": map[string]any{"id": cid, "revision": 1}, "target": map[string]any{"id": gid, "revision": 1}}, "edge-1")
	if !edge.OK {
		t.Fatal(edge.Error)
	}
	b.AcceptGoal = true
	b.AcceptanceMethod = "validator"
	policies := 0
	accept, e := toolkit.New(s, toolkit.Config{Binding: b, AcceptancePolicy: func(context.Context, toolkit.Binding, graph.AcceptGoalInput) error { policies++; return nil }})
	if e != nil {
		t.Fatal(e)
	}
	out := call(accept, "graph_accept_goal", map[string]any{"goal": map[string]any{"id": gid, "revision": 1}, "support_edge_ids": []any{edge.Data.(map[string]any)["edge_id"]}, "rationale": "Reviewed negative test"}, "accept-1")
	if !out.OK {
		t.Fatal(out.Error)
	}
	if e := toolkit.ValidateOutput("graph_accept_goal", mustJSON(out)); e != nil {
		t.Fatal(e)
	}
	if policies != 1 {
		t.Fatal("accept policy not called")
	}
	var data map[string]any
	_ = json.Unmarshal(mustJSON(out.Data), &data)
	if len(data["reviewed_evidence"].([]any)) != 2 || len(data["support_edge_ids"].([]any)) != 2 {
		t.Fatalf("missing full reviewed closure %+v", data)
	}
}
func mustJSON(v any) []byte {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return b
}
