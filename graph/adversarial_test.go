package graph_test

import (
	"context"
	"encoding/json"
	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/store/memory"
	"strings"
	"testing"
)

type fixture struct {
	s *graph.Service
	c graph.CommandContext
	t *testing.T
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{graph.NewService(memory.New()), graph.CommandContext{Namespace: "fixture", GraphID: graph.NewID(), ActorID: "tester"}, t}
	f.must(f.s.CreateGraph(context.Background(), f.next(), graph.CreateGraphInput{Title: "Investigation", Request: "Investigate parser behavior"}))
	return f
}
func (f *fixture) next() graph.CommandContext { c := f.c; c.RequestID = graph.NewID(); return c }
func (f *fixture) must(r graph.Result, e error) graph.Result {
	f.t.Helper()
	if e != nil {
		f.t.Fatal(e)
	}
	return r
}
func (f *fixture) work() (string, *graph.Execution) {
	ctx := context.Background()
	r := f.must(f.s.ProposeWork(ctx, f.next(), graph.ProposeWorkInput{Title: "Work", WorkPayload: graph.WorkPayload{Instruction: "Observe parser"}}))
	id := r.Data["work_item_id"].(string)
	x := &graph.Execution{WorkItemID: id, ExecutionID: graph.NewID()}
	f.must(f.s.TransitionWork(ctx, f.next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: id, Revision: 1}, State: "running", ExecutionID: x.ExecutionID}))
	return id, x
}
func TestSnapshotRoundTripAndAdversarialValidation(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	wid, x := f.work()
	wc := f.next()
	wc.Execution = x
	obs := f.must(f.s.RecordObservation(ctx, wc, graph.RecordObservationInput{Title: "Reference evidence", EvidencePayload: graph.EvidencePayload{Statement: "Failure reproduced", EpistemicStatus: "observed"}, Resources: []graph.ResourceInput{{ResourceType: "log", URI: "urn:parser:log"}}}))
	oid := obs.Data["observation_id"].(string)
	goal := f.must(f.s.AddGoal(ctx, f.next(), graph.AddGoalInput{Title: "Confirm", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Observed log"}}}))
	gid := goal.Data["goal_id"].(string)
	support := f.must(f.s.AddSupport(ctx, f.next(), graph.AddSupportInput{Source: graph.Ref{NodeID: oid, Revision: 1}, Target: graph.Ref{NodeID: gid, Revision: 1}}))
	f.must(f.s.AcceptGoal(ctx, f.next(), graph.AcceptGoalInput{Goal: graph.Ref{NodeID: gid, Revision: 1}, SupportEdgeIDs: []string{support.Data["edge_id"].(string)}, Method: "human", Rationale: "Reviewed log"}))
	running, e := f.s.ExportSnapshot(ctx, f.c.Namespace, f.c.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = graph.ValidateSnapshot(running); graph.ErrorCode(e) != "invalid_argument" {
		t.Fatal("running restore allowed")
	}
	f.must(f.s.TransitionWork(ctx, f.next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 2}, State: "done"}))
	valid, e := f.s.ExportSnapshot(ctx, f.c.Namespace, f.c.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = graph.ValidateSnapshot(valid); e != nil {
		t.Fatalf("valid snapshot rejected %v", e)
	}
	dest := f.next()
	dest.GraphID = graph.NewID()
	dest.Namespace = "restored"
	restored := f.must(f.s.ImportSnapshot(ctx, dest, valid))
	if restored.GraphRevision != 1 {
		t.Fatal("destination revision must start at 1")
	}
	again := f.must(f.s.ImportSnapshot(ctx, dest, valid))
	if string(marshal(restored)) != string(marshal(again)) {
		t.Fatal("import replay changed")
	}
	got, e := f.s.Snapshot(ctx, dest.Namespace, dest.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	var source graph.Snapshot
	_ = json.Unmarshal(valid, &source)
	if string(marshal(got.Nodes)) != string(marshal(source.Nodes)) || string(marshal(got.Edges)) != string(marshal(source.Edges)) {
		t.Fatal("history not preserved")
	}
	exported, e := f.s.ExportSnapshot(ctx, dest.Namespace, dest.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	third := f.next()
	third.GraphID = graph.NewID()
	_, e = f.s.ImportSnapshot(ctx, third, exported)
	if graph.ErrorCode(e) != "invalid_argument" {
		t.Fatal("metadata collision not rejected")
	}
	cases := map[string]func(*graph.Snapshot){"revision gap": func(s *graph.Snapshot) { s.Nodes[0].Revisions[0].Revision = 2 }, "invalid pointer": func(s *graph.Snapshot) { s.Nodes[0].CurrentRevision = 999 }, "duplicate identity": func(s *graph.Snapshot) { s.Nodes = append(s.Nodes, s.Nodes[0]) }, "no origin": func(s *graph.Snapshot) {
		for i, n := range s.Nodes {
			if n.Kind == "origin" {
				s.Nodes = append(s.Nodes[:i], s.Nodes[i+1:]...)
				break
			}
		}
	}, "foreign edge": func(s *graph.Snapshot) { s.Edges[0].SourceID = graph.NewID() }, "acceptance resource omission": func(s *graph.Snapshot) { s.Acceptances[0].ReviewedResourceIDs = []string{} }, "met without acceptance": func(s *graph.Snapshot) { s.Acceptances = []graph.Acceptance{} }, "invalid uri": func(s *graph.Snapshot) { s.ResourceRefs[0].URI = "relative/path" }, "invalid uuid": func(s *graph.Snapshot) { s.Nodes[0].ID = "not-a-uuid" }, "invalid schema": func(s *graph.Snapshot) { s.SchemaVersion = 2 }}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			var candidate graph.Snapshot
			_ = json.Unmarshal(valid, &candidate)
			change(&candidate)
			if _, e := graph.ValidateSnapshot(marshal(candidate)); graph.ErrorCode(e) != "invalid_argument" {
				t.Fatalf("malformed snapshot accepted: %v", e)
			}
		})
	}
	f.must(f.s.RetractEvidence(ctx, f.next(), graph.NodeActionInput{Node: graph.Ref{NodeID: oid, Revision: 1}, Reason: "Log invalidated"}))
	retracted, e := f.s.ExportSnapshot(ctx, f.c.Namespace, f.c.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = graph.ValidateSnapshot(retracted); e != nil {
		t.Fatalf("retracted reference-only observation rejected: %v", e)
	}
	goalView, e := f.s.GetNode(ctx, f.c.Namespace, f.c.GraphID, graph.GetNodeOptions{NodeID: gid})
	if e != nil || goalView.Revision.State != "needs_review" {
		t.Fatalf("acceptance not invalidated %v", e)
	}
}
func marshal(v any) []byte { b, _ := json.Marshal(v); return b }
func wire(v any) []byte {
	var obj map[string]any
	_ = json.Unmarshal(marshal(v), &obj)
	rev := obj["graph_revision"]
	delete(obj, "graph_revision")
	return marshal(map[string]any{"ok": true, "graph_revision": rev, "data": obj})
}
func TestProjectionAndRelatedPaginationBudgets(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	wid, x := f.work()
	goal := f.must(f.s.AddGoal(ctx, f.next(), graph.AddGoalInput{Title: "Mandatory goal", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Keep this criterion"}}}))
	for i := 0; i < 12; i++ {
		c := f.next()
		c.Execution = x
		f.must(f.s.RecordObservation(ctx, c, graph.RecordObservationInput{Title: "Negative observation", EvidencePayload: graph.EvidencePayload{Statement: strings.Repeat("Observed negative result. ", 12), Evidence: "test log", EpistemicStatus: "observed", Polarity: "negative"}}))
	}
	p, e := f.s.ProjectContext(ctx, f.c.Namespace, f.c.GraphID, graph.ProjectionOptions{ByteBudget: 2600, AssignedWorkItemID: wid})
	if e != nil {
		t.Fatal(e)
	}
	if len(wire(p)) > 2600 || len(p.Goals) != 1 || p.Goals[0].ID != goal.Data["goal_id"] || len(p.Work) == 0 || !p.Truncated || p.Omitted["observation"] == 0 {
		t.Fatalf("invalid bounded projection size=%d %#v", len(wire(p)), p)
	}
	if p.Retrieval.GetNode == "" || p.Retrieval.ListNodes == "" {
		t.Fatal("missing retrieval routes")
	}
	_, e = f.s.ProjectContext(ctx, f.c.Namespace, f.c.GraphID, graph.ProjectionOptions{ByteBudget: 1024, AssignedWorkItemID: wid})
	if graph.ErrorCode(e) != "budget_too_small" {
		t.Fatalf("expected anchor budget error, got %v", e)
	}
	for i := 0; i < 8; i++ {
		f.must(f.s.AttachResource(ctx, f.next(), graph.AttachResourceInput{Node: graph.Ref{NodeID: wid, Revision: 2}, Resource: graph.ResourceInput{ResourceType: "log", URI: "urn:log:" + graph.NewID()}}))
	}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 30; page++ {
		v, e := f.s.GetNode(ctx, f.c.Namespace, f.c.GraphID, graph.GetNodeOptions{NodeID: wid, Revision: 2, RelatedCursor: cursor, ByteBudget: 1600})
		if e != nil {
			t.Fatal(e)
		}
		if len(wire(v)) > 1600 {
			t.Fatal("node budget exceeded")
		}
		for _, ed := range v.Edges {
			if seen[ed.ID] {
				t.Fatal("duplicate page record")
			}
			seen[ed.ID] = true
		}
		for _, r := range v.ResourceRefs {
			if seen[r.ID] {
				t.Fatal("duplicate page record")
			}
			seen[r.ID] = true
		}
		if v.NextRelatedCursor == "" {
			if len(seen) != v.TotalRelated {
				t.Fatalf("missing records %d/%d", len(seen), v.TotalRelated)
			}
			break
		}
		if v.NextRelatedCursor == cursor {
			t.Fatal("cursor failed to advance")
		}
		cursor = v.NextRelatedCursor
		if page == 29 {
			t.Fatal("pagination failed to finish")
		}
	}
}
func TestCanonicalReplayAndOversizeMutationRollback(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	h := f.must(f.s.AddHint(ctx, f.next(), graph.AddHintInput{Title: "Hint", HintPayload: graph.HintPayload{Instruction: "Initial"}}))
	id := h.Data["hint_id"].(string)
	c := f.next()
	one := f.must(f.s.ReviseNode(ctx, c, graph.ReviseNodeInput{Node: graph.Ref{NodeID: id, Revision: 1}, Title: "Hint", Payload: json.RawMessage(`{"instruction":"New","extensions":{"a":1,"b":2}}`)}))
	two := f.must(f.s.ReviseNode(ctx, c, graph.ReviseNodeInput{Node: graph.Ref{NodeID: id, Revision: 1}, Title: "Hint", Payload: json.RawMessage(`{"extensions":{"b":2,"a":1},"instruction":"New"}`)}))
	if string(marshal(one)) != string(marshal(two)) {
		t.Fatal("canonical replay differs")
	}
	before, _ := f.s.Snapshot(ctx, f.c.Namespace, f.c.GraphID)
	f.s.Limits.ResultBytes = 50
	_, e := f.s.AddHint(ctx, f.next(), graph.AddHintInput{Title: "Too large result", HintPayload: graph.HintPayload{Instruction: "A"}})
	if graph.ErrorCode(e) != "limit_exceeded" {
		t.Fatal(e)
	}
	after, _ := f.s.Snapshot(ctx, f.c.Namespace, f.c.GraphID)
	if string(marshal(before)) != string(marshal(after)) {
		t.Fatal("oversized mutation committed")
	}
	f.s.Limits.ResultBytes = 64 * 1024
	one.Data["node_id"] = "tamper"
	again := f.must(f.s.ReviseNode(ctx, c, graph.ReviseNodeInput{Node: graph.Ref{NodeID: id, Revision: 1}, Title: "Hint", Payload: json.RawMessage(`{"instruction":"New","extensions":{"a":1,"b":2}}`)}))
	if again.Data["node_id"] == "tamper" {
		t.Fatal("caller mutated committed result")
	}
}
