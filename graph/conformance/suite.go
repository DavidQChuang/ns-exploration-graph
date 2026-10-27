// Package conformance runs the same semantic contract against every storage adapter.
package conformance

import (
	"context"
	"encoding/json"
	"ns-exploration-graph/graph"
	"reflect"
	"sync"
	"testing"
)

func Run(t *testing.T, newStore func(t *testing.T) graph.Store) {
	t.Helper()
	t.Run("PersistentSliceReplayAndFencing", func(t *testing.T) {
		s := graph.NewService(newStore(t))
		ctx := context.Background()
		c := graph.CommandContext{Namespace: "test", GraphID: graph.NewID(), ActorID: "tester", RequestID: graph.NewID()}
		next := func() graph.CommandContext { v := c; v.RequestID = graph.NewID(); return v }
		must := func(r graph.Result, e error) graph.Result {
			t.Helper()
			if e != nil {
				t.Fatal(e)
			}
			return r
		}
		must(s.CreateGraph(ctx, c, graph.CreateGraphInput{Title: "Parser", Request: "Investigate parser"}))
		goal := must(s.AddGoal(ctx, next(), graph.AddGoalInput{Title: "Reproduction", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Reproduce malformed input"}}}))
		work := must(s.ProposeWork(ctx, next(), graph.ProposeWorkInput{Title: "Reproduce", WorkPayload: graph.WorkPayload{Instruction: "Run parser", Priority: 50}}))
		wid := work.Data["work_item_id"].(string)
		exec := graph.NewID()
		must(s.TransitionWork(ctx, next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 1}, State: "running", ExecutionID: exec}))
		wc := next()
		wc.Execution = &graph.Execution{WorkItemID: wid, ExecutionID: exec}
		p := graph.RecordObservationInput{Title: "Negative", EvidencePayload: graph.EvidencePayload{Statement: "No crash", Evidence: "exit 0", EpistemicStatus: "observed", Polarity: "negative"}, Resources: []graph.ResourceInput{{ResourceType: "log", URI: "urn:log:parser"}}}
		obs := must(s.RecordObservation(ctx, wc, p))
		retry := must(s.RecordObservation(ctx, wc, p))
		a, _ := json.Marshal(obs)
		b, _ := json.Marshal(retry)
		if string(a) != string(b) {
			t.Fatal("Replay result changed")
		}
		changed := p
		changed.Title = "Different"
		_, e := s.RecordObservation(ctx, wc, changed)
		if graph.ErrorCode(e) != "idempotency_conflict" {
			t.Fatalf("got %v", e)
		}
		oid := obs.Data["observation_id"].(string)
		con := must(s.RecordConclusion(ctx, next(), graph.RecordConclusionInput{Title: "Finding", EvidencePayload: graph.EvidencePayload{Statement: "Parser rejects input", EpistemicStatus: "inferred"}, Supports: []graph.Ref{{NodeID: oid, Revision: 1}}}))
		cid := con.Data["conclusion_id"].(string)
		gid := goal.Data["goal_id"].(string)
		support := must(s.AddSupport(ctx, next(), graph.AddSupportInput{Source: graph.Ref{NodeID: cid, Revision: 1}, Target: graph.Ref{NodeID: gid, Revision: 1}}))
		accepted := must(s.AcceptGoal(ctx, next(), graph.AcceptGoalInput{Goal: graph.Ref{NodeID: gid, Revision: 1}, SupportEdgeIDs: []string{support.Data["edge_id"].(string)}, Method: "validator", Rationale: "Checked test output"}))
		if accepted.Data["resulting_goal_revision"] != float64(2) {
			t.Fatal("acceptance revision")
		}
		must(s.ReviseNode(ctx, next(), graph.ReviseNodeInput{Node: graph.Ref{NodeID: oid, Revision: 1}, Title: "Corrected", Payload: json.RawMessage(`{"statement":"Parser behavior corrected","evidence":"new log","epistemic_status":"observed"}`)}))
		v, e := s.GetNode(ctx, c.Namespace, c.GraphID, graph.GetNodeOptions{NodeID: gid})
		if e != nil || v.Revision.State != "needs_review" {
			t.Fatalf("transitive invalidation: %#v %v", v, e)
		}
		_, e = s.ProposeWork(ctx, next(), graph.ProposeWorkInput{Title: "Stale derived work", WorkPayload: graph.WorkPayload{Instruction: "Do not use stale conclusion"}, Parents: []graph.Ref{{NodeID: cid, Revision: 1}}})
		if graph.ErrorCode(e) != "stale_evidence" {
			t.Fatalf("stale conclusion accepted: %v", e)
		}
		must(s.TransitionWork(ctx, next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 2}, State: "stopped"}))
		must(s.TransitionWork(ctx, next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 3}, State: "open"}))
		must(s.TransitionWork(ctx, next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 4}, State: "running", ExecutionID: graph.NewID()}))
		stale := next()
		stale.Execution = wc.Execution
		_, e = s.RecordObservation(ctx, stale, p)
		if graph.ErrorCode(e) != "stale_execution" {
			t.Fatalf("stale execution accepted: %v", e)
		}
		must(s.TransitionWork(ctx, next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 5}, State: "done"}))
		must(s.SetGraphState(ctx, next(), graph.SetGraphStateInput{State: "closed"}))
		retry = must(s.RecordObservation(ctx, wc, p))
		b, _ = json.Marshal(retry)
		if string(a) != string(b) {
			t.Fatal("closed replay changed")
		}
		s = graph.NewService(s.Store())
		snap, e := s.Snapshot(ctx, c.Namespace, c.GraphID)
		if e != nil {
			t.Fatal(e)
		}
		if len(snap.Nodes) != 5 {
			t.Fatalf("nodes %d", len(snap.Nodes))
		}
		events, e := s.EventsAfter(ctx, c.Namespace, c.GraphID, 0, 100)
		if e != nil || int64(len(events.Events)) != snap.Graph.Revision {
			t.Fatalf("events mismatch %v", e)
		}
	})
	t.Run("ConcurrentClaimsAndDependencies", func(t *testing.T) {
		s := graph.NewService(newStore(t))
		ctx := context.Background()
		c := graph.CommandContext{Namespace: "test", GraphID: graph.NewID(), ActorID: "tester", RequestID: graph.NewID()}
		if _, e := s.CreateGraph(ctx, c, graph.CreateGraphInput{Title: "Race", Request: "Race"}); e != nil {
			t.Fatal(e)
		}
		newWork := func() string {
			c.RequestID = graph.NewID()
			r, e := s.ProposeWork(ctx, c, graph.ProposeWorkInput{Title: "Work", WorkPayload: graph.WorkPayload{Instruction: "test"}})
			if e != nil {
				t.Fatal(e)
			}
			return r.Data["work_item_id"].(string)
		}
		a, b := newWork(), newWork()
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			cc := c
			cc.RequestID = graph.NewID()
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, e := s.TransitionWork(ctx, cc, graph.TransitionWorkInput{Node: graph.Ref{NodeID: a, Revision: 1}, State: "running", ExecutionID: graph.NewID()})
				results <- e
			}()
		}
		wg.Wait()
		close(results)
		wins := 0
		for e := range results {
			if e == nil {
				wins++
			}
		}
		if wins != 1 {
			t.Fatalf("claim wins %d", wins)
		}
		d := newWork()
		results = make(chan error, 2)
		for i := 0; i < 2; i++ {
			cc := c
			cc.RequestID = graph.NewID()
			x, y := b, d
			if i == 1 {
				x, y = y, x
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, e := s.AddDependency(ctx, cc, graph.DependencyInput{Dependent: graph.Ref{NodeID: x, Revision: 1}, PrerequisiteID: y})
				results <- e
			}()
		}
		wg.Wait()
		close(results)
		wins = 0
		for e := range results {
			if e == nil {
				wins++
			} else if graph.ErrorCode(e) != "cycle_detected" {
				t.Fatal(e)
			}
		}
		if wins != 1 {
			t.Fatalf("dependency wins %d", wins)
		}
	})
	t.Run("FailedCommandHasNoEventAndCursorStales", func(t *testing.T) {
		s := graph.NewService(newStore(t))
		ctx := context.Background()
		c := graph.CommandContext{Namespace: "test", GraphID: graph.NewID(), ActorID: "tester", RequestID: graph.NewID()}
		if _, e := s.CreateGraph(ctx, c, graph.CreateGraphInput{Title: "Cursor", Request: "test"}); e != nil {
			t.Fatal(e)
		}
		for i := 0; i < 3; i++ {
			c.RequestID = graph.NewID()
			if _, e := s.AddHint(ctx, c, graph.AddHintInput{Title: "Hint", HintPayload: graph.HintPayload{Instruction: "test"}}); e != nil {
				t.Fatal(e)
			}
		}
		before, _ := s.Snapshot(ctx, c.Namespace, c.GraphID)
		page, e := s.ListNodes(ctx, c.Namespace, c.GraphID, graph.ListOptions{Limit: 1})
		if e != nil || page.NextCursor == "" {
			t.Fatal(e)
		}
		c.RequestID = graph.NewID()
		_, e = s.ProposeWork(ctx, c, graph.ProposeWorkInput{Title: "Invalid", WorkPayload: graph.WorkPayload{Instruction: "test"}, Parents: []graph.Ref{{NodeID: graph.NewID(), Revision: 1}}})
		if graph.ErrorCode(e) != "not_found" {
			t.Fatal(e)
		}
		after, _ := s.Snapshot(ctx, c.Namespace, c.GraphID)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("failed command mutated snapshot")
		}
		if _, e = s.AddHint(ctx, c, graph.AddHintInput{Title: "Retry failed key", HintPayload: graph.HintPayload{Instruction: "test"}}); e != nil {
			t.Fatal(e)
		}
		_, e = s.ListNodes(ctx, c.Namespace, c.GraphID, graph.ListOptions{Limit: 1, Cursor: page.NextCursor})
		if graph.ErrorCode(e) != "stale_cursor" {
			t.Fatal(e)
		}
		wrong := c
		wrong.Namespace = "other"
		_, e = s.Snapshot(ctx, wrong.Namespace, wrong.GraphID)
		if graph.ErrorCode(e) != "permission_denied" {
			t.Fatal(e)
		}
	})
}
