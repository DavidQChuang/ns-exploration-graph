package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/store/postgres"
	"os"
	"strings"
	"testing"
	"time"
)

// BenchmarkPostgresAggregate measures ordinary reads and the first invalidation of a
// current goal acceptance. Fixture insertion is excluded. Statement counts are
// recorded by a pgx tracer and include BEGIN/COMMIT. Each invalidation iteration
// re-establishes a current acceptance with the timer stopped.
func BenchmarkPostgresAggregate(b *testing.B) {
	dsn := os.Getenv("GRAPH_TEST_DATABASE_URL")
	if dsn == "" {
		b.Fatal("GRAPH_TEST_DATABASE_URL is required for PostgreSQL benchmarks")
	}
	ctx := context.Background()
	log := &postgres.StatementLog{}
	db, e := postgres.OpenTraced(ctx, dsn, log)
	if e != nil {
		b.Fatal(e)
	}
	defer db.Close()
	if e = db.Migrate(ctx); e != nil {
		b.Fatal(e)
	}
	report := func(b *testing.B, statements []string, result any) {
		selects := 0
		for _, sql := range statements {
			if strings.HasPrefix(sql, "SELECT") {
				selects++
			}
		}
		encoded, _ := json.Marshal(result)
		b.ReportMetric(float64(len(encoded)), "json-bytes")
		b.ReportMetric(float64(len(statements))/float64(b.N), "stmts/op")
		b.ReportMetric(float64(selects)/float64(b.N), "selects/op")
	}
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			svc := graph.NewService(db)
			id := graph.NewID()
			c := command(id, "create")
			if _, e = svc.CreateGraph(ctx, c, graph.CreateGraphInput{Title: "Aggregate benchmark", Request: "Measure aggregate costs"}); e != nil {
				b.Fatal(e)
			}
			next := func() graph.CommandContext { c.RequestID = graph.NewID(); return c }
			goal, e := svc.AddGoal(ctx, next(), graph.AddGoalInput{Title: "Benchmark goal", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Measured"}}})
			if e != nil {
				b.Fatal(e)
			}
			w, e := svc.ProposeWork(ctx, next(), graph.ProposeWorkInput{Title: "Fixture producer", WorkPayload: graph.WorkPayload{Instruction: "Measure"}})
			if e != nil {
				b.Fatal(e)
			}
			wid := w.Data["work_item_id"].(string)
			execution := graph.NewID()
			if _, e = svc.TransitionWork(ctx, next(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 1}, State: "running", ExecutionID: execution}); e != nil {
				b.Fatal(e)
			}
			worker := next()
			worker.Execution = &graph.Execution{WorkItemID: wid, ExecutionID: execution}
			obs, e := svc.RecordObservation(ctx, worker, graph.RecordObservationInput{Title: "Measurement evidence", EvidencePayload: graph.EvidencePayload{Statement: "Measured", Evidence: "benchmark fixture", EpistemicStatus: "observed"}})
			if e != nil {
				b.Fatal(e)
			}
			oid := obs.Data["observation_id"].(string)
			gid := goal.Data["goal_id"].(string)
			support, e := svc.AddSupport(ctx, next(), graph.AddSupportInput{Source: graph.Ref{NodeID: oid, Revision: 1}, Target: graph.Ref{NodeID: gid, Revision: 1}})
			if e != nil {
				b.Fatal(e)
			}
			if _, e = svc.AcceptGoal(ctx, next(), graph.AcceptGoalInput{Goal: graph.Ref{NodeID: gid, Revision: 1}, SupportEdgeIDs: []string{support.Data["edge_id"].(string)}, Method: "validator", Rationale: "Fixture"}); e != nil {
				b.Fatal(e)
			}
			var origin string
			observation, goalRevision := 1, 2
			e = db.Update(ctx, c.Namespace, id, nil, func(st *graph.State) error {
				for _, n := range st.Nodes {
					if n.Kind == "origin" {
						origin = n.ID
					}
				}
				now := time.Now().UTC()
				payload := json.RawMessage(`{"instruction":"Synthetic independent work","priority":50}`)
				for len(st.Nodes) < size {
					nid := graph.NewID()
					st.Nodes = append(st.Nodes, graph.Node{ID: nid, Kind: "work_item", CurrentRevision: 1, CreatedAt: now, Revisions: []graph.Revision{{Revision: 1, State: "open", Title: "Synthetic work", Payload: payload, ActorID: "benchmark", CreatedAt: now}}})
					st.Edges = append(st.Edges, graph.Edge{ID: graph.NewID(), SourceID: origin, SourceRevision: 1, TargetID: nid, TargetRevision: 1, Relation: "derived_from", ActorID: "benchmark", CreatedAt: now})
				}
				return nil
			})
			if e != nil {
				b.Fatal(e)
			}
			measure := func(name string, op func() (any, error)) {
				b.Run(name, func(b *testing.B) {
					log.Take()
					var result any
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if result, e = op(); e != nil {
							b.Fatal(e)
						}
					}
					b.StopTimer()
					report(b, log.Take(), result)
				})
			}
			measure("overview", func() (any, error) {
				return svc.ProjectContext(ctx, c.Namespace, id, graph.ProjectionOptions{ByteBudget: 16384})
			})
			measure("traversal", func() (any, error) {
				return svc.Descendants(ctx, c.Namespace, id, graph.TraversalOptions{NodeID: origin, Relations: []string{"derived_from"}, Depth: 1, Limit: 100})
			})
			measure("get_node", func() (any, error) {
				return svc.GetNode(ctx, c.Namespace, id, graph.GetNodeOptions{NodeID: gid})
			})
			b.Run("invalidation", func(b *testing.B) {
				var statements []string
				var result graph.Result
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					log.Take()
					result, e = svc.ReviseNode(ctx, next(), graph.ReviseNodeInput{Node: graph.Ref{NodeID: oid, Revision: observation}, Title: "Revised evidence", Payload: json.RawMessage(`{"statement":"Changed","evidence":"changed log","epistemic_status":"observed"}`)})
					if e != nil {
						b.Fatal(e)
					}
					statements = append(statements, log.Take()...)
					b.StopTimer()
					observation++
					goalRevision++
					if v, e := svc.GetNode(ctx, c.Namespace, id, graph.GetNodeOptions{NodeID: gid}); e != nil || v.Revision.State != "needs_review" || v.CurrentRevision != goalRevision {
						b.Fatalf("acceptance was not invalidated: %v", e)
					}
					support, e := svc.AddSupport(ctx, next(), graph.AddSupportInput{Source: graph.Ref{NodeID: oid, Revision: observation}, Target: graph.Ref{NodeID: gid, Revision: goalRevision}})
					if e != nil {
						b.Fatal(e)
					}
					if _, e = svc.AcceptGoal(ctx, next(), graph.AcceptGoalInput{Goal: graph.Ref{NodeID: gid, Revision: goalRevision}, SupportEdgeIDs: []string{support.Data["edge_id"].(string)}, Method: "validator", Rationale: "Fixture"}); e != nil {
						b.Fatal(e)
					}
					goalRevision++
					b.StartTimer()
				}
				b.StopTimer()
				report(b, statements, result)
			})
		})
	}
}
