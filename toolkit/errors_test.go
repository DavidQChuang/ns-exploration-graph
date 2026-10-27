package toolkit_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"ns-exploration-graph/graph"
	"ns-exploration-graph/toolkit"
)

// failing wraps the real service and fails selected calls with a hostile error.
type failing struct {
	*graph.Service
	err error
}

func (f failing) ProjectContext(context.Context, string, string, graph.ProjectionOptions) (graph.Projection, error) {
	return graph.Projection{}, f.err
}
func (f failing) ProposeWork(context.Context, graph.CommandContext, graph.ProposeWorkInput) (graph.Result, error) {
	return graph.Result{}, f.err
}

// TestEveryStableErrorCodeIsProducedAndSchemaValid drives real tool calls into each
// stable error code and validates the envelope against the result schema. Codes are
// compared with the schema's enum so a new code cannot be added without a test.
func TestEveryStableErrorCodeIsProducedAndSchemaValid(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		seen := map[string]bool{}
		expect := func(tool, code string, run func() toolkit.Envelope) {
			t.Helper()
			before := w.revision()
			out := run()
			w.fails(tool, out, code, before) // schema-valid, stable shape, graph unchanged
			seen[code] = true
		}
		wid := w.work("evidence work")
		x := w.claim(wid, 1)
		worker := w.worker(x)
		planner := w.adapter(nil)
		granted := w.adapter(func(b *toolkit.Binding) { b.AcceptGoal, b.AcceptanceMethod = true, "validator" }, func(c *toolkit.Config) {
			c.AcceptancePolicy = func(context.Context, toolkit.Binding, graph.AcceptGoalInput) error { return nil }
		})
		oid := ok(t, "graph_record_observation", call(worker, "graph_record_observation", obs("Rejected"), "e-ob"))["observation_id"].(string)
		conclude := func(title, support, id string) string {
			return ok(t, "graph_record_conclusion", call(planner, "graph_record_conclusion", map[string]any{"title": title, "statement": "s", "epistemic_status": "inferred", "supports": []any{ref(support, 1)}}, id))["conclusion_id"].(string)
		}
		c1 := conclude("C1", oid, "e-c1")
		c2 := conclude("C2", c1, "e-c2")
		edge := ok(t, "graph_add_support", call(planner, "graph_add_support", map[string]any{"source": ref(c2, 1), "target": ref(w.goal.NodeID, 1)}, "e-s"))["edge_id"]
		origin := ok(t, "graph_list_nodes", call(planner, "graph_list_nodes", map[string]any{"kind": "origin"}, ""))["nodes"].([]any)[0].(map[string]any)["id"].(string)
		addSupport := func(id string, src, dst map[string]any) func() toolkit.Envelope {
			return func() toolkit.Envelope {
				return call(planner, "graph_add_support", map[string]any{"source": src, "target": dst}, id)
			}
		}

		expect("graph_get_node", "invalid_argument", func() toolkit.Envelope {
			return call(planner, "graph_get_node", map[string]any{"node_id": "bad"}, "")
		})
		expect("graph_get_node", "not_found", func() toolkit.Envelope {
			return call(planner, "graph_get_node", map[string]any{"node_id": graph.NewID()}, "")
		})
		expect("graph_propose_work", "permission_denied", func() toolkit.Envelope {
			return call(worker, "graph_propose_work", map[string]any{"title": "t", "instruction": "i", "priority": 1}, "e-perm")
		})
		expect("graph_add_support", "revision_conflict", addSupport("e-rev", ref(oid, 1), ref(w.goal.NodeID, 9)))
		expect("graph_add_support", "invalid_relation", addSupport("e-rel", ref(oid, 1), ref(origin, 1)))
		expect("graph_add_support", "cycle_detected", addSupport("e-cycle", ref(c2, 1), ref(c1, 1)))
		expect("graph_accept_goal", "stale_evidence", func() toolkit.Envelope {
			return call(granted, "graph_accept_goal", map[string]any{"goal": ref(w.goal.NodeID, 1), "support_edge_ids": []any{graph.NewID()}, "rationale": "r"}, "e-stale")
		})
		ok(t, "graph_propose_work", call(planner, "graph_propose_work", map[string]any{"title": "a", "instruction": "i", "priority": 1}, "e-idem"))
		expect("graph_propose_work", "idempotency_conflict", func() toolkit.Envelope {
			return call(planner, "graph_propose_work", map[string]any{"title": "b", "instruction": "i", "priority": 1}, "e-idem")
		})
		// invalid_transition: the goal is already met after a successful acceptance.
		accept := map[string]any{"goal": ref(w.goal.NodeID, 1), "support_edge_ids": []any{edge}, "rationale": "Reviewed."}
		ok(t, "graph_accept_goal", call(granted, "graph_accept_goal", accept, "e-accept"))
		expect("graph_accept_goal", "invalid_transition", func() toolkit.Envelope {
			return call(granted, "graph_accept_goal", map[string]any{"goal": ref(w.goal.NodeID, 2), "support_edge_ids": []any{edge}, "rationale": "Again."}, "e-accept-2")
		})
		// stale_cursor: a cursor does not survive a later write.
		page := ok(t, "graph_list_nodes", call(planner, "graph_list_nodes", map[string]any{"limit": 1}, ""))
		w.work("invalidates cursors")
		expect("graph_list_nodes", "stale_cursor", func() toolkit.Envelope {
			return call(planner, "graph_list_nodes", map[string]any{"limit": 1, "cursor": page["next_cursor"]}, "")
		})
		// limit_exceeded: configured input limit.
		small := w.adapter(nil, func(c *toolkit.Config) {
			c.Limits = toolkit.Limits{InputBytes: 64, ResultBytes: svc.Limits.ResultBytes, OverviewBytes: 16384}
		})
		expect("graph_propose_work", "limit_exceeded", func() toolkit.Envelope {
			return call(small, "graph_propose_work", map[string]any{"title": "t", "instruction": strings.Repeat("x", 100), "priority": 1}, "e-big")
		})
		// budget_too_small: assigned work whose anchor cannot fit the requested budget.
		bigWork := w.workWith("big assignment", strings.Repeat("y", 7000))
		bx := w.claim(bigWork, 1)
		expect("graph_overview", "budget_too_small", func() toolkit.Envelope {
			return call(w.worker(bx), "graph_overview", map[string]any{"byte_budget": 1024}, "")
		})
		// stale_execution: the host stops the worker.
		w.transition(wid, 2, "stopped")
		expect("graph_record_observation", "stale_execution", func() toolkit.Envelope {
			return call(worker, "graph_record_observation", obs("late"), "e-late")
		})
		// graph_read_only: after the host closes the graph.
		w.transition(bigWork, 2, "stopped")
		_, e := svc.SetGraphState(w.ctx, w.host(), graph.SetGraphStateInput{State: "closed"})
		w.must(e)
		expect("graph_propose_work", "graph_read_only", func() toolkit.Envelope {
			return call(planner, "graph_propose_work", map[string]any{"title": "t", "instruction": "i", "priority": 1}, "e-closed")
		})

		want := map[string]bool{}
		for _, d := range loadManifest(t).Tools {
			walk(d.OutputSchema, func(p string, n map[string]any) {
				if strings.HasSuffix(p, "/properties/code") {
					for _, c := range n["enum"].([]any) {
						want[c.(string)] = true
					}
				}
			})
		}
		// already_exists is a host/CreateGraph error that no agent tool can reach;
		// storage_unavailable is covered with a failing service below.
		delete(want, "already_exists")
		delete(want, "storage_unavailable")
		missing := []string{}
		for c := range want {
			if !seen[c] {
				missing = append(missing, c)
			}
		}
		sort.Strings(missing)
		if len(missing) != 0 {
			t.Fatalf("error codes with no real tool-level test: %v", missing)
		}
	})
}

func TestStorageFailuresAreSafeRetryableEnvelopes(t *testing.T) {
	secret := "password=hunter2 host=10.1.2.3 SELECT * FROM exploration_graph.graphs"
	for name, cause := range map[string]error{
		"plain error":         errors.New("pgx: " + secret),
		"coded storage error": graph.Err("storage_unavailable", "pq: "+secret),
		"wrapped plain error": fmt.Errorf("commit: %w", errors.New(secret)),
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t, graph.NewService(memoryStore()))
			a, e := toolkit.New(failing{w.svc, cause}, toolkit.Config{Binding: w.b})
			w.must(e)
			for tool, args := range map[string]map[string]any{"graph_overview": {}, "graph_propose_work": {"title": "t", "instruction": "i", "priority": 1}} {
				out := call(a, tool, args, "storage-1")
				if out.OK || out.Error == nil || out.Error.Code != "storage_unavailable" || !out.Error.Retryable {
					t.Fatalf("%s: %s", tool, mustJSON(out))
				}
				if strings.Contains(string(mustJSON(out)), "hunter2") || strings.Contains(string(mustJSON(out)), "SELECT") || out.Error.Details != nil {
					t.Fatalf("%s leaked internal detail: %s", tool, mustJSON(out))
				}
				if e := toolkit.ValidateOutput(tool, mustJSON(out)); e != nil {
					t.Fatalf("%s: %v", tool, e)
				}
			}
		})
	}
	// Details on coded semantic errors are passed through unchanged.
	f := toolkit.Failure(&graph.Error{Code: "revision_conflict", Message: "Node revision changed; read it again.", Details: map[string]any{"current_revision": 3}})
	if f.Error.Details["current_revision"] != 3 || f.Error.Retryable {
		t.Fatalf("semantic error mangled: %+v", f.Error)
	}
}

func TestDefinitionsAreIndependentCopies(t *testing.T) {
	a, e := toolkit.Definitions()
	if e != nil {
		t.Fatal(e)
	}
	a[0].InputSchema["additionalProperties"] = true
	delete(a[0].OutputSchema, "oneOf")
	b, _ := toolkit.Definitions()
	if b[0].InputSchema["additionalProperties"] != false || b[0].OutputSchema["oneOf"] == nil {
		t.Fatal("mutating returned definitions changed the shared contract")
	}
	if toolkit.ValidateInput("graph_overview", []byte(`{"x":1}`)) == nil {
		t.Fatal("shared compiled schema was altered")
	}
}
