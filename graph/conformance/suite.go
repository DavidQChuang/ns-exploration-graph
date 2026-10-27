// Package conformance runs the same semantic contract against every storage adapter.
package conformance

import (
	"context"
	"encoding/json"
	"ns-exploration-graph/graph"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func Run(t *testing.T, newStore func(t *testing.T) graph.Store) {
	t.Helper()
	runBounded(t, newStore)
	runGaps(t, newStore)
	runPublishedBoundaries(t, newStore)
	runHostLifecycle(t, newStore)
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

// These tests exercise public limits through each actual store. Larger read
// fixtures use one validated import instead of hundreds of unrelated writes.
func runPublishedBoundaries(t *testing.T, newStore func(t *testing.T) graph.Store) {
	t.Run("PublishedReadBoundsAndDefaults", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Bounds")
		snap := w.snapshot()
		origin := snap.Nodes[0]
		works := make([]string, 101)
		for i := range works {
			works[i] = graph.NewID()
			n := origin
			n.ID, n.Kind = works[i], "work_item"
			n.Revisions = append([]graph.Revision(nil), origin.Revisions...)
			n.Revisions[0].State, n.Revisions[0].Title = "open", "Work"
			n.Revisions[0].Payload = json.RawMessage(`{"instruction":"Inspect boundary","priority":0}`)
			snap.Nodes = append(snap.Nodes, n)
			snap.Edges = append(snap.Edges, graph.Edge{ID: graph.NewID(), SourceID: origin.ID, TargetID: n.ID, SourceRevision: 1, TargetRevision: 1, Relation: "derived_from", ActorID: w.c.ActorID, CreatedAt: origin.CreatedAt})
			if i > 0 {
				snap.Edges = append(snap.Edges, graph.Edge{ID: graph.NewID(), SourceID: works[i-1], TargetID: n.ID, Relation: "depends_on", ActorID: w.c.ActorID, CreatedAt: origin.CreatedAt})
			}
		}
		b, e := json.Marshal(snap)
		if e != nil {
			t.Fatal(e)
		}
		w.c.GraphID = graph.NewID()
		w.must(w.s.ImportSnapshot(w.ctx, w.next(), b))
		for _, tc := range []struct{ limit, count int }{{0, 20}, {1, 1}, {100, 100}} {
			p, e := w.s.ListNodes(w.ctx, w.c.Namespace, w.c.GraphID, graph.ListOptions{Limit: tc.limit})
			if e != nil || len(p.Nodes) != tc.count || p.Total != 102 || p.Omitted != 102-tc.count || p.NextCursor == "" {
				t.Fatalf("list limit %d: count=%d total=%d omitted=%d cursor=%q err=%v", tc.limit, len(p.Nodes), p.Total, p.Omitted, p.NextCursor, e)
			}
		}
		for _, limit := range []int{-1, 101} {
			if _, e := w.s.ListNodes(w.ctx, w.c.Namespace, w.c.GraphID, graph.ListOptions{Limit: limit}); graph.ErrorCode(e) != "limit_exceeded" {
				t.Fatalf("list limit %d: %v", limit, e)
			}
		}
		for _, direction := range []string{"ancestors", "descendants"} {
			walk := w.s.Descendants
			start := works[0]
			if direction == "ancestors" {
				walk, start = w.s.Ancestors, works[100]
			}
			for _, tc := range []struct{ depth, count int }{{0, 8}, {1, 1}, {64, 64}} {
				p, e := walk(w.ctx, w.c.Namespace, w.c.GraphID, graph.TraversalOptions{NodeID: start, Relations: []string{"depends_on"}, Depth: tc.depth, Limit: 1000})
				if e != nil || len(p.Nodes) != tc.count || p.Total != tc.count {
					t.Fatalf("%s depth %d: count=%d total=%d err=%v", direction, tc.depth, len(p.Nodes), p.Total, e)
				}
			}
			for _, depth := range []int{-1, 65} {
				if _, e := walk(w.ctx, w.c.Namespace, w.c.GraphID, graph.TraversalOptions{NodeID: start, Relations: []string{"depends_on"}, Depth: depth}); graph.ErrorCode(e) != "limit_exceeded" {
					t.Fatalf("%s depth %d: %v", direction, depth, e)
				}
			}
		}
		for _, tc := range []struct{ limit, count int }{{0, 100}, {1, 1}, {1000, 101}} {
			p, e := w.s.Descendants(w.ctx, w.c.Namespace, w.c.GraphID, graph.TraversalOptions{NodeID: origin.ID, Relations: []string{"derived_from"}, Limit: tc.limit})
			if e != nil || len(p.Nodes) != tc.count || p.Total != 101 || p.Omitted != 101-tc.count || (p.NextCursor != "") != (tc.count < 101) {
				t.Fatalf("traversal limit %d: %#v %v", tc.limit, p, e)
			}
		}
		for _, limit := range []int{-1, 1001} {
			if _, e := w.s.Descendants(w.ctx, w.c.Namespace, w.c.GraphID, graph.TraversalOptions{NodeID: origin.ID, Relations: []string{"derived_from"}, Limit: limit}); graph.ErrorCode(e) != "limit_exceeded" {
				t.Fatalf("traversal limit %d: %v", limit, e)
			}
		}
	})
	t.Run("PublishedOverviewBoundsAndDefault", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Bounds")
		for _, budget := range []int{1024, 65536} {
			p, e := w.s.ProjectContext(w.ctx, w.c.Namespace, w.c.GraphID, graph.ProjectionOptions{ByteBudget: budget})
			if e != nil || len(wire(t, p)) > budget {
				t.Fatalf("budget %d: %v", budget, e)
			}
		}
		for _, budget := range []int{-1, 1023, 65537} {
			if _, e := w.s.ProjectContext(w.ctx, w.c.Namespace, w.c.GraphID, graph.ProjectionOptions{ByteBudget: budget}); graph.ErrorCode(e) != "limit_exceeded" {
				t.Fatalf("budget %d: %v", budget, e)
			}
		}
		for i := 0; i < 3; i++ {
			w.must(w.s.AddHint(w.ctx, w.next(), graph.AddHintInput{Title: "Large hint", HintPayload: graph.HintPayload{Instruction: strings.Repeat("界", 3000)}}))
		}
		implicit, e := w.s.ProjectContext(w.ctx, w.c.Namespace, w.c.GraphID, graph.ProjectionOptions{})
		if e != nil {
			t.Fatal(e)
		}
		explicit, e := w.s.ProjectContext(w.ctx, w.c.Namespace, w.c.GraphID, graph.ProjectionOptions{ByteBudget: 16384})
		if e != nil || !reflect.DeepEqual(implicit, explicit) || !implicit.Truncated || len(wire(t, implicit)) > 16384 {
			t.Fatalf("default overview is not bounded at 16KiB: %v", e)
		}
		full, e := w.s.ProjectContext(w.ctx, w.c.Namespace, w.c.GraphID, graph.ProjectionOptions{ByteBudget: 65536})
		if e != nil || full.Truncated || len(full.Hints) != 3 {
			t.Fatalf("large budget did not recover hints: %v", e)
		}
	})
	t.Run("RequestIdentityCharactersAndCommandBytes", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Bounds")
		if w.s.Limits.CommandBytes != 256*1024 || w.s.Limits.ResultBytes != 64*1024 || w.s.Limits.SnapshotBytes != 64*1024*1024 || w.s.Limits.MaxReferences != 100 {
			t.Fatalf("published defaults drifted: %#v", w.s.Limits)
		}
		p := graph.AddHintInput{Title: "Boundary", HintPayload: graph.HintPayload{Instruction: "Inspect"}}
		for _, id := range []string{strings.Repeat("a", 200), strings.Repeat("界", 200)} {
			c := w.next()
			c.RequestID = id
			first := w.must(w.s.AddHint(w.ctx, c, p))
			again := w.must(w.s.AddHint(w.ctx, c, p))
			if !reflect.DeepEqual(first, again) {
				t.Fatal("boundary identity did not replay")
			}
		}
		for _, id := range []string{strings.Repeat("a", 201), strings.Repeat("界", 201), "invalid-" + string([]byte{0xff})} {
			before := w.snapshot()
			c := w.next()
			c.RequestID = id
			if _, e := w.s.AddHint(w.ctx, c, p); graph.ErrorCode(e) != "invalid_argument" {
				t.Fatalf("invalid identity accepted: %v", e)
			}
			assertBoundaryUnchanged(t, w, before)
		}
		// Measure the public command encoding independently; padding with multibyte
		// text distinguishes the documented UTF-8 byte cap from character count.
		c := w.next()
		p.Instruction = ""
		b, e := json.Marshal(map[string]any{"context": c, "kind": "AddHint", "payload": p})
		if e != nil {
			t.Fatal(e)
		}
		padding := 256*1024 - len(b)
		p.Instruction = strings.Repeat("界", padding/3) + strings.Repeat("x", padding%3)
		w.must(w.s.AddHint(w.ctx, c, p))
		c = w.next()
		p.Instruction += "x"
		before := w.snapshot()
		if _, e := w.s.AddHint(w.ctx, c, p); graph.ErrorCode(e) != "limit_exceeded" {
			t.Fatalf("256KiB+1 command: %v", e)
		}
		assertBoundaryUnchanged(t, w, before)
		p.Instruction = "Retry rejected request"
		w.must(w.s.AddHint(w.ctx, c, p))
	})
	t.Run("ReferenceCapsAndMutationResultAtomicity", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Bounds")
		_, wc := w.running("Producer", 1)
		refs := make([]graph.Ref, 100)
		for i := range refs {
			refs[i] = graph.Ref{NodeID: w.observe(wc, "Observation", "Measured"), Revision: 1}
		}
		resources := make([]graph.ResourceInput, 100)
		for i := range resources {
			resources[i] = graph.ResourceInput{ResourceType: "log", URI: "urn:boundary:" + graph.NewID()}
		}
		work := graph.ProposeWorkInput{Title: "Boundary work", WorkPayload: graph.WorkPayload{Instruction: "Inspect"}, Parents: refs}
		if r := w.must(w.s.ProposeWork(w.ctx, w.next(), work)); len(r.Data["lineage_edge_ids"].([]any)) != 100 {
			t.Fatal("100 parents lost lineage")
		}
		conclusion := graph.RecordConclusionInput{Title: "Boundary conclusion", EvidencePayload: graph.EvidencePayload{Statement: "Measured", EpistemicStatus: "inferred"}, Supports: refs, Resources: resources}
		r := w.must(w.s.RecordConclusion(w.ctx, w.next(), conclusion))
		if len(r.Data["support_edge_ids"].([]any)) != 100 || len(r.Data["resource_reference_ids"].([]any)) != 100 {
			t.Fatal("100 supports/resources lost records")
		}
		observation := graph.RecordObservationInput{Title: "Boundary observation", EvidencePayload: graph.EvidencePayload{Statement: "Measured", Evidence: "Log", EpistemicStatus: "observed"}, Resources: resources}
		wc.RequestID = graph.NewID()
		r = w.must(w.s.RecordObservation(w.ctx, wc, observation))
		// Fixed-length generated UUIDs make the next same-shape result exactly as
		// large, without predicting any generated identifier.
		encoded, e := json.Marshal(struct {
			OK bool `json:"ok"`
			graph.Result
		}{true, r})
		if e != nil {
			t.Fatal(e)
		}
		w.s.Limits.ResultBytes = len(encoded)
		wc.RequestID = graph.NewID()
		at := w.must(w.s.RecordObservation(w.ctx, wc, observation))
		encodedAt, _ := json.Marshal(struct {
			OK bool `json:"ok"`
			graph.Result
		}{true, at})
		if len(encodedAt) != len(encoded) {
			t.Fatal("result fixture size changed")
		}
		w.s.Limits.ResultBytes--
		before := w.snapshot()
		wc.RequestID = graph.NewID()
		if _, e := w.s.RecordObservation(w.ctx, wc, observation); graph.ErrorCode(e) != "limit_exceeded" {
			t.Fatalf("result one byte over configured cap: %v", e)
		}
		assertBoundaryUnchanged(t, w, before)
		w.s.Limits.ResultBytes = 65536
		w.must(w.s.RecordObservation(w.ctx, wc, observation))
		for _, tc := range []struct {
			name, code string
			invoke     func(graph.CommandContext) (graph.Result, error)
		}{
			{"parents", "invalid_argument", func(c graph.CommandContext) (graph.Result, error) {
				p := work
				p.Parents = append(append([]graph.Ref(nil), refs...), refs[0])
				return w.s.ProposeWork(w.ctx, c, p)
			}},
			{"supports", "limit_exceeded", func(c graph.CommandContext) (graph.Result, error) {
				p := conclusion
				p.Supports = append(append([]graph.Ref(nil), refs...), refs[0])
				return w.s.RecordConclusion(w.ctx, c, p)
			}},
			{"conclusion resources", "limit_exceeded", func(c graph.CommandContext) (graph.Result, error) {
				p := conclusion
				p.Resources = append(append([]graph.ResourceInput(nil), resources...), resources[0])
				return w.s.RecordConclusion(w.ctx, c, p)
			}},
			{"observation resources", "limit_exceeded", func(c graph.CommandContext) (graph.Result, error) {
				p := observation
				p.Resources = append(append([]graph.ResourceInput(nil), resources...), resources[0])
				c.Execution = wc.Execution
				return w.s.RecordObservation(w.ctx, c, p)
			}},
		} {
			before := w.snapshot()
			c := w.next()
			if _, e := tc.invoke(c); graph.ErrorCode(e) != tc.code {
				t.Fatalf("101 %s: %v", tc.name, e)
			}
			assertBoundaryUnchanged(t, w, before)
			w.must(w.s.AddHint(w.ctx, c, graph.AddHintInput{Title: "Rejected key reusable", HintPayload: graph.HintPayload{Instruction: tc.name}}))
		}
		// An invalid final resource exercises rollback after the node, support
		// edges, and earlier references have already been assembled.
		before = w.snapshot()
		invalid := conclusion
		invalid.Resources = append([]graph.ResourceInput(nil), resources...)
		invalid.Resources[99].URI = "not an absolute URI"
		c := w.next()
		if _, e := w.s.RecordConclusion(w.ctx, c, invalid); graph.ErrorCode(e) != "invalid_argument" {
			t.Fatalf("invalid final resource: %v", e)
		}
		assertBoundaryUnchanged(t, w, before)
		w.must(w.s.RecordConclusion(w.ctx, c, conclusion))
		goal := w.must(w.s.AddGoal(w.ctx, w.next(), graph.AddGoalInput{Title: "Boundary goal", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Checked all observations"}}})).Data["goal_id"].(string)
		edges := make([]string, 100)
		for i, source := range refs {
			edges[i] = w.must(w.s.AddSupport(w.ctx, w.next(), graph.AddSupportInput{Source: source, Target: graph.Ref{NodeID: goal, Revision: 1}})).Data["edge_id"].(string)
		}
		accept := graph.AcceptGoalInput{Goal: graph.Ref{NodeID: goal, Revision: 1}, SupportEdgeIDs: append(append([]string(nil), edges...), graph.NewID()), Method: "validator", Rationale: "Checked all observations"}
		before = w.snapshot()
		c = w.next()
		if _, e := w.s.AcceptGoal(w.ctx, c, accept); graph.ErrorCode(e) != "invalid_argument" {
			t.Fatalf("101 direct supports: %v", e)
		}
		assertBoundaryUnchanged(t, w, before)
		accept.SupportEdgeIDs = edges
		w.must(w.s.AcceptGoal(w.ctx, c, accept))
	})
	t.Run("SnapshotByteBoundariesAreAtomic", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Bounds")
		b, e := w.s.ExportSnapshot(w.ctx, w.c.Namespace, w.c.GraphID)
		if e != nil {
			t.Fatal(e)
		}
		// A tiny valid document padded with legal JSON whitespace reaches the
		// actual 64MiB import boundary without millions of graph objects or rows.
		at := make([]byte, 64*1024*1024)
		copy(at, b)
		for i := len(b); i < len(at); i++ {
			at[i] = ' '
		}
		c := w.next()
		c.GraphID = graph.NewID()
		w.must(w.s.ImportSnapshot(w.ctx, c, at))
		c = w.next()
		c.GraphID = graph.NewID()
		if _, e := w.s.ImportSnapshot(w.ctx, c, append(at, ' ')); graph.ErrorCode(e) != "limit_exceeded" {
			t.Fatalf("64MiB+1 import: %v", e)
		}
		if _, e := w.s.Snapshot(w.ctx, c.Namespace, c.GraphID); graph.ErrorCode(e) != "not_found" {
			t.Fatalf("rejected import left graph: %v", e)
		}
		w.must(w.s.ImportSnapshot(w.ctx, c, b))
		// Export uses the same configured byte gate. The exact serialized source
		// fits; one byte less rejects without returning a partial document.
		w.s.Limits.SnapshotBytes = len(b)
		atExport, e := w.s.ExportSnapshot(w.ctx, w.c.Namespace, w.c.GraphID)
		if e != nil || string(atExport) != string(b) {
			t.Fatalf("export at exact cap: %v", e)
		}
		w.s.Limits.SnapshotBytes--
		if partial, e := w.s.ExportSnapshot(w.ctx, w.c.Namespace, w.c.GraphID); graph.ErrorCode(e) != "limit_exceeded" || len(partial) != 0 {
			t.Fatalf("export over cap: bytes=%d err=%v", len(partial), e)
		}
	})
}

func assertBoundaryUnchanged(t *testing.T, w *world, before graph.Snapshot) {
	t.Helper()
	if !reflect.DeepEqual(before, w.snapshot()) {
		t.Fatal("rejected boundary command changed graph")
	}
	events, e := w.s.EventsAfter(w.ctx, w.c.Namespace, w.c.GraphID, before.Graph.Revision, 1000)
	if e != nil || len(events.Events) != 0 || events.GraphRevision != before.Graph.Revision {
		t.Fatalf("rejected boundary command wrote events: %#v %v", events, e)
	}
}

func runHostLifecycle(t *testing.T, newStore func(t *testing.T) graph.Store) {
	runHostGoalLifecycle(t, newStore)
	runHostDependencyAndHintLifecycle(t, newStore)
	runHostGraphLifecycle(t, newStore)
	t.Run("HostResourceURIsMatchPortableContract", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Resource origin")
		producer, wc := w.running("Probe", 1)
		oid := w.observe(wc, "Observed", "Measured")
		valid := graph.ResourceInput{ResourceType: "log", URI: "urn:log:valid%20escaped"}
		for _, uri := range []string{"http://[not-ipv6]/log", "http://::1/log", "https://example.com/%zz", "relative/log", "urn:log:\ncontrol"} {
			for _, command := range []string{"RecordObservation", "RecordConclusion", "AttachResource"} {
				before := w.snapshot()
				c := w.next()
				resource := valid
				resource.URI = uri
				var e error
				switch command {
				case "RecordObservation":
					c.Execution = wc.Execution
					_, e = w.s.RecordObservation(w.ctx, c, graph.RecordObservationInput{Title: "Invalid resource", EvidencePayload: graph.EvidencePayload{Statement: "Measured", Evidence: "Log", EpistemicStatus: "observed"}, Resources: []graph.ResourceInput{valid, resource}})
				case "RecordConclusion":
					_, e = w.s.RecordConclusion(w.ctx, c, graph.RecordConclusionInput{Title: "Invalid resource", EvidencePayload: graph.EvidencePayload{Statement: "Measured", EpistemicStatus: "inferred"}, Supports: []graph.Ref{{NodeID: oid, Revision: 1}}, Resources: []graph.ResourceInput{valid, resource}})
				case "AttachResource":
					_, e = w.s.AttachResource(w.ctx, c, graph.AttachResourceInput{Node: graph.Ref{NodeID: oid, Revision: 1}, Resource: resource})
				}
				if e == nil {
					w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: producer, Revision: 2}, State: "done"}))
					encoded, exportErr := w.s.ExportSnapshot(w.ctx, w.c.Namespace, w.c.GraphID)
					_, validationErr := graph.ValidateSnapshot(encoded)
					t.Fatalf("%s accepted invalid URI %q: export=%v validation=%v", command, uri, exportErr, validationErr)
				}
				if graph.ErrorCode(e) != "invalid_argument" {
					t.Fatalf("%s accepted invalid URI %q: %v", command, uri, e)
				}
				assertBoundaryUnchanged(t, w, before)
				w.must(w.s.AttachResource(w.ctx, c, graph.AttachResourceInput{Node: graph.Ref{NodeID: oid, Revision: 1}, Resource: valid}))
			}
		}
		w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: producer, Revision: 2}, State: "done"}))
		exported, e := w.s.ExportSnapshot(w.ctx, w.c.Namespace, w.c.GraphID)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = graph.ValidateSnapshot(exported); e != nil {
			t.Fatalf("host-accepted resource did not validate portably: %v", e)
		}
		dest := w.next()
		dest.GraphID = graph.NewID()
		w.must(w.s.ImportSnapshot(w.ctx, dest, exported))
	})
}

func runHostGoalLifecycle(t *testing.T, newStore func(t *testing.T) graph.Store) {
	for _, tc := range []struct{ command, from, to string }{
		{"ReopenGoal", "met", "open"}, {"ReopenGoal", "abandoned", "open"}, {"ReopenGoal", "needs_review", "open"},
		{"AbandonGoal", "open", "abandoned"}, {"AbandonGoal", "met", "abandoned"}, {"AbandonGoal", "needs_review", "abandoned"},
	} {
		t.Run("HostGoalLifecycle/"+tc.command+"/"+tc.from, func(t *testing.T) {
			w := newWorld(t, newStore(t), "Goal lifecycle")
			var gid string
			if tc.from == "met" || tc.from == "needs_review" {
				_, wc := w.running("Probe", 1)
				oid := w.observe(wc, "Observed", "Measured")
				gid, _ = w.accepted(wc, "Reviewed goal", oid)
				if tc.from == "needs_review" {
					w.must(w.s.RetractEvidence(w.ctx, w.next(), graph.NodeActionInput{Node: graph.Ref{NodeID: oid, Revision: 1}, Reason: "Evidence withdrawn"}))
				}
			} else {
				gid = w.must(w.s.AddGoal(w.ctx, w.next(), graph.AddGoalInput{Title: "Goal", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Review evidence"}}})).Data["goal_id"].(string)
				if tc.from == "abandoned" {
					w.must(w.s.AbandonGoal(w.ctx, w.next(), graph.NodeActionInput{Node: graph.Ref{NodeID: gid, Revision: 1}, Reason: "Deferred"}))
				}
			}
			before, e := w.s.ExplainGoal(w.ctx, w.c.Namespace, w.c.GraphID, gid)
			if e != nil || before.Goal.Current().State != tc.from {
				t.Fatalf("start state: %#v %v", before, e)
			}
			call := w.s.ReopenGoal
			if tc.command == "AbandonGoal" {
				call = w.s.AbandonGoal
			}
			c := w.next()
			p := graph.NodeActionInput{Node: graph.Ref{NodeID: gid, Revision: before.Goal.CurrentRevision}, Reason: "Explicit host decision"}
			r := w.must(call(w.ctx, c, p))
			after, e := w.s.ExplainGoal(w.ctx, w.c.Namespace, w.c.GraphID, gid)
			if e != nil || r.GraphRevision != before.GraphRevision+1 || after.Goal.CurrentRevision != before.Goal.CurrentRevision+1 || after.Goal.Current().State != tc.to || len(after.Goal.Revisions) != len(before.Goal.Revisions)+1 {
				t.Fatalf("transition must add exactly one revision: %#v %v", after, e)
			}
			if !reflect.DeepEqual(before.Goal.Revisions, after.Goal.Revisions[:len(before.Goal.Revisions)]) || string(before.Goal.Current().Payload) != string(after.Goal.Current().Payload) || before.Goal.Current().Title != after.Goal.Current().Title {
				t.Fatal("goal action rewrote historical revisions or criteria")
			}
			if len(after.Acceptances) != len(before.Acceptances) {
				t.Fatal("goal action removed or fabricated acceptance history")
			}
			for i, old := range before.Acceptances {
				got := after.Acceptances[i]
				if got.State != "stale" || got.StaleReason == "" {
					t.Fatalf("acceptance remains current: %#v", got)
				}
				got.State, got.StaleReason = old.State, old.StaleReason
				if !reflect.DeepEqual(old, got) {
					t.Fatal("goal action rewrote reviewed acceptance identity/evidence")
				}
			}
			events, e := w.s.EventsAfter(w.ctx, w.c.Namespace, w.c.GraphID, before.GraphRevision, 10)
			if e != nil || len(events.Events) != 1 || events.Events[0].CommandKind != tc.command || events.Events[0].RequestID != c.RequestID || events.Events[0].ActorID != c.ActorID || !reflect.DeepEqual(events.Events[0].Result, r) {
				t.Fatalf("goal action audit event: %#v %v", events, e)
			}
			w.must(w.s.AddHint(w.ctx, w.next(), graph.AddHintInput{Title: "Later mutation", HintPayload: graph.HintPayload{Instruction: "Review"}}))
			committed := w.snapshot()
			if replay := w.must(call(w.ctx, c, p)); !reflect.DeepEqual(replay, r) {
				t.Fatal("goal action replay changed")
			}
			assertBoundaryUnchanged(t, w, committed)
			changed := p
			changed.Reason = "Different decision"
			if _, e := call(w.ctx, c, changed); graph.ErrorCode(e) != "idempotency_conflict" {
				t.Fatalf("changed retry: %v", e)
			}
			assertBoundaryUnchanged(t, w, committed)
			rejectHostMutation(t, w, "revision_conflict", func(c graph.CommandContext) (graph.Result, error) { return call(w.ctx, c, p) })
			p.Node.Revision++
			rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) { return call(w.ctx, c, p) })
			origin := ""
			for _, n := range w.snapshot().Nodes {
				if n.Kind == "origin" {
					origin = n.ID
				}
			}
			rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) {
				return call(w.ctx, c, graph.NodeActionInput{Node: graph.Ref{NodeID: origin, Revision: 1}})
			})
			if tc.command == "AbandonGoal" && tc.from == "met" {
				w.must(w.s.ReopenGoal(w.ctx, w.next(), graph.NodeActionInput{Node: p.Node, Reason: "Resume review"}))
				_, acceptances := w.explain(gid)
				for _, state := range acceptances {
					if state != "stale" {
						t.Fatal("reopening abandoned goal resurrected acceptance")
					}
				}
			}
		})
	}
}

func runHostDependencyAndHintLifecycle(t *testing.T, newStore func(t *testing.T) graph.Store) {
	t.Run("HostRemoveDependencyRestoresEligibilityWithoutRevisingWork", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Dependency lifecycle")
		dependent, prerequisite := w.open("Dependent"), w.open("Prerequisite")
		p := graph.DependencyInput{Dependent: graph.Ref{NodeID: dependent, Revision: 1}, PrerequisiteID: prerequisite}
		added := w.must(w.s.AddDependency(w.ctx, w.next(), p))
		eligible := func(id string) bool {
			page, e := w.s.ListEligibleWork(w.ctx, w.c.Namespace, w.c.GraphID)
			if e != nil {
				t.Fatal(e)
			}
			for _, n := range page.Nodes {
				if n.ID == id {
					return true
				}
			}
			return false
		}
		if eligible(dependent) || !eligible(prerequisite) {
			t.Fatal("undone prerequisite did not block only dependent work")
		}
		rejectHostMutation(t, w, "revision_conflict", func(c graph.CommandContext) (graph.Result, error) {
			q := p
			q.Dependent.Revision = 2
			return w.s.RemoveDependency(w.ctx, c, q)
		})
		rejectHostMutation(t, w, "not_found", func(c graph.CommandContext) (graph.Result, error) {
			q := p
			q.PrerequisiteID = graph.NewID()
			return w.s.RemoveDependency(w.ctx, c, q)
		})
		before := w.snapshot()
		c := w.next()
		removed := w.must(w.s.RemoveDependency(w.ctx, c, p))
		after := w.snapshot()
		if removed.Data["edge_id"] != added.Data["edge_id"] || removed.GraphRevision != before.Graph.Revision+1 || !reflect.DeepEqual(before.Nodes, after.Nodes) || len(w.dependsOn(dependent)) != 0 || !eligible(dependent) {
			t.Fatal("dependency removal changed nodes, wrong edge, or did not restore eligibility")
		}
		// Readding the same logical dependency gives a new edge. Replaying the
		// earlier removal must return its original result without deleting it.
		newEdge := w.must(w.s.AddDependency(w.ctx, w.next(), p)).Data["edge_id"].(string)
		committed := w.snapshot()
		if replay := w.must(w.s.RemoveDependency(w.ctx, c, p)); !reflect.DeepEqual(replay, removed) {
			t.Fatal("dependency replay changed")
		}
		assertBoundaryUnchanged(t, w, committed)
		if edges := w.dependsOn(dependent); len(edges) != 1 || edges[0].ID != newEdge {
			t.Fatal("replay removed the replacement edge")
		}
		w.must(w.s.RemoveDependency(w.ctx, w.next(), p))
		rejectHostMutation(t, w, "not_found", func(c graph.CommandContext) (graph.Result, error) { return w.s.RemoveDependency(w.ctx, c, p) })
		w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: p.Dependent, State: "running", ExecutionID: graph.NewID()}))
		running := p
		running.Dependent.Revision = 2
		rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) { return w.s.RemoveDependency(w.ctx, c, running) })
		w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: dependent, Revision: 2}, State: "paused"}))
		q := p
		q.Dependent.Revision = 3
		rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) { return w.s.RemoveDependency(w.ctx, c, q) })
		w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: q.Dependent, State: "open"}))
		q.Dependent.Revision = 4
		w.must(w.s.AddDependency(w.ctx, w.next(), q))
		w.must(w.s.RemoveDependency(w.ctx, w.next(), q))
		w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: q.Dependent, State: "running", ExecutionID: graph.NewID()}))
		w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: dependent, Revision: 5}, State: "done"}))
		q.Dependent.Revision = 6
		rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) { return w.s.RemoveDependency(w.ctx, c, q) })
	})
	t.Run("HostConsumedHintsAndRetractedEvidenceStayImmutable", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Terminal nodes")
		hint := w.must(w.s.AddHint(w.ctx, w.next(), graph.AddHintInput{Title: "Hint", HintPayload: graph.HintPayload{Instruction: "Inspect"}})).Data["hint_id"].(string)
		_, wc := w.running("Probe", 1)
		oid := w.observe(wc, "Observed", "Measured")
		for _, tc := range []struct {
			id     string
			action func(context.Context, graph.CommandContext, graph.NodeActionInput) (graph.Result, error)
			state  string
		}{{hint, w.s.ConsumeHint, "consumed"}, {oid, w.s.RetractEvidence, "retracted"}} {
			old := w.node(tc.id)
			p := graph.NodeActionInput{Node: graph.Ref{NodeID: tc.id, Revision: 1}, Reason: "Host reviewed"}
			c := w.next()
			r := w.must(tc.action(w.ctx, c, p))
			node := w.node(tc.id)
			if node.Revision.State != tc.state || node.Revision.Revision != 2 || node.Revision.Title != old.Revision.Title || string(node.Revision.Payload) != string(old.Revision.Payload) {
				t.Fatal("terminal action changed immutable content")
			}
			historical, e := w.s.GetRevision(w.ctx, w.c.Namespace, w.c.GraphID, graph.Ref{NodeID: tc.id, Revision: 1})
			if e != nil || !reflect.DeepEqual(historical.Revision, old.Revision) {
				t.Fatalf("old revision changed: %#v %v", historical, e)
			}
			before := w.snapshot()
			if replay := w.must(tc.action(w.ctx, c, p)); !reflect.DeepEqual(replay, r) {
				t.Fatal("terminal action retry changed")
			}
			assertBoundaryUnchanged(t, w, before)
			wrongActor := c
			wrongActor.ActorID = "different-host-actor"
			if _, e := tc.action(w.ctx, wrongActor, p); graph.ErrorCode(e) != "idempotency_conflict" {
				t.Fatalf("wrong-context replay: %v", e)
			}
			assertBoundaryUnchanged(t, w, before)
			p.Node.Revision = 2
			rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) { return tc.action(w.ctx, c, p) })
			rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) {
				return w.s.ReviseNode(w.ctx, c, graph.ReviseNodeInput{Node: p.Node, Title: "Reactivate", Payload: node.Revision.Payload})
			})
		}
		origin := ""
		for _, n := range w.snapshot().Nodes {
			if n.Kind == "origin" {
				origin = n.ID
			}
		}
		rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) {
			return w.s.ConsumeHint(w.ctx, c, graph.NodeActionInput{Node: graph.Ref{NodeID: origin, Revision: 1}})
		})
		rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) {
			return w.s.ReviseNode(w.ctx, c, graph.ReviseNodeInput{Node: graph.Ref{NodeID: origin, Revision: 1}, Title: "Changed origin", Payload: json.RawMessage(`{"request":"Changed"}`)})
		})
	})
}

func runHostGraphLifecycle(t *testing.T, newStore func(t *testing.T) graph.Store) {
	t.Run("HostGraphLifecycleAndExpectedRevisionReplay", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Graph lifecycle")
		rejectHostMutation(t, w, "already_exists", func(c graph.CommandContext) (graph.Result, error) {
			return w.s.CreateGraph(w.ctx, c, graph.CreateGraphInput{Title: "Duplicate", Request: "Duplicate"})
		})
		expected := w.snapshot().Graph.Revision
		c := w.next()
		c.ExpectedGraphRevision = &expected
		p := graph.AddHintInput{Title: "Optimistic write", HintPayload: graph.HintPayload{Instruction: "Review"}}
		r := w.must(w.s.AddHint(w.ctx, c, p))
		before := w.snapshot()
		if replay := w.must(w.s.AddHint(w.ctx, c, p)); !reflect.DeepEqual(replay, r) {
			t.Fatal("committed replay rejected its now-stale expected graph revision")
		}
		assertBoundaryUnchanged(t, w, before)
		rejected := rejectHostMutation(t, w, "revision_conflict", func(c graph.CommandContext) (graph.Result, error) {
			c.ExpectedGraphRevision = &expected
			return w.s.AddHint(w.ctx, c, p)
		})
		current := before.Graph.Revision
		rejected.ExpectedGraphRevision = &current
		w.must(w.s.AddHint(w.ctx, rejected, p))
		work, _ := w.running("Running", 1)
		rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) {
			return w.s.SetGraphState(w.ctx, c, graph.SetGraphStateInput{State: "closed"})
		})
		w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: work, Revision: 2}, State: "done"}))
		w.must(w.s.SetGraphState(w.ctx, w.next(), graph.SetGraphStateInput{State: "closed"}))
		rejectHostMutation(t, w, "graph_read_only", func(c graph.CommandContext) (graph.Result, error) { return w.s.AddHint(w.ctx, c, p) })
		rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) {
			return w.s.SetGraphState(w.ctx, c, graph.SetGraphStateInput{State: "closed"})
		})
		w.must(w.s.SetGraphState(w.ctx, w.next(), graph.SetGraphStateInput{State: "active"}))
		w.must(w.s.AddHint(w.ctx, w.next(), p))
		rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) {
			return w.s.SetGraphState(w.ctx, c, graph.SetGraphStateInput{State: "archived"})
		})
		w.must(w.s.SetGraphState(w.ctx, w.next(), graph.SetGraphStateInput{State: "closed"}))
		w.must(w.s.SetGraphState(w.ctx, w.next(), graph.SetGraphStateInput{State: "archived"}))
		rejectHostMutation(t, w, "graph_read_only", func(c graph.CommandContext) (graph.Result, error) { return w.s.AddHint(w.ctx, c, p) })
		for _, state := range []string{"active", "closed", "archived"} {
			rejectHostMutation(t, w, "invalid_transition", func(c graph.CommandContext) (graph.Result, error) {
				return w.s.SetGraphState(w.ctx, c, graph.SetGraphStateInput{State: state})
			})
		}
		if _, e := w.s.ListNodes(w.ctx, w.c.Namespace, w.c.GraphID, graph.ListOptions{}); e != nil {
			t.Fatalf("archived graph not readable: %v", e)
		}
		if replay := w.must(w.s.AddHint(w.ctx, c, p)); !reflect.DeepEqual(replay, r) {
			t.Fatal("archived graph lost a committed retry")
		}
	})
}

func rejectHostMutation(t *testing.T, w *world, code string, call func(graph.CommandContext) (graph.Result, error)) graph.CommandContext {
	t.Helper()
	before := w.snapshot()
	c := w.next()
	if _, e := call(c); graph.ErrorCode(e) != code {
		t.Fatalf("want %s, got %v", code, e)
	}
	assertBoundaryUnchanged(t, w, before)
	return c
}
