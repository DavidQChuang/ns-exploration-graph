package conformance

import (
	"encoding/json"
	"fmt"
	"ns-exploration-graph/graph"
	"strings"
	"sync"
	"testing"
	"time"
)

// accepted adds one observation-backed goal, accepts it, and returns the goal and acceptance IDs.
func (w *world) accepted(wc graph.CommandContext, title, obs string) (string, string) {
	w.t.Helper()
	gid := w.must(w.s.AddGoal(w.ctx, w.next(), graph.AddGoalInput{Title: title, GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{title + " holds"}}})).Data["goal_id"].(string)
	edge := w.must(w.s.AddSupport(w.ctx, w.next(), graph.AddSupportInput{Source: graph.Ref{NodeID: obs, Revision: 1}, Target: graph.Ref{NodeID: gid, Revision: 1}})).Data["edge_id"].(string)
	a := w.must(w.s.AcceptGoal(w.ctx, w.next(), graph.AcceptGoalInput{Goal: graph.Ref{NodeID: gid, Revision: 1}, SupportEdgeIDs: []string{edge}, Method: "validator", Rationale: "Checked " + title}))
	return gid, a.Data["acceptance_id"].(string)
}

// explain returns the goal's revision states and its acceptance states by ID.
func (w *world) explain(gid string) ([]string, map[string]string) {
	w.t.Helper()
	x, e := w.s.ExplainGoal(w.ctx, w.c.Namespace, w.c.GraphID, gid)
	if e != nil {
		w.t.Fatal(e)
	}
	revs, acc := []string{}, map[string]string{}
	for _, r := range x.Goal.Revisions {
		revs = append(revs, r.State)
	}
	for _, a := range x.Acceptances {
		acc[a.ID] = a.State
	}
	return revs, acc
}
func (w *world) snapshot() graph.Snapshot {
	w.t.Helper()
	s, e := w.s.Snapshot(w.ctx, w.c.Namespace, w.c.GraphID)
	if e != nil {
		w.t.Fatal(e)
	}
	return s
}
func (w *world) dependsOn(dependent string) []graph.Edge {
	out := []graph.Edge{}
	for _, ed := range w.snapshot().Edges {
		if ed.Relation == "depends_on" && ed.SourceID == dependent {
			out = append(out, ed)
		}
	}
	return out
}
func (w *world) open(title string) string {
	w.t.Helper()
	return w.must(w.s.ProposeWork(w.ctx, w.next(), graph.ProposeWorkInput{Title: title, WorkPayload: graph.WorkPayload{Instruction: "Run " + title}})).Data["work_item_id"].(string)
}
func sameStrings(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

func runGaps(t *testing.T, newStore func(t *testing.T) graph.Store) {
	t.Run("DisjointEvidenceRevisionInvalidatesOnlyItsGoal", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Disjoint origin")
		_, wc := w.running("Probe", 10)
		o1, o2 := w.observe(wc, "First", "Fails on BOM"), w.observe(wc, "Second", "Fails on NUL")
		g1, a1 := w.accepted(wc, "Goal one", o1)
		g2, a2 := w.accepted(wc, "Goal two", o2)
		before := w.node(g2).Revision
		w.must(w.s.ReviseNode(w.ctx, w.next(), graph.ReviseNodeInput{Node: graph.Ref{NodeID: o1, Revision: 1}, Title: "First corrected", Payload: json.RawMessage(`{"statement":"Fails on BOM only","evidence":"log v2","epistemic_status":"observed"}`)}))
		revs, acc := w.explain(g1)
		if !sameStrings(revs, []string{"open", "met", "needs_review"}) || acc[a1] != "stale" {
			t.Fatalf("goal one revisions %v acceptances %v", revs, acc)
		}
		revs, acc = w.explain(g2)
		if !sameStrings(revs, []string{"open", "met"}) || acc[a2] != "current" || len(acc) != 1 {
			t.Fatalf("goal two revisions %v acceptances %v", revs, acc)
		}
		if after := w.node(g2).Revision; after.Revision != before.Revision || after.State != "met" {
			t.Fatalf("goal two changed: %#v -> %#v", before, after)
		}
	})
	t.Run("ReviseMetGoalCriteriaIsOneRevisionNotAcceptanceSecondInvalidation", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Criteria origin")
		_, wc := w.running("Probe", 10)
		gid, aid := w.accepted(wc, "Criteria goal", w.observe(wc, "Seen", "Fails"))
		other, otherAcceptance := w.accepted(wc, "Untouched goal", w.observe(wc, "Elsewhere", "Also fails"))
		events := func() int {
			page, e := w.s.EventsAfter(w.ctx, w.c.Namespace, w.c.GraphID, 0, 1000)
			if e != nil {
				t.Fatal(e)
			}
			return len(page.Events)
		}
		revision, count := w.snapshot().Graph.Revision, events()
		changed := json.RawMessage(`{"acceptance_criteria":["Regression passes","BOM input rejected cleanly"]}`)
		r := w.must(w.s.ReviseNode(w.ctx, w.next(), graph.ReviseNodeInput{Node: graph.Ref{NodeID: gid, Revision: 2}, Title: "Criteria goal", Payload: changed}))
		if r.GraphRevision != revision+1 || r.Data["node_revision"] != float64(3) || events() != count+1 {
			t.Fatalf("one command must add one graph revision and one event: %#v events %d->%d", r, count, events())
		}
		x, e := w.s.ExplainGoal(w.ctx, w.c.Namespace, w.c.GraphID, gid)
		if e != nil || x.Goal.CurrentRevision != 3 || len(x.Goal.Revisions) != 3 {
			t.Fatalf("extra goal revision: %#v %v", x.Goal, e)
		}
		for i, state := range []string{"open", "met", "needs_review"} {
			if x.Goal.Revisions[i].State != state {
				t.Fatalf("revision %d is %s, want %s", i+1, x.Goal.Revisions[i].State, state)
			}
		}
		if !jsonEqualString(t, x.Goal.Revisions[2].Payload, changed) {
			t.Fatalf("criteria not revised: %s", x.Goal.Revisions[2].Payload)
		}
		if len(x.Acceptances) != 1 || x.Acceptances[0].ID != aid || x.Acceptances[0].State != "stale" {
			t.Fatalf("acceptance not stale: %#v", x.Acceptances)
		}
		if revs, acc := w.explain(other); !sameStrings(revs, []string{"open", "met"}) || acc[otherAcceptance] != "current" {
			t.Fatalf("unrelated goal changed: %v %v", revs, acc)
		}
	})
	t.Run("ClaimRacingDependencyEditNeverRunsWorkWithUndonePrerequisite", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Race origin")
		claimFirst, dependencyFirst := 0, 0
		for i := 0; i < 25; i++ {
			work, prerequisite := w.open(fmt.Sprintf("Work %d", i)), w.open(fmt.Sprintf("Prerequisite %d", i))
			var wg sync.WaitGroup
			start := make(chan struct{})
			var claimErr, depErr error
			claim, dep := w.next(), w.next()
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				_, claimErr = w.s.TransitionWork(w.ctx, claim, graph.TransitionWorkInput{Node: graph.Ref{NodeID: work, Revision: 1}, State: "running", ExecutionID: graph.NewID()})
			}()
			go func() {
				defer wg.Done()
				<-start
				_, depErr = w.s.AddDependency(w.ctx, dep, graph.DependencyInput{Dependent: graph.Ref{NodeID: work, Revision: 1}, PrerequisiteID: prerequisite})
			}()
			close(start)
			wg.Wait()
			state, edges := w.node(work).Revision.State, w.dependsOn(work)
			if w.node(prerequisite).Revision.State == "done" {
				t.Fatalf("attempt %d: prerequisite finished", i)
			}
			if state == "running" && (depErr == nil || len(edges) != 0) {
				t.Fatalf("attempt %d: running work depends on an undone prerequisite: dep %v edges %d", i, depErr, len(edges))
			}
			if len(edges) != 0 && (claimErr == nil || state != "open") {
				t.Fatalf("attempt %d: dependency exists but claim %v, state %s", i, claimErr, state)
			}
			switch {
			case claimErr == nil && graph.ErrorCode(depErr) == "revision_conflict" && state == "running":
				claimFirst++
			case depErr == nil && graph.ErrorCode(claimErr) == "invalid_transition" && state == "open" && len(edges) == 1:
				dependencyFirst++
			default:
				t.Fatalf("attempt %d: claim %v, dependency %v, state %s, edges %d", i, claimErr, depErr, state, len(edges))
			}
		}
		t.Logf("claim won %d, dependency won %d of 25", claimFirst, dependencyFirst)
	})
	t.Run("AddDependencyOnRunningWorkIsInvalidTransition", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Running dependency origin")
		work, _ := w.running("Running", 10)
		prerequisite := w.open("Prerequisite")
		if _, e := w.s.AddDependency(w.ctx, w.next(), graph.DependencyInput{Dependent: graph.Ref{NodeID: work, Revision: 2}, PrerequisiteID: prerequisite}); graph.ErrorCode(e) != "invalid_transition" {
			t.Fatalf("dependency on running work: %v", e)
		}
		if v := w.node(work).Revision; v.State != "running" || v.Revision != 2 || len(w.dependsOn(work)) != 0 {
			t.Fatalf("running work changed: %#v edges %d", v, len(w.dependsOn(work)))
		}
	})
	t.Run("SnapshotRelationCycleRejectedWithoutGraphOrEvents", func(t *testing.T) {
		w := newWorld(t, newStore(t), "Cycle origin")
		probe, wc := w.running("Probe", 10)
		o := w.observe(wc, "Seen", "Fails")
		c1 := w.must(w.s.RecordConclusion(w.ctx, w.next(), graph.RecordConclusionInput{Title: "Cause", EvidencePayload: graph.EvidencePayload{Statement: "Bug", EpistemicStatus: "inferred"}, Supports: []graph.Ref{{NodeID: o, Revision: 1}}})).Data["conclusion_id"].(string)
		c2 := w.must(w.s.RecordConclusion(w.ctx, w.next(), graph.RecordConclusionInput{Title: "Effect", EvidencePayload: graph.EvidencePayload{Statement: "Crash", EpistemicStatus: "inferred"}, Supports: []graph.Ref{{NodeID: o, Revision: 1}}})).Data["conclusion_id"].(string)
		w.must(w.s.AddSupport(w.ctx, w.next(), graph.AddSupportInput{Source: graph.Ref{NodeID: c1, Revision: 1}, Target: graph.Ref{NodeID: c2, Revision: 1}}))
		a, b := w.open("First"), w.open("Second")
		w.must(w.s.AddDependency(w.ctx, w.next(), graph.DependencyInput{Dependent: graph.Ref{NodeID: a, Revision: 1}, PrerequisiteID: b}))
		w.must(w.s.TransitionWork(w.ctx, w.next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: probe, Revision: 2}, State: "done"}))
		exported, e := w.s.ExportSnapshot(w.ctx, w.c.Namespace, w.c.GraphID)
		if e != nil {
			t.Fatal(e)
		}
		restore := func(label string, mutate func(*graph.Snapshot)) (graph.CommandContext, error) {
			var s graph.Snapshot
			_ = json.Unmarshal(exported, &s)
			mutate(&s)
			dest := w.next()
			dest.Namespace, dest.GraphID = "cycle-"+label, graph.NewID()
			encoded, _ := json.Marshal(s)
			_, e := w.s.ImportSnapshot(w.ctx, dest, encoded)
			return dest, e
		}
		back := func(relation, source, target string, revision int) func(*graph.Snapshot) {
			return func(s *graph.Snapshot) {
				s.Edges = append(s.Edges, graph.Edge{ID: graph.NewID(), SourceID: source, TargetID: target, SourceRevision: revision, TargetRevision: revision, Relation: relation, ActorID: "tester", CreatedAt: time.Now().UTC()})
			}
		}
		if _, e = restore("control", func(*graph.Snapshot) {}); e != nil {
			t.Fatalf("acyclic control rejected: %v", e)
		}
		for name, mutate := range map[string]func(*graph.Snapshot){"supports": back("supports", c2, c1, 1), "depends_on": back("depends_on", b, a, 0)} {
			dest, e := restore(name, mutate)
			if graph.ErrorCode(e) != "invalid_argument" || !strings.Contains(e.Error(), "cycle") {
				t.Fatalf("%s cycle accepted or rejected for another reason: %v", name, e)
			}
			if _, e = w.s.Snapshot(w.ctx, dest.Namespace, dest.GraphID); graph.ErrorCode(e) != "not_found" {
				t.Fatalf("%s: rejected restore left graph state: %v", name, e)
			}
			if _, e = w.s.EventsAfter(w.ctx, dest.Namespace, dest.GraphID, 0, 10); graph.ErrorCode(e) != "not_found" {
				t.Fatalf("%s: rejected restore left events: %v", name, e)
			}
		}
	})
}
func jsonEqualString(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return string(ja) == string(jb)
}
