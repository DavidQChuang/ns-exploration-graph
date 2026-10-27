package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/store/postgres"
	"os"
	"reflect"
	"strings"
	"testing"
)

func check(t *testing.T) func(graph.Result, error) graph.Result {
	return func(r graph.Result, e error) graph.Result { t.Helper(); return must(t, r, e) }
}
func traced(t *testing.T) (*postgres.Store, *postgres.StatementLog) {
	t.Helper()
	database(t)
	log := &postgres.StatementLog{}
	s, e := postgres.OpenTraced(context.Background(), os.Getenv("GRAPH_TEST_DATABASE_URL"), log)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	log.Take()
	return s, log
}
func rowCounts(t *testing.T, db *postgres.Store, id string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{"graphs", "nodes", "node_revisions", "edges", "resource_refs", "goal_acceptances", "graph_events"} {
		var n int
		if e := db.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM exploration_graph.`+table+` WHERE graph_id=$1`, id).Scan(&n); e != nil {
			t.Fatal(e)
		}
		out[table] = n
	}
	return out
}
func restorableGraph(t *testing.T, svc *graph.Service) (string, []byte, []byte) {
	t.Helper()
	ctx := context.Background()
	id := graph.NewID()
	create(t, svc, id)
	goal := check(t)(svc.AddGoal(ctx, command(id, "goal"), graph.AddGoalInput{Title: "Fixed", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"passes"}}})).Data["goal_id"].(string)
	wid := work(t, svc, id, "work")
	x := graph.NewID()
	check(t)(svc.TransitionWork(ctx, command(id, "claim"), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 1}, State: "running", ExecutionID: x}))
	c := command(id, "observe")
	c.Execution = &graph.Execution{WorkItemID: wid, ExecutionID: x}
	oid := check(t)(svc.RecordObservation(ctx, c, graph.RecordObservationInput{Title: "Passing", EvidencePayload: graph.EvidencePayload{Statement: "passes", EpistemicStatus: "observed"}, Resources: []graph.ResourceInput{{ResourceType: "log", URI: "test://log"}}})).Data["observation_id"].(string)
	edge := check(t)(svc.AddSupport(ctx, command(id, "support"), graph.AddSupportInput{Source: graph.Ref{NodeID: oid, Revision: 1}, Target: graph.Ref{NodeID: goal, Revision: 1}})).Data["edge_id"].(string)
	check(t)(svc.AcceptGoal(ctx, command(id, "accept"), graph.AcceptGoalInput{Goal: graph.Ref{NodeID: goal, Revision: 1}, SupportEdgeIDs: []string{edge}, Method: "validator", Rationale: "checked"}))
	running, e := svc.ExportSnapshot(ctx, "postgres-tests", id)
	if e != nil {
		t.Fatal(e)
	}
	check(t)(svc.TransitionWork(ctx, command(id, "done"), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 2}, State: "done"}))
	valid, e := svc.ExportSnapshot(ctx, "postgres-tests", id)
	if e != nil {
		t.Fatal(e)
	}
	return id, running, valid
}

func TestPostgresRestoreRejectionLeavesNoRows(t *testing.T) {
	base := database(t)
	ctx := context.Background()
	_, running, valid := restorableGraph(t, graph.NewService(base))
	dest := command(graph.NewID(), "restore")
	empty := rowCounts(t, base, dest.GraphID)
	for table, n := range empty {
		if n != 0 {
			t.Fatalf("fresh identity has %s rows", table)
		}
	}
	var unsupported graph.Snapshot
	_ = json.Unmarshal(valid, &unsupported)
	unsupported.SchemaVersion = 2
	bad, _ := json.Marshal(unsupported)
	for name, encoded := range map[string][]byte{"running": running, "unsupported version": bad, "invalid json": []byte(`{"schema_version":1`)} {
		if _, e := graph.NewService(base).ImportSnapshot(ctx, dest, encoded); graph.ErrorCode(e) != "invalid_argument" {
			t.Fatalf("%s restore: %v", name, e)
		}
		if got := rowCounts(t, base, dest.GraphID); !reflect.DeepEqual(got, empty) {
			t.Fatalf("%s restore left rows %v", name, got)
		}
	}
	for _, point := range []string{"after_nodes", "after_edges", "before_events", "after_events", "before_commit"} {
		failing := database(t, postgres.WithFaultInjector(func(p string) error {
			if p == point {
				return errors.New("injected restore failure")
			}
			return nil
		}))
		if _, e := graph.NewService(failing).ImportSnapshot(ctx, dest, valid); e == nil {
			t.Fatalf("%s: fault did not abort restore", point)
		}
		if got := rowCounts(t, base, dest.GraphID); !reflect.DeepEqual(got, empty) {
			t.Fatalf("%s: partial restore persisted %v", point, got)
		}
	}
	restored := check(t)(graph.NewService(base).ImportSnapshot(ctx, dest, valid))
	if restored.GraphRevision != 1 {
		t.Fatalf("restored revision %d", restored.GraphRevision)
	}
	var snap graph.Snapshot
	_ = json.Unmarshal(valid, &snap)
	revisions := 0
	for _, n := range snap.Nodes {
		revisions += len(n.Revisions)
	}
	want := map[string]int{"graphs": 1, "nodes": len(snap.Nodes), "node_revisions": revisions, "edges": len(snap.Edges), "resource_refs": len(snap.ResourceRefs), "goal_acceptances": len(snap.Acceptances), "graph_events": 1}
	if got := rowCounts(t, base, dest.GraphID); !reflect.DeepEqual(got, want) {
		t.Fatalf("restored rows %v, want %v", got, want)
	}
}

func TestPostgresReadsUseBoundedStatementSets(t *testing.T) {
	db, log := traced(t)
	svc := graph.NewService(db)
	ctx := context.Background()
	counts := func(size int) map[string]int {
		t.Helper()
		id := graph.NewID()
		create(t, svc, id)
		wid := ""
		for i := 0; i < size; i++ {
			wid = work(t, svc, id, graph.NewID())
		}
		log.Take()
		out := map[string]int{}
		reads := map[string]func() error{
			"overview": func() error {
				_, e := svc.ProjectContext(ctx, "postgres-tests", id, graph.ProjectionOptions{})
				return e
			},
			"get_node": func() error {
				_, e := svc.GetNode(ctx, "postgres-tests", id, graph.GetNodeOptions{NodeID: wid})
				return e
			},
			"list": func() error {
				_, e := svc.ListNodes(ctx, "postgres-tests", id, graph.ListOptions{})
				return e
			},
		}
		for name, read := range reads {
			if e := read(); e != nil {
				t.Fatal(e)
			}
			for _, sql := range log.Take() {
				out[name]++
				if name != "get_node" && (strings.Contains(sql, "resource_refs") || strings.Contains(sql, "goal_acceptances") || strings.Contains(sql, "node_revisions WHERE")) {
					t.Fatalf("%s loaded history or unrelated records: %s", name, sql)
				}
				if strings.Contains(sql, "graph_events") {
					t.Fatalf("%s read events: %s", name, sql)
				}
			}
		}
		c := command(id, "mutation")
		check(t)(svc.AddHint(ctx, c, graph.AddHintInput{Title: "Hint", HintPayload: graph.HintPayload{Instruction: "x"}}))
		first := log.Take()
		check(t)(svc.AddHint(ctx, c, graph.AddHintInput{Title: "Hint", HintPayload: graph.HintPayload{Instruction: "x"}}))
		for _, sql := range append(first, log.Take()...) {
			if strings.Contains(sql, "graph_events") && strings.HasPrefix(sql, "SELECT") && !strings.Contains(sql, "request_id=$2") {
				t.Fatalf("mutation loaded event history: %s", sql)
			}
		}
		return out
	}
	small, large := counts(3), counts(150)
	if !reflect.DeepEqual(small, large) {
		t.Fatalf("statement counts depend on graph size: %v vs %v", small, large)
	}
	if small["overview"] != 5 || small["list"] != 5 || small["get_node"] != 8 {
		t.Fatalf("unexpected statement counts %v", small)
	}
}
