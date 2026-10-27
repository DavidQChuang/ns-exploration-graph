package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/conformance"
	"ns-exploration-graph/graph/store/postgres"
	"os"
	"reflect"
	"sync"
	"testing"
)

func database(t *testing.T, options ...func(*postgres.Store)) *postgres.Store {
	t.Helper()
	dsn := os.Getenv("GRAPH_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" || os.Getenv("GRAPH_REQUIRE_POSTGRES") == "1" {
			t.Fatal("GRAPH_TEST_DATABASE_URL is required; PostgreSQL conformance cannot be skipped in CI")
		}
		t.Skip("set GRAPH_TEST_DATABASE_URL to run isolated PostgreSQL conformance")
	}
	s, e := postgres.Open(context.Background(), dsn, options...)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if e = s.Migrate(context.Background()); e != nil {
		t.Fatal(e)
	}
	return s
}
func command(id, request string) graph.CommandContext {
	return graph.CommandContext{Namespace: "postgres-tests", GraphID: id, RequestID: request, ActorID: "tester"}
}
func must(t *testing.T, r graph.Result, e error) graph.Result {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func create(t *testing.T, s *graph.Service, id string) {
	t.Helper()
	r, e := s.CreateGraph(context.Background(), command(id, "create"), graph.CreateGraphInput{Title: "Parser investigation", Request: "Find parser defect"})
	must(t, r, e)
}
func work(t *testing.T, s *graph.Service, id, request string) string {
	t.Helper()
	r, e := s.ProposeWork(context.Background(), command(id, request), graph.ProposeWorkInput{Title: request, WorkPayload: graph.WorkPayload{Instruction: "Reproduce defect", Priority: 50}})
	return must(t, r, e).Data["work_item_id"].(string)
}
func TestPostgresPersistentRestartLostResponse(t *testing.T) {
	db := database(t)
	ctx := context.Background()
	svc := graph.NewService(db)
	id := graph.NewID()
	create(t, svc, id)
	r, e := svc.AddGoal(ctx, command(id, "goal"), graph.AddGoalInput{Title: "Resolve parser", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"regression test passes"}}})
	must(t, r, e)
	wid := work(t, svc, id, "reproduce")
	execution := graph.NewID()
	r, e = svc.TransitionWork(ctx, command(id, "claim"), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 1}, State: "running", ExecutionID: execution})
	must(t, r, e)
	c := command(id, "observation")
	c.Execution = &graph.Execution{WorkItemID: wid, ExecutionID: execution}
	input := graph.RecordObservationInput{Title: "Negative reproduction", EvidencePayload: graph.EvidencePayload{Statement: "Parser rejects valid input", EpistemicStatus: "observed", Polarity: "negative"}, Resources: []graph.ResourceInput{{ResourceType: "test-log", URI: "test://parser/failure"}}}
	committed, e := svc.RecordObservation(ctx, c, input)
	must(t, committed, e) // response intentionally discarded before reconnect
	r, e = svc.TransitionWork(ctx, cWithRequest(c, "complete"), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 2}, State: "done"})
	must(t, r, e)
	r, e = svc.SetGraphState(ctx, command(id, "close"), graph.SetGraphStateInput{State: "closed"})
	must(t, r, e)
	// A fresh driver pool and service have no process-local graph or idempotency state.
	restarted := database(t)
	svc = graph.NewService(restarted)
	retry, e := svc.RecordObservation(ctx, c, input)
	must(t, retry, e)
	if !reflect.DeepEqual(committed, retry) {
		t.Fatalf("lost response retry changed result: %#v != %#v", committed, retry)
	}
	state, e := restarted.Read(ctx, c.Namespace, id)
	if e != nil {
		t.Fatal(e)
	}
	if len(state.Nodes) != 4 || len(state.ResourceRefs) != 1 || state.Graph.State != "closed" || len(state.Events) != 0 {
		t.Fatalf("unexpected persisted snapshot: nodes=%d refs=%d graph=%s events=%d", len(state.Nodes), len(state.ResourceRefs), state.Graph.State, len(state.Events))
	}
	events, revision, e := restarted.EventsAfter(ctx, c.Namespace, id, 0, 100)
	if e != nil {
		t.Fatal(e)
	}
	if len(events) != 7 || revision != 7 {
		t.Fatalf("retry duplicated event: %d/%d", len(events), revision)
	}
	input.Title = "different"
	_, e = svc.RecordObservation(ctx, c, input)
	if graph.ErrorCode(e) != "idempotency_conflict" {
		t.Fatalf("got %v", e)
	}
}
func cWithRequest(c graph.CommandContext, r string) graph.CommandContext { c.RequestID = r; return c }
func TestPostgresInjectedRollback(t *testing.T) {
	for _, point := range []string{"after_nodes", "after_edges", "before_events", "after_events", "before_commit"} {
		t.Run(point, func(t *testing.T) {
			base := database(t)
			ctx := context.Background()
			id := graph.NewID()
			create(t, graph.NewService(base), id)
			before, e := base.Read(ctx, "postgres-tests", id)
			if e != nil {
				t.Fatal(e)
			}
			failing := database(t, postgres.WithFaultInjector(func(p string) error {
				if p == point {
					return errors.New("injected failure")
				}
				return nil
			}))
			svc := graph.NewService(failing)
			_, e = svc.AddGoal(ctx, command(id, "rollback"), graph.AddGoalInput{Title: "must rollback", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"atomic writes"}}})
			if e == nil {
				t.Fatal("fault did not abort")
			}
			after, e := base.Read(ctx, "postgres-tests", id)
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("partial node/edge/graph mutation persisted")
			}
			events, revision, e := base.EventsAfter(ctx, "postgres-tests", id, 0, 100)
			if e != nil {
				t.Fatal(e)
			}
			if len(events) != 1 || revision != 1 {
				t.Fatal("failed mutation persisted event")
			}
			r, e := graph.NewService(base).AddGoal(ctx, command(id, "rollback"), graph.AddGoalInput{Title: "must rollback", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"atomic writes"}}})
			must(t, r, e)
		})
	}
}
func TestPostgresSimultaneousClaim(t *testing.T) {
	db := database(t)
	svc := graph.NewService(db)
	id := graph.NewID()
	create(t, svc, id)
	wid := work(t, svc, id, "work")
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, e := svc.TransitionWork(context.Background(), command(id, fmt.Sprintf("claim-%d", i)), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 1}, State: "running", ExecutionID: graph.NewID()})
			results <- e
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for e := range results {
		if e == nil {
			winners++
		} else if graph.ErrorCode(e) != "revision_conflict" {
			t.Fatal(e)
		}
	}
	if winners != 1 {
		t.Fatalf("claim winners=%d", winners)
	}
}
func TestPostgresConcurrentDependencies(t *testing.T) {
	db := database(t)
	svc := graph.NewService(db)
	id := graph.NewID()
	create(t, svc, id)
	a := work(t, svc, id, "A")
	b := work(t, svc, id, "B")
	start := make(chan struct{})
	out := make(chan error, 2)
	for i, p := range []graph.DependencyInput{{Dependent: graph.Ref{NodeID: a, Revision: 1}, PrerequisiteID: b}, {Dependent: graph.Ref{NodeID: b, Revision: 1}, PrerequisiteID: a}} {
		go func(i int, p graph.DependencyInput) {
			<-start
			_, e := svc.AddDependency(context.Background(), command(id, fmt.Sprintf("dep-%d", i)), p)
			out <- e
		}(i, p)
	}
	close(start)
	wins := 0
	for i := 0; i < 2; i++ {
		e := <-out
		if e == nil {
			wins++
		} else if graph.ErrorCode(e) != "cycle_detected" {
			t.Fatal(e)
		}
	}
	if wins != 1 {
		t.Fatalf("dependency winners=%d", wins)
	}
}
func TestPostgresConcurrentCreation(t *testing.T) {
	db := database(t)
	svc := graph.NewService(db)
	id := graph.NewID()
	ctx := context.Background()
	c := command(id, "create")
	input := graph.CreateGraphInput{Title: "origin", Request: "create once"}
	start := make(chan struct{})
	out := make(chan graph.Result, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; r, e := svc.CreateGraph(ctx, c, input); out <- r; errs <- e }()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
	a, b := <-out, <-out
	if !reflect.DeepEqual(a, b) {
		t.Fatal("concurrent creation did not replay same result")
	}
}

func TestPostgresConformance(t *testing.T) {
	conformance.Run(t, func(t *testing.T) graph.Store { return database(t) })
}
