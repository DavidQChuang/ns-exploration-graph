package toolkit_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"ns-exploration-graph/graph"
	"ns-exploration-graph/toolkit"
)

var allTools = []string{"graph_overview", "graph_get_node", "graph_list_nodes", "graph_propose_work", "graph_record_observation", "graph_record_conclusion", "graph_add_support", "graph_accept_goal"}

func ref(id string, rev int) map[string]any { return map[string]any{"id": id, "revision": rev} }

// valid returns one schema-valid argument set per tool.
func valid(id, edge string) map[string]any {
	return map[string]any{
		"graph_overview":           map[string]any{"byte_budget": 4096},
		"graph_get_node":           map[string]any{"node_id": id, "revision": 1},
		"graph_list_nodes":         map[string]any{"kind": "goal", "limit": 5},
		"graph_propose_work":       map[string]any{"title": "t", "instruction": "i", "priority": 5, "parents": []any{ref(id, 1)}},
		"graph_record_observation": map[string]any{"title": "t", "statement": "s", "evidence": "e", "epistemic_status": "observed", "resource_refs": []any{map[string]any{"resource_type": "log", "uri": "file:///x.log"}}},
		"graph_record_conclusion":  map[string]any{"title": "t", "statement": "s", "epistemic_status": "inferred", "supports": []any{ref(id, 1)}},
		"graph_add_support":        map[string]any{"source": ref(id, 1), "target": ref(id, 1)},
		"graph_accept_goal":        map[string]any{"goal": ref(id, 1), "support_edge_ids": []any{edge}, "rationale": "r"},
	}
}

// ok asserts a successful envelope whose JSON validates against the tool's result schema.
func ok(t *testing.T, name string, out toolkit.Envelope) map[string]any {
	t.Helper()
	if !out.OK {
		t.Fatalf("%s failed: %+v", name, out.Error)
	}
	if e := toolkit.ValidateOutput(name, mustJSON(out)); e != nil {
		t.Fatalf("%s result violates its schema: %v\n%s", name, e, mustJSON(out))
	}
	return data(t, out)
}

// fails asserts a stable error envelope (schema-valid) and that the graph did not change.
func (w *world) fails(name string, out toolkit.Envelope, code string, before int64) {
	w.t.Helper()
	wantCode(w.t, out, code)
	if e := toolkit.ValidateOutput(name, mustJSON(out)); e != nil {
		w.t.Fatalf("%s error envelope violates schema: %v\n%s", name, e, mustJSON(out))
	}
	if after := w.revision(); after != before {
		w.t.Fatalf("%s: failed write changed graph revision %d -> %d", name, before, after)
	}
}

func TestAllEightToolsAgainstRealService(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		planner := w.adapter(nil)
		ov := ok(t, "graph_overview", call(planner, "graph_overview", map[string]any{}, ""))
		if len(ov["goals"].([]any)) != 1 || ov["origin"] == nil {
			t.Fatalf("overview lacks anchors %v", ov)
		}
		origin := ov["origin"].(map[string]any)
		page := ok(t, "graph_list_nodes", call(planner, "graph_list_nodes", map[string]any{"limit": 1}, ""))
		if len(page["nodes"].([]any)) != 1 || page["next_cursor"] == nil || page["total"].(float64) != 2 {
			t.Fatalf("pagination %v", page)
		}
		next := ok(t, "graph_list_nodes", call(planner, "graph_list_nodes", map[string]any{"limit": 1, "cursor": page["next_cursor"]}, ""))
		if len(next["nodes"].([]any)) != 1 || next["next_cursor"] != nil {
			t.Fatalf("second page %v", next)
		}
		prop := ok(t, "graph_propose_work", call(planner, "graph_propose_work", map[string]any{"title": "Reproduce", "instruction": "Run twelve through the parser.", "priority": 50, "parents": []any{ref(origin["id"].(string), 1)}}, "propose-1"))
		wid := prop["work_item_id"].(string)
		if len(prop["lineage_edge_ids"].([]any)) != 1 || prop["node_revision"].(float64) != 1 {
			t.Fatalf("proposal %v", prop)
		}
		x := w.claim(wid, 1)
		worker := w.worker(x)
		ok(t, "graph_overview", call(worker, "graph_overview", map[string]any{"byte_budget": 8192}, ""))
		wn := ok(t, "graph_get_node", call(worker, "graph_get_node", map[string]any{"node_id": wid}, ""))
		if wn["current_revision"].(float64) != 2 {
			t.Fatalf("assigned work not running %v", wn)
		}
		o := obs("Malformed input rejected")
		o["resource_refs"] = []any{map[string]any{"resource_type": "test_log", "uri": "fixture://parser/rejection-log", "sha256": strings.Repeat("a", 64)}}
		ob := ok(t, "graph_record_observation", call(worker, "graph_record_observation", o, "obs-1"))
		oid := ob["observation_id"].(string)
		if len(ob["resource_reference_ids"].([]any)) != 1 || ob["yields_edge_id"] == nil {
			t.Fatalf("observation %v", ob)
		}
		con := ok(t, "graph_record_conclusion", call(worker, "graph_record_conclusion", map[string]any{"title": "Rejection verified", "statement": "Malformed text is rejected.", "epistemic_status": "inferred", "supports": []any{ref(oid, 1)}}, "con-1"))
		cid := con["conclusion_id"].(string)
		if con["yields_edge_id"] == nil || len(con["support_edge_ids"].([]any)) != 1 {
			t.Fatalf("bound conclusion lacks producer/support edges %v", con)
		}
		w.transition(wid, 2, "done")
		nv := ok(t, "graph_get_node", call(planner, "graph_get_node", map[string]any{"node_id": oid, "revision": 1}, ""))
		if len(nv["resource_refs"].([]any)) != 1 || nv["freshness"] != "fresh" {
			t.Fatalf("evidence retrieval %v", nv)
		}
		ok(t, "graph_list_nodes", call(planner, "graph_list_nodes", map[string]any{"kind": "observation", "state": "active"}, ""))
		goal := w.goal
		sup := ok(t, "graph_add_support", call(planner, "graph_add_support", map[string]any{"source": ref(cid, 1), "target": ref(goal.NodeID, 1)}, "sup-1"))
		granted := w.adapter(func(b *toolkit.Binding) { b.AcceptGoal, b.AcceptanceMethod = true, "validator" }, func(c *toolkit.Config) {
			c.AcceptancePolicy = func(context.Context, toolkit.Binding, graph.AcceptGoalInput) error { return nil }
		})
		acc := ok(t, "graph_accept_goal", call(granted, "graph_accept_goal", map[string]any{"goal": ref(goal.NodeID, 1), "support_edge_ids": []any{sup["edge_id"]}, "rationale": "Reviewed the stored rejection log."}, "accept-1"))
		if acc["resulting_goal_revision"].(float64) != 2 || len(acc["reviewed_evidence"].([]any)) != 2 || len(acc["reviewed_resource_ids"].([]any)) != 1 {
			t.Fatalf("acceptance closure %v", acc)
		}
		// The bound producer and actor came from the host binding, not tool arguments.
		s := w.snapshot()
		for _, e := range s.Edges {
			if e.Relation == "yields" && e.SourceID != wid {
				t.Fatalf("yields edge from %s, not the bound work item", e.SourceID)
			}
			if e.ActorID != w.b.ActorID {
				t.Fatalf("edge actor %q is not the bound actor", e.ActorID)
			}
		}
		if len(s.Acceptances) != 1 || s.Acceptances[0].Method != "validator" || s.Acceptances[0].ActorID != w.b.ActorID {
			t.Fatalf("acceptance %+v", s.Acceptances)
		}
	})
}

func TestStrictSchemasBoundsAndUnknownFields(t *testing.T) {
	id, edge := graph.NewID(), graph.NewID()
	good := valid(id, edge)
	for _, n := range allTools {
		b := mustJSON(good[n])
		if e := toolkit.ValidateInput(n, b); e != nil {
			t.Fatalf("baseline %s invalid: %v", n, e)
		}
		// Unknown fields are rejected at the top level and in every nested object, never discarded.
		for _, extra := range []string{"graph_id", "namespace", "actor_id", "actor", "execution_id", "work_item_id", "producer", "producer_id", "method", "request_id", "invocation_id", "role", "expected_graph_revision", "kind_override", "edge_type"} {
			m := clone(good[n].(map[string]any))
			m[extra] = graph.NewID()
			if toolkit.ValidateInput(n, mustJSON(m)) == nil {
				t.Errorf("%s accepted top-level %s", n, extra)
			}
		}
		walk(good[n], func(p string, node map[string]any) {
			if p == "" {
				return
			}
			c := clone(good[n].(map[string]any))
			var target map[string]any
			walkClone(c, p, func(m map[string]any) { target = m })
			if target != nil {
				target["actor_id"] = "spoof"
				if toolkit.ValidateInput(n, mustJSON(c)) == nil {
					t.Errorf("%s accepted nested field at %s", n, p)
				}
			}
		})
		for _, bad := range []string{``, `null`, `[]`, `"x"`, `{`, string(b) + ` {}`} {
			if toolkit.ValidateInput(n, []byte(bad)) == nil {
				t.Errorf("%s accepted malformed document %q", n, bad)
			}
		}
	}
	long := func(n int) string { return strings.Repeat("x", n) }
	uuids := func(n int) []any {
		out := []any{}
		for i := 0; i < n; i++ {
			out = append(out, graph.NewID())
		}
		return out
	}
	refs := func(n int) []any {
		out := []any{}
		for i := 0; i < n; i++ {
			out = append(out, ref(graph.NewID(), 1))
		}
		return out
	}
	resources := func(n int) []any {
		out := []any{}
		for i := 0; i < n; i++ {
			out = append(out, map[string]any{"resource_type": "log", "uri": fmt.Sprintf("file:///%d.log", i)})
		}
		return out
	}
	with := func(tool string, k string, v any) map[string]any {
		m := clone(good[tool].(map[string]any))
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
		return map[string]any{"tool": tool, "args": m}
	}
	type bound struct {
		name  string
		input map[string]any
		valid bool
	}
	cases := []bound{
		{"byte_budget min", with("graph_overview", "byte_budget", 1024), true},
		{"byte_budget max", with("graph_overview", "byte_budget", 65536), true},
		{"byte_budget below", with("graph_overview", "byte_budget", 1023), false},
		{"byte_budget above", with("graph_overview", "byte_budget", 65537), false},
		{"byte_budget float", with("graph_overview", "byte_budget", 2048.5), false},
		{"byte_budget string", with("graph_overview", "byte_budget", "2048"), false},
		{"revision max", with("graph_get_node", "revision", 2147483647), true},
		{"revision zero", with("graph_get_node", "revision", 0), false},
		{"revision overflow", with("graph_get_node", "revision", 2147483648), false},
		{"node_id missing", with("graph_get_node", "node_id", nil), false},
		{"node_id not uuid", with("graph_get_node", "node_id", "../etc/passwd"), false},
		{"related_cursor max", with("graph_get_node", "related_cursor", long(8192)), true},
		{"related_cursor over", with("graph_get_node", "related_cursor", long(8193)), false},
		{"related_cursor empty", with("graph_get_node", "related_cursor", ""), false},
		{"limit min", with("graph_list_nodes", "limit", 1), true},
		{"limit max", with("graph_list_nodes", "limit", 100), true},
		{"limit zero", with("graph_list_nodes", "limit", 0), false},
		{"limit over", with("graph_list_nodes", "limit", 101), false},
		{"kind arbitrary", with("graph_list_nodes", "kind", "edge"), false},
		{"state arbitrary", with("graph_list_nodes", "state", "deleted"), false},
		{"title max", with("graph_propose_work", "title", long(512)), true},
		{"title over", with("graph_propose_work", "title", long(513)), false},
		{"title empty", with("graph_propose_work", "title", ""), false},
		{"instruction max", with("graph_propose_work", "instruction", long(8192)), true},
		{"instruction over", with("graph_propose_work", "instruction", long(8193)), false},
		{"priority max", with("graph_propose_work", "priority", 100), true},
		{"priority over", with("graph_propose_work", "priority", 101), false},
		{"priority negative", with("graph_propose_work", "priority", -1), false},
		{"priority missing", with("graph_propose_work", "priority", nil), false},
		{"parents max", with("graph_propose_work", "parents", refs(100)), true},
		{"parents over", with("graph_propose_work", "parents", refs(101)), false},
		{"parent without revision", with("graph_propose_work", "parents", []any{map[string]any{"id": id}}), false},
		{"parents duplicate", with("graph_propose_work", "parents", []any{ref(id, 1), ref(id, 1)}), false},
		{"observation evidence missing", with("graph_record_observation", "evidence", nil), false},
		{"observation evidence empty", with("graph_record_observation", "evidence", ""), false},
		{"observation status invalid", with("graph_record_observation", "epistemic_status", "verified"), false},
		{"observation polarity invalid", with("graph_record_observation", "polarity", "mixed"), false},
		{"observation polarity negative", with("graph_record_observation", "polarity", "negative"), true},
		{"observation resources max", with("graph_record_observation", "resource_refs", resources(100)), true},
		{"observation resources over", with("graph_record_observation", "resource_refs", resources(101)), false},
		{"observation relative uri", with("graph_record_observation", "resource_refs", []any{map[string]any{"resource_type": "log", "uri": "x.log"}}), false},
		{"observation bad sha256", with("graph_record_observation", "resource_refs", []any{map[string]any{"resource_type": "log", "uri": "file:///x", "sha256": "ABC"}}), false},
		{"conclusion supports missing", with("graph_record_conclusion", "supports", nil), false},
		{"conclusion supports max", with("graph_record_conclusion", "supports", refs(100)), true},
		{"conclusion supports over", with("graph_record_conclusion", "supports", refs(101)), false},
		{"conclusion status invalid", with("graph_record_conclusion", "epistemic_status", "proven"), false},
		{"support revision zero", with("graph_add_support", "source", ref(id, 0)), false},
		{"support missing target", with("graph_add_support", "target", nil), false},
		{"accept edges max", with("graph_accept_goal", "support_edge_ids", uuids(100)), true},
		{"accept edges over", with("graph_accept_goal", "support_edge_ids", uuids(101)), false},
		{"accept edges empty", with("graph_accept_goal", "support_edge_ids", []any{}), false},
		{"accept edges duplicate", with("graph_accept_goal", "support_edge_ids", []any{edge, edge}), false},
		{"accept rationale missing", with("graph_accept_goal", "rationale", nil), false},
		{"accept rationale max", with("graph_accept_goal", "rationale", long(8192)), true},
		{"accept rationale over", with("graph_accept_goal", "rationale", long(8193)), false},
	}
	for _, c := range cases {
		e := toolkit.ValidateInput(c.input["tool"].(string), mustJSON(c.input["args"]))
		if (e == nil) != c.valid {
			t.Errorf("%s: valid=%v but validator said %v", c.name, c.valid, e)
		}
	}
}

// walkClone calls fn with the object addressed by a walk() path (a "/"-joined key path;
// arrays are traversed transparently, first element only).
func walkClone(v any, path string, fn func(map[string]any)) {
	cur := v
	for _, k := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if a, ok := cur.([]any); ok && len(a) > 0 {
			cur = a[0]
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return
		}
		cur = m[k]
	}
	if a, ok := cur.([]any); ok && len(a) > 0 {
		cur = a[0]
	}
	if m, ok := cur.(map[string]any); ok {
		fn(m)
	}
}

func TestInvalidInputIsRejectedBeforeServiceEntry(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		x := w.claim(w.work("assigned"), 1)
		policyCalls := 0
		solo := w.adapter(func(b *toolkit.Binding) {
			b.Role, b.Execution, b.AcceptGoal, b.AcceptanceMethod = "solo", x, true, "validator"
		}, func(c *toolkit.Config) {
			c.AcceptancePolicy = func(context.Context, toolkit.Binding, graph.AcceptGoalInput) error { policyCalls++; return nil }
		})
		before := w.revision()
		good := valid(w.goal.NodeID, graph.NewID())
		for _, n := range allTools {
			for _, key := range []string{"graph_id", "actor_id", "execution_id", "work_item_id", "producer_id", "namespace", "method", "request_id"} {
				m := clone(good[n].(map[string]any))
				m[key] = graph.NewID()
				w.fails(n, call(solo, n, m, "spoof-"+n+key), "invalid_argument", before)
			}
			w.fails(n, call(solo, n, []any{}, "array-"+n), "invalid_argument", before)
		}
		if policyCalls != 0 {
			t.Fatal("acceptance policy ran on schema-invalid input")
		}
	})
}

func TestInputByteLimitBoundary(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		pad := func(n int) []byte { return []byte(`{"limit":1}` + strings.Repeat(" ", n)) }
		limits := toolkit.Limits{InputBytes: 100, ResultBytes: svc.Limits.ResultBytes, OverviewBytes: 16384}
		a := w.adapter(nil, func(c *toolkit.Config) { c.Limits = limits })
		at := a.Invoke(w.ctx, "graph_list_nodes", pad(100-len(`{"limit":1}`)), "")
		if !at.OK {
			t.Fatalf("input exactly at the limit rejected: %+v", at.Error)
		}
		before := w.revision()
		over := a.Invoke(w.ctx, "graph_list_nodes", pad(100-len(`{"limit":1}`)+1), "")
		w.fails("graph_list_nodes", over, "limit_exceeded", before)
		huge := a.Invoke(w.ctx, "graph_propose_work", mustJSON(map[string]any{"title": "t", "instruction": strings.Repeat("x", 8000), "priority": 1}), "big-1")
		w.fails("graph_propose_work", huge, "limit_exceeded", before)
	})
}

func TestRolePresetsAndOptInAcceptance(t *testing.T) {
	m := loadManifest(t)
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		x := w.claim(w.work("assigned"), 1)
		policy := func(c *toolkit.Config) {
			c.AcceptancePolicy = func(context.Context, toolkit.Binding, graph.AcceptGoalInput) error { return nil }
		}
		names := func(a *toolkit.Adapter) []string {
			out := []string{}
			for _, d := range a.Definitions() {
				out = append(out, d.Name)
				if d.OutputSchema == nil || d.InputSchema == nil {
					t.Fatalf("%s lacks schemas", d.Name)
				}
			}
			sort.Strings(out)
			return out
		}
		for _, role := range []string{"planner", "worker", "solo"} {
			want := append([]string{}, m.Roles[role]...)
			sort.Strings(want)
			plain := w.adapter(func(b *toolkit.Binding) { b.Role, b.Execution = role, x })
			if got := names(plain); strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("%s preset %v, contract %v", role, got, want)
			}
			granted := w.adapter(func(b *toolkit.Binding) {
				b.Role, b.Execution, b.AcceptGoal, b.AcceptanceMethod = role, x, true, "human"
			}, policy)
			if got := names(granted); !equalSets(got, append(append([]string{}, want...), "graph_accept_goal")) {
				t.Fatalf("%s with grant %v", role, got)
			}
			// Everything outside the preset is refused on direct invocation, even though a
			// client could send the tool name: discovery is not the authorization boundary.
			allowed := map[string]bool{}
			for _, n := range want {
				allowed[n] = true
			}
			for _, n := range allTools {
				if allowed[n] {
					continue
				}
				before := w.revision()
				w.fails(n, call(plain, n, valid(w.goal.NodeID, graph.NewID())[n], "deny-"+n), "permission_denied", before)
			}
		}
		// Solo is exactly the union of planner and worker tools; acceptance is never implied.
		union := map[string]bool{}
		for _, n := range append(append([]string{}, m.Roles["planner"]...), m.Roles["worker"]...) {
			union[n] = true
		}
		if len(union) != len(m.Roles["solo"]) {
			t.Fatalf("solo %v is not planner+worker %v", m.Roles["solo"], union)
		}
		for _, n := range m.Roles["solo"] {
			if !union[n] || n == "graph_accept_goal" {
				t.Fatalf("solo contains %s", n)
			}
		}
		// Acceptance grant needs method and host policy; model cannot supply either.
		bind := func(f func(*toolkit.Binding)) toolkit.Config {
			b := w.b
			f(&b)
			return toolkit.Config{Binding: b}
		}
		accept := func(b *toolkit.Binding) { b.AcceptGoal = true }
		for name, c := range map[string]toolkit.Config{
			"no method or policy": bind(accept),
			"no policy":           bind(func(b *toolkit.Binding) { accept(b); b.AcceptanceMethod = "validator" }),
			"no method":           func() toolkit.Config { c := bind(accept); policy(&c); return c }(),
			"unknown method": func() toolkit.Config {
				c := bind(func(b *toolkit.Binding) { accept(b); b.AcceptanceMethod = "root" })
				policy(&c)
				return c
			}(),
			"unknown role":      bind(func(b *toolkit.Binding) { b.Role = "admin" }),
			"missing actor":     bind(func(b *toolkit.Binding) { b.ActorID = "" }),
			"missing namespace": bind(func(b *toolkit.Binding) { b.Namespace = "" }),
			"bad graph id":      bind(func(b *toolkit.Binding) { b.GraphID = "graph" }),
			"bad execution":     bind(func(b *toolkit.Binding) { b.Execution = &graph.Execution{WorkItemID: "w", ExecutionID: "x"} }),
			"result limit drift": func() toolkit.Config {
				c := bind(func(*toolkit.Binding) {})
				c.Limits = toolkit.Limits{1000, 2048, 1024}
				return c
			}(),
		} {
			if _, e := toolkit.New(svc, c); e == nil {
				t.Errorf("adapter accepted unsafe configuration: %s", name)
			}
		}
		if _, e := toolkit.New(nil, bind(func(*toolkit.Binding) {})); e == nil {
			t.Error("adapter accepted nil service")
		}
		// Host acceptance policy can refuse; the model cannot override it, and a refusal changes nothing.
		denied := w.adapter(func(b *toolkit.Binding) { b.AcceptGoal, b.AcceptanceMethod = true, "agent" }, func(c *toolkit.Config) {
			c.AcceptancePolicy = func(context.Context, toolkit.Binding, graph.AcceptGoalInput) error {
				return graph.Err("permission_denied", "host policy refuses")
			}
		})
		before := w.revision()
		w.fails("graph_accept_goal", call(denied, "graph_accept_goal", valid(w.goal.NodeID, graph.NewID())["graph_accept_goal"], "acc"), "permission_denied", before)
		// Binding returned by the adapter is a copy: callers cannot widen a live session.
		b := denied.Binding()
		b.Role, b.AcceptGoal = "solo", false
		if denied.Binding().Role != "planner" || !denied.Binding().AcceptGoal {
			t.Fatal("adapter binding was mutable through Binding()")
		}
	})
}
func equalSets(a, b []string) bool {
	x, y := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	return strings.Join(x, ",") == strings.Join(y, ",")
}

func TestInventedIdentifiersAndRevisionsAreRejected(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		x := w.claim(w.work("assigned"), 1)
		solo := w.adapter(func(b *toolkit.Binding) {
			b.Role, b.Execution, b.AcceptGoal, b.AcceptanceMethod = "solo", x, true, "validator"
		}, func(c *toolkit.Config) {
			c.AcceptancePolicy = func(context.Context, toolkit.Binding, graph.AcceptGoalInput) error { return nil }
		})
		ob := ok(t, "graph_record_observation", call(solo, "graph_record_observation", obs("real"), "real-obs"))
		oid := ob["observation_id"].(string)
		invented := graph.NewID()
		before := w.revision()
		cases := []struct {
			tool string
			args map[string]any
			code string
		}{
			{"graph_get_node", map[string]any{"node_id": invented}, "not_found"},
			{"graph_get_node", map[string]any{"node_id": oid, "revision": 99}, ""},
			{"graph_propose_work", map[string]any{"title": "t", "instruction": "i", "priority": 1, "parents": []any{ref(invented, 1)}}, "not_found"},
			{"graph_propose_work", map[string]any{"title": "t", "instruction": "i", "priority": 1, "parents": []any{ref(oid, 7)}}, "revision_conflict"},
			{"graph_record_conclusion", map[string]any{"title": "t", "statement": "s", "epistemic_status": "inferred", "supports": []any{ref(invented, 1)}}, "not_found|stale_evidence"},
			{"graph_record_conclusion", map[string]any{"title": "t", "statement": "s", "epistemic_status": "inferred", "supports": []any{ref(oid, 2)}}, "revision_conflict|stale_evidence"},
			{"graph_add_support", map[string]any{"source": ref(invented, 1), "target": ref(w.goal.NodeID, 1)}, "not_found|stale_evidence"},
			{"graph_add_support", map[string]any{"source": ref(oid, 1), "target": ref(invented, 1)}, "not_found"},
			{"graph_add_support", map[string]any{"source": ref(oid, 1), "target": ref(w.goal.NodeID, 5)}, "revision_conflict"},
			{"graph_accept_goal", map[string]any{"goal": ref(invented, 1), "support_edge_ids": []any{graph.NewID()}, "rationale": "r"}, "not_found"},
			{"graph_accept_goal", map[string]any{"goal": ref(w.goal.NodeID, 1), "support_edge_ids": []any{invented}, "rationale": "r"}, "stale_evidence"},
		}
		for i, c := range cases {
			out := call(solo, c.tool, c.args, fmt.Sprintf("invent-%d", i))
			if c.code == "" { // a revision beyond the node's latest: any stable conflict/not_found is acceptable, but never success
				if out.OK {
					t.Fatalf("%s accepted invented revision", c.tool)
				}
				continue
			}
			// Unknown evidence is reported as not_found or stale_evidence depending on the lookup path; both are stable rejections.
			code := c.code
			if out.Error != nil && strings.Contains("|"+c.code+"|", "|"+out.Error.Code+"|") {
				code = out.Error.Code
			}
			w.fails(c.tool, out, code, before)
		}
	})
}

func TestIdempotentRetryAuthorizationAndIdentity(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		args := map[string]any{"title": "Reproduce", "instruction": "Run it", "priority": 1}
		a := w.adapter(nil)
		first := call(a, "graph_propose_work", args, "durable-1")
		ok(t, "graph_propose_work", first)
		after := w.revision()
		// Same identity and arguments: exactly the original envelope, no second event.
		retry := call(a, "graph_propose_work", args, "durable-1")
		if string(mustJSON(first)) != string(mustJSON(retry)) || w.revision() != after {
			t.Fatalf("retry changed result or graph\n%s\n%s", mustJSON(first), mustJSON(retry))
		}
		// Same identity, different arguments: explicit conflict, not a silent overwrite or replay.
		other := map[string]any{"title": "Different", "instruction": "Run it", "priority": 1}
		w.fails("graph_propose_work", call(a, "graph_propose_work", other, "durable-1"), "idempotency_conflict", after)
		// A new invocation identity is a new operation even for identical arguments.
		second := call(a, "graph_propose_work", args, "durable-2")
		if data(t, second)["work_item_id"] == data(t, first)["work_item_id"] {
			t.Fatal("distinct invocation identity was deduplicated")
		}
		after = w.revision()
		// Another actor presenting the first invocation identity cannot obtain or replay the first result.
		eve := w.adapter(func(b *toolkit.Binding) { b.ActorID = "eve" })
		stolen := call(eve, "graph_propose_work", args, "durable-1")
		if stolen.OK && data(t, stolen)["work_item_id"] == data(t, first)["work_item_id"] {
			t.Fatal("different actor replayed another actor's invocation")
		}
		after = w.revision()
		// A retry is authorized again: revoked host authorization, narrowed role, or lost grant all deny it.
		revoked := w.adapter(nil, func(c *toolkit.Config) {
			c.Authorize = func(context.Context, toolkit.Binding, string) error { return graph.Err("permission_denied", "revoked") }
		})
		w.fails("graph_propose_work", call(revoked, "graph_propose_work", args, "durable-1"), "permission_denied", after)
		reader := w.adapter(func(b *toolkit.Binding) { b.Role = "worker" })
		w.fails("graph_propose_work", call(reader, "graph_propose_work", args, "durable-1"), "permission_denied", after)
		// Missing or oversized invocation identity never reaches the service.
		w.fails("graph_propose_work", call(a, "graph_propose_work", args, ""), "invalid_argument", after)
		w.fails("graph_propose_work", call(a, "graph_propose_work", args, strings.Repeat("i", 201)), "invalid_argument", after)
		if r := call(a, "graph_propose_work", args, strings.Repeat("i", 200)); !r.OK {
			t.Fatalf("200-character identity rejected: %+v", r.Error)
		}
		after = w.revision()
		// After the graph closes, the committed retry still returns its original result; new writes cannot.
		w.must(func() error {
			_, e := svc.SetGraphState(w.ctx, w.host(), graph.SetGraphStateInput{State: "closed"})
			return e
		}())
		after = w.revision()
		again := call(a, "graph_propose_work", args, "durable-1")
		if string(mustJSON(again)) != string(mustJSON(first)) {
			t.Fatalf("retry after close differs: %s", mustJSON(again))
		}
		w.fails("graph_propose_work", call(a, "graph_propose_work", args, "durable-new"), "graph_read_only", after)
	})
}

func TestStaleExecutionStopsOldWorker(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		wid := w.work("flaky work")
		old := w.claim(wid, 1)
		oldWorker := w.worker(old)
		first := ok(t, "graph_record_observation", call(oldWorker, "graph_record_observation", obs("First"), "old-1"))
		// Host stops the worker, reopens the work and starts a replacement execution.
		w.transition(wid, 2, "stopped")
		w.transition(wid, 3, "open")
		fresh := w.claim(wid, 4)
		before := w.revision()
		w.fails("graph_record_observation", call(oldWorker, "graph_record_observation", obs("Zombie write"), "old-2"), "stale_execution", before)
		w.fails("graph_record_conclusion", call(oldWorker, "graph_record_conclusion", map[string]any{"title": "t", "statement": "s", "epistemic_status": "inferred", "supports": []any{ref(first["observation_id"].(string), 1)}}, "old-3"), "stale_execution", before)
		// The old worker's committed pre-stop write replays only as the original success (retry contract).
		if replay := call(oldWorker, "graph_record_observation", obs("First"), "old-1"); !replay.OK || data(t, replay)["observation_id"] != first["observation_id"] {
			t.Fatalf("committed retry did not replay: %s", mustJSON(replay))
		}
		newWorker := w.worker(fresh)
		ok(t, "graph_record_observation", call(newWorker, "graph_record_observation", obs("Replacement"), "new-1"))
		// Completed work also fences the execution.
		w.transition(wid, 5, "done")
		w.fails("graph_record_observation", call(newWorker, "graph_record_observation", obs("Late"), "new-2"), "stale_execution", w.revision())
		// A binding naming a work item but a guessed execution identity is also stale.
		guess := w.worker(&graph.Execution{WorkItemID: wid, ExecutionID: graph.NewID()})
		w.fails("graph_record_observation", call(guess, "graph_record_observation", obs("Guess"), "guess-1"), "stale_execution", w.revision())
		// Reads remain available so the stopped worker can return control cleanly.
		ok(t, "graph_overview", call(oldWorker, "graph_overview", map[string]any{}, ""))
	})
}

func TestEmptyResultWorkIsExplicit(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		wid := w.work("search for crash logs")
		x := w.claim(wid, 1)
		worker := w.worker(x)
		before := w.revision()
		// An observation cannot be empty: no silent "nothing happened" evidence.
		for _, evidence := range []string{"", "   "} {
			e := obs("Nothing")
			e["evidence"] = evidence
			w.fails("graph_record_observation", call(worker, "graph_record_observation", e, "empty-"+evidence), "invalid_argument", before)
		}
		// An empty search is recorded as an explicit, queryable negative observation.
		neg := obs("No crash logs found")
		neg["statement"] = "The crash log directory contained zero matching files."
		neg["evidence"] = "find logs/ -name 'crash*' -> 0 results"
		rec := ok(t, "graph_record_observation", call(worker, "graph_record_observation", neg, "negative-1"))
		nv := ok(t, "graph_get_node", call(worker, "graph_get_node", map[string]any{"node_id": rec["observation_id"]}, ""))
		if nv["node"].(map[string]any)["payload"].(map[string]any)["polarity"] != "negative" {
			t.Fatalf("negative result not retained: %v", nv["node"])
		}
		// A worker that produced nothing is distinguishable from one that produced evidence:
		// the work has no outgoing yields edge, so the host can classify it as exhausted, not done.
		silentID := w.work("silent work")
		w.claim(silentID, 1)
		wv := ok(t, "graph_get_node", call(w.adapter(nil), "graph_get_node", map[string]any{"node_id": silentID}, ""))
		for _, e := range wv["edges"].([]any) {
			if e.(map[string]any)["relation"] == "yields" {
				t.Fatalf("silent work unexpectedly yields evidence: %v", e)
			}
		}
		w.transition(silentID, 2, "exhausted")
		lst := ok(t, "graph_list_nodes", call(w.adapter(nil), "graph_list_nodes", map[string]any{"kind": "work_item", "state": "exhausted"}, ""))
		if lst["total"].(float64) != 1 || lst["nodes"].([]any)[0].(map[string]any)["id"] != silentID {
			t.Fatalf("empty-result work not visible as exhausted: %v", lst)
		}
	})
}
