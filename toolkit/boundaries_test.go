package toolkit_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ns-exploration-graph/graph"
	"ns-exploration-graph/toolkit"
)

func TestPublishedRuntimeReadDefaultsAndBounds(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		a := w.adapter(nil)
		if a.Limits() != (toolkit.Limits{InputBytes: 256 * 1024, ResultBytes: 64 * 1024, OverviewBytes: 16 * 1024}) {
			t.Fatalf("published tool defaults drifted: %#v", a.Limits())
		}
		for i := 0; i < 23; i++ {
			instruction := "Inspect"
			if i < 3 {
				instruction = strings.Repeat("界", 3000)
			}
			_, e := svc.AddHint(w.ctx, w.host(), graph.AddHintInput{Title: "Hint", HintPayload: graph.HintPayload{Instruction: instruction}})
			w.must(e)
		}
		implicit := call(a, "graph_overview", map[string]any{}, "")
		explicit := call(a, "graph_overview", map[string]any{"byte_budget": 16384}, "")
		ok(t, "graph_overview", implicit)
		ok(t, "graph_overview", explicit)
		if !reflect.DeepEqual(implicit, explicit) || len(mustJSON(implicit)) > 16384 || !data(t, implicit)["truncated"].(bool) {
			t.Fatal("overview omitted budget does not use 16KiB")
		}
		wide := call(a, "graph_overview", map[string]any{"byte_budget": 65536}, "")
		if ok(t, "graph_overview", wide)["truncated"].(bool) || len(mustJSON(wide)) > 65536 {
			t.Fatal("64KiB overview failed to recover optional content")
		}
		for _, budget := range []int{1023, 65537} {
			wantCode(t, call(a, "graph_overview", map[string]any{"byte_budget": budget}, ""), "invalid_argument")
		}
		// The 1KiB value is schema-valid even when this graph's mandatory goal
		// anchors need a larger budget.
		low := call(a, "graph_overview", map[string]any{"byte_budget": 1024}, "")
		if !low.OK {
			wantCode(t, low, "budget_too_small")
		}
		for _, tc := range []struct {
			args  map[string]any
			count int
		}{{map[string]any{}, 20}, {map[string]any{"limit": 1}, 1}, {map[string]any{"limit": 100}, 25}} {
			p := ok(t, "graph_list_nodes", call(a, "graph_list_nodes", tc.args, ""))
			if len(p["nodes"].([]any)) != tc.count || p["total"] != float64(25) || p["omitted"] != float64(25-tc.count) {
				t.Fatalf("list arguments %#v: %#v", tc.args, p)
			}
		}
		wantCode(t, call(a, "graph_list_nodes", map[string]any{"limit": 101}, ""), "invalid_argument")
	})
}

func TestPublishedToolInputBytesAndInvocationCharacters(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		a := w.adapter(nil)
		const prefix = `{"limit":1}`
		at := []byte(prefix + strings.Repeat(" ", 256*1024-len(prefix)))
		ok(t, "graph_list_nodes", a.Invoke(w.ctx, "graph_list_nodes", at, ""))
		before := w.snapshot()
		wantCode(t, a.Invoke(w.ctx, "graph_list_nodes", append(at, ' '), ""), "limit_exceeded")
		mutation := []byte(`{"title":"Boundary","instruction":"Inspect","priority":1}`)
		mutation = append(mutation, []byte(strings.Repeat(" ", 256*1024+1-len(mutation)))...)
		wantCode(t, a.Invoke(w.ctx, "graph_propose_work", mutation, "oversized-input"), "limit_exceeded")
		if !reflect.DeepEqual(before, w.snapshot()) {
			t.Fatal("input rejection changed graph")
		}
		args := map[string]any{"title": "Boundary", "instruction": "Inspect", "priority": 1}
		// A rejected over-size argument has not reserved its invocation key.
		ok(t, "graph_propose_work", call(a, "graph_propose_work", args, "oversized-input"))
		for _, id := range []string{strings.Repeat("a", 200), strings.Repeat("界", 200)} {
			first := call(a, "graph_propose_work", args, id)
			ok(t, "graph_propose_work", first)
			if again := call(a, "graph_propose_work", args, id); !reflect.DeepEqual(first, again) {
				t.Fatal("200-character invocation did not replay")
			}
		}
		for _, id := range []string{strings.Repeat("a", 201), strings.Repeat("界", 201), "invalid-" + string([]byte{0xff})} {
			before := w.snapshot()
			wantCode(t, call(a, "graph_propose_work", args, id), "invalid_argument")
			if !reflect.DeepEqual(before, w.snapshot()) {
				t.Fatal("invalid invocation identity changed graph")
			}
		}
	})
}

func TestToolResultActual64KiBBoundary(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		r, e := svc.AddHint(w.ctx, w.host(), graph.AddHintInput{Title: "Byte boundary", HintPayload: graph.HintPayload{Instruction: "x"}})
		w.must(e)
		id := r.Data["hint_id"].(string)
		args := map[string]any{"node_id": id}
		initial := call(w.adapter(nil), "graph_get_node", args, "")
		ok(t, "graph_get_node", initial)
		base := len(mustJSON(initial))
		snapshot := w.snapshot()
		// Import retains the fixed node timestamps and IDs. Only ASCII payload
		// padding changes, so the complete result can be set to exactly 64KiB.
		for _, extra := range []int{0, 1} {
			snap := snapshot
			snap.Nodes = append([]graph.Node(nil), snapshot.Nodes...)
			for i := range snap.Nodes {
				if snap.Nodes[i].ID != id {
					continue
				}
				snap.Nodes[i].Revisions = append([]graph.Revision(nil), snap.Nodes[i].Revisions...)
				payload, e := json.Marshal(graph.HintPayload{Instruction: "x" + strings.Repeat("p", 65536-base+extra)})
				w.must(e)
				snap.Nodes[i].Revisions[0].Payload = payload
			}
			encoded, e := json.Marshal(snap)
			w.must(e)
			c := w.host()
			c.GraphID = graph.NewID()
			_, e = svc.ImportSnapshot(w.ctx, c, encoded)
			w.must(e)
			a := w.adapter(func(b *toolkit.Binding) { b.GraphID = c.GraphID })
			out := call(a, "graph_get_node", args, "")
			if extra == 0 {
				ok(t, "graph_get_node", out)
				if n := len(mustJSON(out)); n != 65536 {
					t.Fatalf("boundary fixture encoded %d bytes, want 65536", n)
				}
			} else {
				wantCode(t, out, "limit_exceeded")
			}
		}
	})
}
