package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"ns-exploration-graph/graph"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// wire independently encodes the complete tool result envelope measured against budgets.
func wire(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	var data map[string]json.RawMessage
	if e = json.Unmarshal(b, &data); e != nil {
		t.Fatal(e)
	}
	rev := data["graph_revision"]
	delete(data, "graph_revision")
	b, e = json.Marshal(map[string]any{"ok": true, "graph_revision": rev, "data": data})
	if e != nil {
		t.Fatal(e)
	}
	return b
}

type world struct {
	t   *testing.T
	s   *graph.Service
	ctx context.Context
	c   graph.CommandContext
}

func newWorld(t *testing.T, store graph.Store, title string) *world {
	w := &world{t, graph.NewService(store), context.Background(), graph.CommandContext{Namespace: "bounded", GraphID: graph.NewID(), ActorID: "tester"}}
	w.must(w.s.CreateGraph(w.ctx, w.next(), graph.CreateGraphInput{Title: title, Request: "Investigate the parser: «résumé» 解析器 🙂 <&>"}))
	return w
}
func (w *world) next() graph.CommandContext { c := w.c; c.RequestID = graph.NewID(); return c }
func (w *world) must(r graph.Result, e error) graph.Result {
	w.t.Helper()
	if e != nil {
		w.t.Fatal(e)
	}
	return r
}
func (w *world) running(title string, priority int) (string, graph.CommandContext) {
	w.t.Helper()
	id := w.must(w.s.ProposeWork(w.ctx, w.next(), graph.ProposeWorkInput{Title: title, WorkPayload: graph.WorkPayload{Instruction: "Run " + title, Priority: priority}})).Data["work_item_id"].(string)
	x := graph.NewID()
	w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: id, Revision: 1}, State: "running", ExecutionID: x}))
	wc := w.next()
	wc.Execution = &graph.Execution{WorkItemID: id, ExecutionID: x}
	return id, wc
}
func (w *world) observe(wc graph.CommandContext, title, statement string, resources ...graph.ResourceInput) string {
	w.t.Helper()
	wc.RequestID = graph.NewID()
	return w.must(w.s.RecordObservation(w.ctx, wc, graph.RecordObservationInput{Title: title, EvidencePayload: graph.EvidencePayload{Statement: statement, Evidence: "log " + title, EpistemicStatus: "observed", Polarity: "negative"}, Resources: resources})).Data["observation_id"].(string)
}
func (w *world) node(id string) graph.NodeView {
	w.t.Helper()
	v, e := w.s.GetNode(w.ctx, w.c.Namespace, w.c.GraphID, graph.GetNodeOptions{NodeID: id})
	if e != nil {
		w.t.Fatal(e)
	}
	return v
}
func requiredBytes(t *testing.T, e error) int {
	t.Helper()
	ge, ok := e.(*graph.Error)
	if !ok || ge.Code != "budget_too_small" {
		t.Fatalf("expected budget_too_small, got %v", e)
	}
	switch v := ge.Details["required_bytes"].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	t.Fatalf("missing required_bytes: %#v", ge.Details)
	return 0
}

// fullOnly hides optional store extensions so results can be compared with the
// complete-aggregate Read/Update path.
type fullOnly struct{ s graph.Store }

func (f fullOnly) Update(ctx context.Context, ns, id string, create *graph.Graph, fn func(*graph.State) error) error {
	return f.s.Update(ctx, ns, id, create, fn)
}
func (f fullOnly) Read(ctx context.Context, ns, id string) (*graph.State, error) {
	return f.s.Read(ctx, ns, id)
}
func (f fullOnly) EventsAfter(ctx context.Context, ns, id string, after int64, limit int) ([]graph.Event, int64, error) {
	return f.s.(graph.EventStore).EventsAfter(ctx, ns, id, after, limit)
}

func runBounded(t *testing.T, newStore func(t *testing.T) graph.Store) {
	t.Run("ProjectionUTF8BudgetAnchorsAndRetrieval", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Origin «résumé» 🙂")
		criteria := strings.Repeat("Critère accepté 検証済み 🙂 <&> ", 14)
		goals := map[string]bool{}
		for i := 0; i < 2; i++ {
			goals[w.must(w.s.AddGoal(w.ctx, w.next(), graph.AddGoalInput{Title: fmt.Sprintf("Goal %d «目標»", i), GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{criteria}}})).Data["goal_id"].(string)] = true
		}
		abandoned := w.must(w.s.AddGoal(w.ctx, w.next(), graph.AddGoalInput{Title: "Abandoned", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"none"}}})).Data["goal_id"].(string)
		w.must(w.s.AbandonGoal(w.ctx, w.next(), graph.NodeActionInput{Node: graph.Ref{NodeID: abandoned, Revision: 1}, Reason: "out of scope"}))
		assigned, wc := w.running("Assigned «作業»", 10)
		statements := map[string]string{}
		for i := 0; i < 15; i++ {
			s := fmt.Sprintf("Observation %d: ", i) + strings.Repeat("négatif 失敗 🙂 <&> ", 3+i%5)
			statements[w.observe(wc, fmt.Sprintf("Obs %d", i), s)] = s
		}
		for i := 0; i < 3; i++ {
			w.must(w.s.AddHint(w.ctx, w.next(), graph.AddHintInput{Title: "Hint «ヒント»", HintPayload: graph.HintPayload{Instruction: strings.Repeat("Prüfe 🙂 ", 5)}}))
		}
		for i := 0; i < 5; i++ {
			w.must(w.s.ProposeWork(w.ctx, w.next(), graph.ProposeWorkInput{Title: fmt.Sprintf("Queued %d", i), WorkPayload: graph.WorkPayload{Instruction: "Queued ✓", Priority: i * 10}}))
		}
		project := func(budget int) (graph.Projection, error) {
			return w.s.ProjectContext(w.ctx, w.c.Namespace, w.c.GraphID, graph.ProjectionOptions{ByteBudget: budget, AssignedWorkItemID: assigned})
		}
		_, e := project(1024)
		mandatory := requiredBytes(t, e)
		if mandatory <= 1025 {
			t.Fatalf("fixture anchors too small to test boundary: %d", mandatory)
		}
		if _, e = project(mandatory - 1); requiredBytes(t, e) != mandatory {
			t.Fatal("required_bytes changed with budget")
		}
		anchors, e := project(mandatory)
		if e != nil {
			t.Fatalf("anchors rejected at their exact size: %v", e)
		}
		if got := len(wire(t, anchors)); got > mandatory {
			t.Fatalf("anchor projection %d bytes exceeds %d", got, mandatory)
		}
		full, e := project(64 * 1024)
		if e != nil {
			t.Fatal(e)
		}
		fullBytes := wire(t, full)
		if utf8.RuneCount(fullBytes) >= len(fullBytes) {
			t.Fatal("fixture must contain multi-byte UTF-8")
		}
		if len(full.Evidence) != 15 || len(full.Hints) != 3 || len(full.Work) != 6 {
			t.Fatalf("full projection omitted candidates: evidence=%d hints=%d work=%d", len(full.Evidence), len(full.Hints), len(full.Work))
		}
		exact, e := project(len(fullBytes))
		if e != nil || !reflect.DeepEqual(wire(t, exact), fullBytes) {
			t.Fatalf("projection at its exact UTF-8 size changed: %v", e)
		}
		under, e := project(len(fullBytes) - 1)
		if e != nil || len(wire(t, under)) > len(fullBytes)-1 || reflect.DeepEqual(wire(t, under), fullBytes) {
			t.Fatalf("one byte under full size did not omit content: %v", e)
		}
		for budget := mandatory; budget <= len(fullBytes)+64; budget += 97 {
			p, e := project(budget)
			if e != nil {
				t.Fatalf("budget %d: %v", budget, e)
			}
			if n := len(wire(t, p)); n > budget {
				t.Fatalf("budget %d exceeded by UTF-8 result of %d bytes", budget, n)
			}
			got := map[string]bool{}
			for _, g := range p.Goals {
				got[g.ID] = true
				if g.ID == abandoned {
					t.Fatal("abandoned goal projected as anchor")
				}
			}
			if !reflect.DeepEqual(got, goals) || len(p.Work) == 0 || p.Work[0].ID != assigned || p.Origin.ID == "" {
				t.Fatalf("budget %d dropped anchors", budget)
			}
			counts := map[string]int{"origin": 1, "goal": len(p.Goals), "work_item": len(p.Work), "hint": len(p.Hints), "edges": len(p.Edges)}
			for _, n := range p.Evidence {
				counts[n.Kind]++
			}
			truncated := false
			for k, total := range p.Totals {
				if p.Omitted[k] != total-counts[k] {
					t.Fatalf("budget %d omitted %s=%d, want %d", budget, k, p.Omitted[k], total-counts[k])
				}
				truncated = truncated || p.Omitted[k] > 0
			}
			if p.Truncated != truncated || p.Retrieval.GetNode == "" || p.Retrieval.ListNodes == "" || p.Retrieval.Snapshot == "" {
				t.Fatalf("budget %d truncation/retrieval metadata invalid", budget)
			}
		}
		if anchors.Totals["observation"] != 15 || anchors.Omitted["observation"] != 15 || anchors.Totals["goal"] != 3 {
			t.Fatalf("totals %#v omitted %#v", anchors.Totals, anchors.Omitted)
		}
		included := map[string]bool{}
		for _, n := range anchors.Evidence {
			included[n.ID] = true
		}
		listed := map[string]bool{}
		cursor := ""
		for {
			page, e := w.s.ListNodes(w.ctx, w.c.Namespace, w.c.GraphID, graph.ListOptions{Kind: "observation", Limit: 4, Cursor: cursor})
			if e != nil {
				t.Fatal(e)
			}
			for _, n := range page.Nodes {
				listed[n.ID] = true
			}
			if cursor = page.NextCursor; cursor == "" {
				break
			}
		}
		if len(listed) != anchors.Totals["observation"] {
			t.Fatalf("listed %d observations, projection total %d", len(listed), anchors.Totals["observation"])
		}
		for id := range listed {
			if included[id] {
				continue
			}
			var p graph.EvidencePayload
			if e := json.Unmarshal(w.node(id).Revision.Payload, &p); e != nil || p.Statement != statements[id] {
				t.Fatalf("omitted evidence %s not retrievable intact: %v", id, e)
			}
		}
	})
	t.Run("CursorsCoverOnceAndRejectAfterMutation", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Cursor origin")
		for i := 0; i < 7; i++ {
			w.must(w.s.AddHint(w.ctx, w.next(), graph.AddHintInput{Title: fmt.Sprintf("Hint %d", i), HintPayload: graph.HintPayload{Instruction: "page"}}))
			w.must(w.s.ProposeWork(w.ctx, w.next(), graph.ProposeWorkInput{Title: fmt.Sprintf("Work %d", i), WorkPayload: graph.WorkPayload{Instruction: "page"}}))
		}
		seen := map[string]int{}
		cursor, total := "", 0
		for pages := 0; ; pages++ {
			page, e := w.s.ListNodes(w.ctx, w.c.Namespace, w.c.GraphID, graph.ListOptions{Limit: 3, Cursor: cursor})
			if e != nil {
				t.Fatal(e)
			}
			total = page.Total
			if page.Omitted != page.Total-len(page.Nodes) {
				t.Fatalf("page omitted %d of %d with %d nodes", page.Omitted, page.Total, len(page.Nodes))
			}
			for _, n := range page.Nodes {
				seen[n.ID]++
			}
			if cursor = page.NextCursor; cursor == "" {
				break
			}
			if pages > 20 {
				t.Fatal("pagination did not terminate")
			}
		}
		if total != 15 || len(seen) != total {
			t.Fatalf("pagination covered %d of %d", len(seen), total)
		}
		for id, n := range seen {
			if n != 1 {
				t.Fatalf("node %s returned %d times", id, n)
			}
		}
		page, e := w.s.ListNodes(w.ctx, w.c.Namespace, w.c.GraphID, graph.ListOptions{Limit: 3})
		if e != nil {
			t.Fatal(e)
		}
		origin := ""
		for _, n := range mustList(t, w, "origin").Nodes {
			origin = n.ID
		}
		walk, e := w.s.Descendants(w.ctx, w.c.Namespace, w.c.GraphID, graph.TraversalOptions{NodeID: origin, Relations: []string{"derived_from"}, Depth: 1, Limit: 2})
		if e != nil || walk.Total != 7 || walk.NextCursor == "" {
			t.Fatalf("traversal page: %#v %v", walk, e)
		}
		full := w.node(origin)
		related, e := w.s.GetNode(w.ctx, w.c.Namespace, w.c.GraphID, graph.GetNodeOptions{NodeID: origin, ByteBudget: len(wire(t, full)) - 1})
		if e != nil || related.NextRelatedCursor == "" {
			t.Fatalf("related page: %v", e)
		}
		w.must(w.s.AddHint(w.ctx, w.next(), graph.AddHintInput{Title: "Concurrent change", HintPayload: graph.HintPayload{Instruction: "mutate"}}))
		if _, e = w.s.ListNodes(w.ctx, w.c.Namespace, w.c.GraphID, graph.ListOptions{Limit: 3, Cursor: page.NextCursor}); graph.ErrorCode(e) != "stale_cursor" {
			t.Fatalf("list cursor after mutation: %v", e)
		}
		if _, e = w.s.Descendants(w.ctx, w.c.Namespace, w.c.GraphID, graph.TraversalOptions{NodeID: origin, Relations: []string{"derived_from"}, Depth: 1, Limit: 2, Cursor: walk.NextCursor}); graph.ErrorCode(e) != "stale_cursor" {
			t.Fatalf("traversal cursor after mutation: %v", e)
		}
		if _, e = w.s.GetNode(w.ctx, w.c.Namespace, w.c.GraphID, graph.GetNodeOptions{NodeID: origin, RelatedCursor: related.NextRelatedCursor}); graph.ErrorCode(e) != "stale_cursor" {
			t.Fatalf("related cursor after mutation: %v", e)
		}
		restarted, e := w.s.ListNodes(w.ctx, w.c.Namespace, w.c.GraphID, graph.ListOptions{Limit: 3})
		if e != nil || restarted.Total != total+1 {
			t.Fatalf("restarted query total %d: %v", restarted.Total, e)
		}
	})
	t.Run("EventsResumeAcrossReconnectAndConcurrentWrites", func(t *testing.T) {
		store := newStore(t)
		w := newWorld(t, store, "Events origin")
		consumed := map[int64]int{}
		last := int64(0)
		poll := func(s *graph.Service) {
			t.Helper()
			page, e := s.EventsAfter(w.ctx, w.c.Namespace, w.c.GraphID, last, 3)
			if e != nil {
				t.Fatal(e)
			}
			for _, ev := range page.Events {
				if ev.Revision <= last {
					t.Fatalf("event %d not after %d", ev.Revision, last)
				}
				consumed[ev.Revision]++
				last = ev.Revision
			}
			if page.NextRevision != last || page.GraphRevision < last {
				t.Fatalf("page next=%d graph=%d last=%d", page.NextRevision, page.GraphRevision, last)
			}
		}
		poll(w.s)
		var wg sync.WaitGroup
		errs := make(chan error, 20)
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					_, e := w.s.AddHint(w.ctx, w.next(), graph.AddHintInput{Title: "Concurrent", HintPayload: graph.HintPayload{Instruction: "write"}})
					errs <- e
				}
			}()
		}
		for i := 0; i < 4; i++ {
			poll(w.s)
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		if _, e := w.s.ProposeWork(w.ctx, w.next(), graph.ProposeWorkInput{Title: "Invalid", WorkPayload: graph.WorkPayload{Instruction: "x"}, Parents: []graph.Ref{{NodeID: graph.NewID(), Revision: 1}}}); graph.ErrorCode(e) != "not_found" {
			t.Fatalf("expected failed write, got %v", e)
		}
		reconnected := graph.NewService(store)
		final := int64(0)
		for i := 0; i < 50; i++ {
			snap, e := reconnected.Snapshot(w.ctx, w.c.Namespace, w.c.GraphID)
			if e != nil {
				t.Fatal(e)
			}
			final = snap.Graph.Revision
			if last == final {
				break
			}
			poll(reconnected)
		}
		if final != 21 || len(consumed) != int(final) {
			t.Fatalf("consumed %d events for revision %d", len(consumed), final)
		}
		for r := int64(1); r <= final; r++ {
			if consumed[r] != 1 {
				t.Fatalf("revision %d consumed %d times", r, consumed[r])
			}
		}
		empty, e := reconnected.EventsAfter(w.ctx, w.c.Namespace, w.c.GraphID, final, 10)
		if e != nil || len(empty.Events) != 0 || empty.NextRevision != final {
			t.Fatalf("caught-up feed: %#v %v", empty, e)
		}
	})
	t.Run("SnapshotRestoreRoundTripAndRejection", func(t *testing.T) {
		store := newStore(t)
		w := newWorld(t, store, "Restore origin «復元»")
		gid := w.must(w.s.AddGoal(w.ctx, w.next(), graph.AddGoalInput{Title: "Parser fixed", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Regression passes"}}})).Data["goal_id"].(string)
		wid, wc := w.running("Reproduce", 50)
		oid := w.observe(wc, "First run", "Parser fails", graph.ResourceInput{ResourceType: "log", URI: "urn:log:first", SHA256: strings.Repeat("a", 64)})
		accept := func(source graph.Ref, goalRevision int) string {
			edge := w.must(w.s.AddSupport(w.ctx, w.next(), graph.AddSupportInput{Source: source, Target: graph.Ref{NodeID: gid, Revision: goalRevision}})).Data["edge_id"].(string)
			return w.must(w.s.AcceptGoal(w.ctx, w.next(), graph.AcceptGoalInput{Goal: graph.Ref{NodeID: gid, Revision: goalRevision}, SupportEdgeIDs: []string{edge}, Method: "validator", Rationale: "Checked"})).Data["acceptance_id"].(string)
		}
		cid := w.must(w.s.RecordConclusion(w.ctx, w.next(), graph.RecordConclusionInput{Title: "Cause", EvidencePayload: graph.EvidencePayload{Statement: "Tokenizer bug", EpistemicStatus: "inferred"}, Supports: []graph.Ref{{NodeID: oid, Revision: 1}}})).Data["conclusion_id"].(string)
		first := accept(graph.Ref{NodeID: cid, Revision: 1}, 1)
		w.must(w.s.ReviseNode(w.ctx, w.next(), graph.ReviseNodeInput{Node: graph.Ref{NodeID: oid, Revision: 1}, Title: "First run corrected", Payload: json.RawMessage(`{"statement":"Parser fails on BOM","evidence":"log v2","epistemic_status":"observed"}`)}))
		second := accept(graph.Ref{NodeID: oid, Revision: 2}, 3)
		hint := w.must(w.s.AddHint(w.ctx, w.next(), graph.AddHintInput{Title: "Hint", HintPayload: graph.HintPayload{Instruction: "Check BOM"}})).Data["hint_id"].(string)
		w.must(w.s.ConsumeHint(w.ctx, w.next(), graph.NodeActionInput{Node: graph.Ref{NodeID: hint, Revision: 1}}))
		dest := w.next()
		dest.Namespace = "restored"
		dest.GraphID = graph.NewID()
		absent := func(c graph.CommandContext) {
			t.Helper()
			if _, e := w.s.Snapshot(w.ctx, c.Namespace, c.GraphID); graph.ErrorCode(e) != "not_found" {
				t.Fatalf("rejected restore left graph state: %v", e)
			}
			if _, e := w.s.EventsAfter(w.ctx, c.Namespace, c.GraphID, 0, 10); graph.ErrorCode(e) != "not_found" {
				t.Fatalf("rejected restore left events: %v", e)
			}
		}
		running, e := w.s.ExportSnapshot(w.ctx, w.c.Namespace, w.c.GraphID)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.s.ImportSnapshot(w.ctx, dest, running); graph.ErrorCode(e) != "invalid_argument" {
			t.Fatalf("running work restored: %v", e)
		}
		absent(dest)
		w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 2}, State: "done"}))
		exported, e := w.s.ExportSnapshot(w.ctx, w.c.Namespace, w.c.GraphID)
		if e != nil {
			t.Fatal(e)
		}
		again, e := w.s.ExportSnapshot(w.ctx, w.c.Namespace, w.c.GraphID)
		if e != nil || string(exported) != string(again) {
			t.Fatalf("export is not repeatable: %v", e)
		}
		var source graph.Snapshot
		if e = json.Unmarshal(exported, &source); e != nil {
			t.Fatal(e)
		}
		mutations := map[string]func(*graph.Snapshot){
			"unsupported version": func(s *graph.Snapshot) { s.SchemaVersion = 2 },
			"foreign reference":   func(s *graph.Snapshot) { s.Edges[len(s.Edges)-1].TargetID = graph.NewID() },
			"acceptance closure": func(s *graph.Snapshot) {
				for _, ed := range s.Edges {
					if ed.Relation == "spawns" {
						s.Acceptances[0].SupportEdgeIDs = []string{ed.ID}
					}
				}
			},
			"running work": func(s *graph.Snapshot) {
				for i := range s.Nodes {
					if s.Nodes[i].ID == wid {
						s.Nodes[i].Revisions = s.Nodes[i].Revisions[:2]
						s.Nodes[i].CurrentRevision = 2
					}
				}
			},
		}
		names := []string{}
		for name := range mutations {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			var bad graph.Snapshot
			_ = json.Unmarshal(exported, &bad)
			mutations[name](&bad)
			encoded, _ := json.Marshal(bad)
			if _, e = w.s.ImportSnapshot(w.ctx, dest, encoded); graph.ErrorCode(e) != "invalid_argument" {
				t.Fatalf("%s accepted: %v", name, e)
			}
			absent(dest)
		}
		restored := w.must(w.s.ImportSnapshot(w.ctx, dest, exported))
		if restored.GraphRevision != 1 {
			t.Fatalf("destination revision %d", restored.GraphRevision)
		}
		if replay := w.must(w.s.ImportSnapshot(w.ctx, dest, exported)); !reflect.DeepEqual(replay, restored) {
			t.Fatal("restore replay changed result")
		}
		copied, e := w.s.ExportSnapshot(w.ctx, dest.Namespace, dest.GraphID)
		if e != nil {
			t.Fatal(e)
		}
		var target graph.Snapshot
		if e = json.Unmarshal(copied, &target); e != nil {
			t.Fatal(e)
		}
		for name, pair := range map[string][2]any{"nodes": {source.Nodes, target.Nodes}, "edges": {source.Edges, target.Edges}, "resources": {source.ResourceRefs, target.ResourceRefs}, "acceptances": {source.Acceptances, target.Acceptances}} {
			a, _ := json.Marshal(pair[0])
			b, _ := json.Marshal(pair[1])
			if string(a) != string(b) {
				t.Fatalf("restore changed %s", name)
			}
		}
		meta, _ := target.Graph.Metadata["import_source"].(map[string]any)
		if meta["graph_id"] != w.c.GraphID || fmt.Sprint(meta["revision"]) != fmt.Sprint(source.Graph.Revision) || target.Graph.Revision != 1 || target.Graph.State != "active" {
			t.Fatalf("restore metadata %#v", target.Graph)
		}
		events, e := w.s.EventsAfter(w.ctx, dest.Namespace, dest.GraphID, 0, 10)
		if e != nil || len(events.Events) != 1 || events.Events[0].CommandKind != "ImportSnapshot" || events.Events[0].Revision != 1 {
			t.Fatalf("restore events %#v %v", events, e)
		}
		explained, e := w.s.ExplainGoal(w.ctx, dest.Namespace, dest.GraphID, gid)
		if e != nil || len(explained.Acceptances) != 2 || explained.Goal.CurrentRevision != 4 || len(explained.Goal.Revisions) != 4 {
			t.Fatalf("restored goal history %#v %v", explained, e)
		}
		states := map[string]string{}
		for _, a := range explained.Acceptances {
			states[a.ID] = a.State
		}
		if states[first] != "stale" || states[second] != "current" {
			t.Fatalf("restored acceptance states %#v", states)
		}
		restoredWork := dest
		restoredWork.RequestID = graph.NewID()
		if _, e = w.s.ReviseNode(w.ctx, restoredWork, graph.ReviseNodeInput{Node: graph.Ref{NodeID: oid, Revision: 2}, Title: "Restored revision", Payload: json.RawMessage(`{"statement":"Changed after restore","evidence":"log v3","epistemic_status":"observed"}`)}); e != nil {
			t.Fatal(e)
		}
		goal, e := w.s.GetNode(w.ctx, dest.Namespace, dest.GraphID, graph.GetNodeOptions{NodeID: gid})
		if e != nil || goal.Revision.State != "needs_review" {
			t.Fatalf("restored acceptance was not invalidated: %v", e)
		}
		if original := w.node(gid); original.Revision.State != "met" {
			t.Fatal("restored graph mutation leaked into source")
		}
	})
	t.Run("SnapshotRestoreUsesStoredClosureNotClock", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Clock origin")
		gid := w.must(w.s.AddGoal(w.ctx, w.next(), graph.AddGoalInput{Title: "Explained", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Cause known"}}})).Data["goal_id"].(string)
		wid, wc := w.running("Probe", 20)
		o1 := w.observe(wc, "Reviewed", "Fails", graph.ResourceInput{ResourceType: "log", URI: "urn:log:reviewed"})
		o2 := w.observe(wc, "Later", "Also fails")
		cid := w.must(w.s.RecordConclusion(w.ctx, w.next(), graph.RecordConclusionInput{Title: "Cause", EvidencePayload: graph.EvidencePayload{Statement: "Bug", EpistemicStatus: "inferred"}, Supports: []graph.Ref{{NodeID: o1, Revision: 1}}})).Data["conclusion_id"].(string)
		direct := w.must(w.s.AddSupport(w.ctx, w.next(), graph.AddSupportInput{Source: graph.Ref{NodeID: cid, Revision: 1}, Target: graph.Ref{NodeID: gid, Revision: 1}})).Data["edge_id"].(string)
		w.must(w.s.AcceptGoal(w.ctx, w.next(), graph.AcceptGoalInput{Goal: graph.Ref{NodeID: gid, Revision: 1}, SupportEdgeIDs: []string{direct}, Method: "validator", Rationale: "Checked"}))
		late := w.must(w.s.AddSupport(w.ctx, w.next(), graph.AddSupportInput{Source: graph.Ref{NodeID: o2, Revision: 1}, Target: graph.Ref{NodeID: cid, Revision: 1}})).Data["edge_id"].(string)
		lateResource := w.must(w.s.AttachResource(w.ctx, w.next(), graph.AttachResourceInput{Node: graph.Ref{NodeID: o1, Revision: 1}, Resource: graph.ResourceInput{ResourceType: "log", URI: "urn:log:later"}})).Data["resource_reference_id"].(string)
		w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 2}, State: "done"}))
		exported, e := w.s.ExportSnapshot(w.ctx, w.c.Namespace, w.c.GraphID)
		if e != nil {
			t.Fatal(e)
		}
		var source graph.Snapshot
		if e = json.Unmarshal(exported, &source); e != nil {
			t.Fatal(e)
		}
		accepted := source.Acceptances[0]
		if len(accepted.SupportEdgeIDs) != 2 || len(accepted.ReviewedResourceIDs) != 1 {
			t.Fatalf("fixture closure %#v", accepted)
		}
		for _, id := range append(accepted.SupportEdgeIDs, accepted.ReviewedResourceIDs...) {
			if id == late || id == lateResource {
				t.Fatal("later attachment entered the acceptance")
			}
		}
		restore := func(s graph.Snapshot) (graph.CommandContext, error) {
			dest := w.next()
			dest.Namespace, dest.GraphID = "clock", graph.NewID()
			encoded, _ := json.Marshal(s)
			_, e := w.s.ImportSnapshot(w.ctx, dest, encoded)
			return dest, e
		}
		check := func(name string, s graph.Snapshot) {
			t.Helper()
			dest, e := restore(s)
			if e != nil {
				t.Fatalf("%s rejected: %v", name, e)
			}
			copied, e := w.s.Snapshot(w.ctx, dest.Namespace, dest.GraphID)
			if e != nil || len(copied.Acceptances) != 1 {
				t.Fatalf("%s restored acceptances: %v", name, e)
			}
			a, _ := json.Marshal(s.Acceptances[0])
			b, _ := json.Marshal(copied.Acceptances[0])
			if string(a) != string(b) {
				t.Fatalf("%s changed the stored closure\n%s\n%s", name, a, b)
			}
			explained, e := w.s.ExplainGoal(w.ctx, dest.Namespace, dest.GraphID, gid)
			if e != nil || len(explained.Acceptances) != 1 || explained.Acceptances[0].State != "current" {
				t.Fatalf("%s restored explanation %#v %v", name, explained, e)
			}
		}
		check("later support and resource", source)
		var skewed graph.Snapshot
		_ = json.Unmarshal(exported, &skewed)
		goalRevisions := map[int]time.Time{}
		for _, n := range skewed.Nodes {
			if n.ID == gid {
				for _, r := range n.Revisions {
					goalRevisions[r.Revision] = r.CreatedAt
				}
			}
		}
		for i := range skewed.Edges {
			if skewed.Edges[i].ID == direct {
				skewed.Edges[i].CreatedAt = goalRevisions[1].Add(-time.Hour)
			}
		}
		skewed.Acceptances[0].CreatedAt = goalRevisions[2].Add(-time.Hour)
		check("skewed clocks", skewed)
		for _, drop := range accepted.SupportEdgeIDs {
			var bad graph.Snapshot
			_ = json.Unmarshal(exported, &bad)
			kept := []string{}
			for _, id := range accepted.SupportEdgeIDs {
				if id != drop {
					kept = append(kept, id)
				}
			}
			bad.Acceptances[0].SupportEdgeIDs = kept
			dest, e := restore(bad)
			if graph.ErrorCode(e) != "invalid_argument" {
				t.Fatalf("closure without %s accepted: %v", drop, e)
			}
			if _, e = w.s.Snapshot(w.ctx, dest.Namespace, dest.GraphID); graph.ErrorCode(e) != "not_found" {
				t.Fatalf("rejected restore left graph state: %v", e)
			}
			if _, e = w.s.EventsAfter(w.ctx, dest.Namespace, dest.GraphID, 0, 10); graph.ErrorCode(e) != "not_found" {
				t.Fatalf("rejected restore left events: %v", e)
			}
		}
	})
	t.Run("ProposeWorkParentKinds", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Parents origin")
		gid := w.must(w.s.AddGoal(w.ctx, w.next(), graph.AddGoalInput{Title: "Goal", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Done"}}})).Data["goal_id"].(string)
		hid := w.must(w.s.AddHint(w.ctx, w.next(), graph.AddHintInput{Title: "Hint", HintPayload: graph.HintPayload{Instruction: "Look"}})).Data["hint_id"].(string)
		wid, wc := w.running("Probe", 10)
		oid := w.observe(wc, "Seen", "Fails")
		cid := w.must(w.s.RecordConclusion(w.ctx, w.next(), graph.RecordConclusionInput{Title: "Cause", EvidencePayload: graph.EvidencePayload{Statement: "Bug", EpistemicStatus: "inferred"}, Supports: []graph.Ref{{NodeID: oid, Revision: 1}}})).Data["conclusion_id"].(string)
		w.must(w.s.RetractEvidence(w.ctx, w.next(), graph.NodeActionInput{Node: graph.Ref{NodeID: oid, Revision: 1}, Reason: "Bad log"}))
		propose := func(parent graph.Ref) error {
			_, e := w.s.ProposeWork(w.ctx, w.next(), graph.ProposeWorkInput{Title: "Child", WorkPayload: graph.WorkPayload{Instruction: "x"}, Parents: []graph.Ref{parent}})
			return e
		}
		for code, parents := range map[string][]graph.Ref{"invalid_relation": {{NodeID: gid, Revision: 1}, {NodeID: hid, Revision: 1}, {NodeID: wid, Revision: 2}}, "stale_evidence": {{NodeID: oid, Revision: 2}, {NodeID: cid, Revision: 1}}} {
			for _, p := range parents {
				if e := propose(p); graph.ErrorCode(e) != code {
					t.Fatalf("parent %s: want %s, got %v", p.NodeID, code, e)
				}
			}
		}
	})
	t.Run("ScopedReadsMatchCompleteAggregateReads", func(t *testing.T) {
		store := newStore(t)
		w := newWorld(t, store, "Scoped «範囲»")
		plain := graph.NewService(fullOnly{store})
		gid := w.must(w.s.AddGoal(w.ctx, w.next(), graph.AddGoalInput{Title: "Goal", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Done"}}})).Data["goal_id"].(string)
		wid, wc := w.running("Probe", 30)
		oid := w.observe(wc, "Observed", "Result 🙂", graph.ResourceInput{ResourceType: "log", URI: "urn:log:scoped"})
		cid := w.must(w.s.RecordConclusion(w.ctx, w.next(), graph.RecordConclusionInput{Title: "Concluded", EvidencePayload: graph.EvidencePayload{Statement: "Because", EpistemicStatus: "inferred"}, Supports: []graph.Ref{{NodeID: oid, Revision: 1}}})).Data["conclusion_id"].(string)
		edge := w.must(w.s.AddSupport(w.ctx, w.next(), graph.AddSupportInput{Source: graph.Ref{NodeID: cid, Revision: 1}, Target: graph.Ref{NodeID: gid, Revision: 1}})).Data["edge_id"].(string)
		acceptCtx := w.next()
		acceptInput := graph.AcceptGoalInput{Goal: graph.Ref{NodeID: gid, Revision: 1}, SupportEdgeIDs: []string{edge}, Method: "human", Rationale: "Reviewed"}
		accepted := w.must(w.s.AcceptGoal(w.ctx, acceptCtx, acceptInput))
		w.must(w.s.ReviseNode(w.ctx, w.next(), graph.ReviseNodeInput{Node: graph.Ref{NodeID: oid, Revision: 1}, Title: "Observed again", Payload: json.RawMessage(`{"statement":"Revised","evidence":"log","epistemic_status":"observed"}`)}))
		other := w.must(w.s.ProposeWork(w.ctx, w.next(), graph.ProposeWorkInput{Title: "Blocked", WorkPayload: graph.WorkPayload{Instruction: "wait"}})).Data["work_item_id"].(string)
		w.must(w.s.AddDependency(w.ctx, w.next(), graph.DependencyInput{Dependent: graph.Ref{NodeID: other, Revision: 1}, PrerequisiteID: wid}))
		w.must(w.s.ProposeWork(w.ctx, w.next(), graph.ProposeWorkInput{Title: "Free", WorkPayload: graph.WorkPayload{Instruction: "go"}}))
		same := func(name string, a, b any, ea, eb error) {
			t.Helper()
			if graph.ErrorCode(ea) != graph.ErrorCode(eb) || (ea == nil) != (eb == nil) {
				t.Fatalf("%s errors differ: %v / %v", name, ea, eb)
			}
			ja, _ := json.Marshal(a)
			jb, _ := json.Marshal(b)
			if string(ja) != string(jb) {
				t.Fatalf("%s differs:\n%s\n%s", name, ja, jb)
			}
		}
		ns, id := w.c.Namespace, w.c.GraphID
		for _, kind := range []string{"", "observation", "work_item"} {
			a, ea := w.s.ListNodes(w.ctx, ns, id, graph.ListOptions{Kind: kind, Limit: 3})
			b, eb := plain.ListNodes(w.ctx, ns, id, graph.ListOptions{Kind: kind, Limit: 3})
			same("ListNodes "+kind, a, b, ea, eb)
		}
		for _, o := range []graph.GetNodeOptions{{NodeID: oid}, {NodeID: oid, Revision: 1}, {NodeID: cid}, {NodeID: gid}, {NodeID: wid}, {NodeID: graph.NewID()}, {NodeID: "not-a-uuid"}, {NodeID: oid, Revision: 9}} {
			a, ea := w.s.GetNode(w.ctx, ns, id, o)
			b, eb := plain.GetNode(w.ctx, ns, id, o)
			same("GetNode", a, b, ea, eb)
		}
		a, ea := w.s.ExplainGoal(w.ctx, ns, id, gid)
		b, eb := plain.ExplainGoal(w.ctx, ns, id, gid)
		same("ExplainGoal", a, b, ea, eb)
		e1, ee1 := w.s.ListEligibleWork(w.ctx, ns, id)
		e2, ee2 := plain.ListEligibleWork(w.ctx, ns, id)
		same("ListEligibleWork", e1, e2, ee1, ee2)
		origin := mustList(t, w, "origin").Nodes[0].ID
		for _, rel := range [][]string{{"derived_from", "spawns"}, {"supports", "yields"}, {"depends_on"}} {
			t1, te1 := w.s.Descendants(w.ctx, ns, id, graph.TraversalOptions{NodeID: origin, Relations: rel, Depth: 4})
			t2, te2 := plain.Descendants(w.ctx, ns, id, graph.TraversalOptions{NodeID: origin, Relations: rel, Depth: 4})
			same("Descendants", t1, t2, te1, te2)
			t1, te1 = w.s.Ancestors(w.ctx, ns, id, graph.TraversalOptions{NodeID: gid, Relations: rel, Depth: 4})
			t2, te2 = plain.Ancestors(w.ctx, ns, id, graph.TraversalOptions{NodeID: gid, Relations: rel, Depth: 4})
			same("Ancestors", t1, t2, te1, te2)
			t1, te1 = w.s.Neighbors(w.ctx, ns, id, graph.TraversalOptions{NodeID: wid, Relations: rel})
			t2, te2 = plain.Neighbors(w.ctx, ns, id, graph.TraversalOptions{NodeID: wid, Relations: rel})
			same("Neighbors", t1, t2, te1, te2)
		}
		for _, budget := range []int{1024, 1500, 2000, 3000, 16384} {
			p1, pe1 := w.s.ProjectContext(w.ctx, ns, id, graph.ProjectionOptions{ByteBudget: budget, AssignedWorkItemID: wid})
			p2, pe2 := plain.ProjectContext(w.ctx, ns, id, graph.ProjectionOptions{ByteBudget: budget, AssignedWorkItemID: wid})
			same(fmt.Sprintf("ProjectContext %d", budget), p1, p2, pe1, pe2)
		}
		viaPlain := w.must(plain.AcceptGoal(w.ctx, acceptCtx, acceptInput))
		if !reflect.DeepEqual(viaPlain, accepted) {
			t.Fatal("complete-history replay differs from request-scoped replay")
		}
		changed := acceptInput
		changed.Rationale = "Different"
		if _, e := plain.AcceptGoal(w.ctx, acceptCtx, changed); graph.ErrorCode(e) != "idempotency_conflict" {
			t.Fatalf("complete-history conflict: %v", e)
		}
		if _, e := w.s.AcceptGoal(w.ctx, acceptCtx, changed); graph.ErrorCode(e) != "idempotency_conflict" {
			t.Fatalf("request-scoped conflict: %v", e)
		}
		plainWrite := w.next()
		hint := graph.AddHintInput{Title: "Written without extensions", HintPayload: graph.HintPayload{Instruction: "x"}}
		first := w.must(plain.AddHint(w.ctx, plainWrite, hint))
		if replay := w.must(w.s.AddHint(w.ctx, plainWrite, hint)); !reflect.DeepEqual(first, replay) {
			t.Fatal("request-scoped replay of complete-history write differs")
		}
	})
}
func mustList(t *testing.T, w *world, kind string) graph.NodePage {
	t.Helper()
	p, e := w.s.ListNodes(w.ctx, w.c.Namespace, w.c.GraphID, graph.ListOptions{Kind: kind})
	if e != nil || len(p.Nodes) == 0 {
		t.Fatalf("list %s: %v", kind, e)
	}
	return p
}
