package toolkit_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"

	"ns-exploration-graph/graph"
	"ns-exploration-graph/toolkit"
)

func readNormalized(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) // tolerate autocrlf checkouts
}
func loadManifest(t *testing.T) (m struct {
	Tools []struct {
		Name         string         `json:"name"`
		InputSchema  map[string]any `json:"inputSchema"`
		OutputSchema map[string]any `json:"outputSchema"`
	} `json:"tools"`
	Roles map[string][]string `json:"role_presets"`
	Gated map[string][]string `json:"capability_gated_tools"`
}) {
	t.Helper()
	if e := json.Unmarshal(readNormalized(t, "../spec/tool-contracts.json"), &m); e != nil {
		t.Fatal(e)
	}
	return m
}
func walk(v any, fn func(path string, node map[string]any)) {
	var rec func(string, any)
	rec = func(p string, v any) {
		switch x := v.(type) {
		case map[string]any:
			fn(p, x)
			for k, c := range x {
				rec(p+"/"+k, c)
			}
		case []any:
			for _, c := range x {
				rec(p, c)
			}
		}
	}
	rec("", v)
}

// pythonCmd finds a working Python 3 interpreter (stdlib only is needed).
func pythonCmd(t *testing.T) []string {
	t.Helper()
	candidates := [][]string{{"python3"}, {"python"}, {"py", "-3"}}
	if p := os.Getenv("GRAPH_PYTHON"); p != "" {
		candidates = [][]string{strings.Fields(p)}
	}
	for _, c := range candidates {
		if _, e := exec.LookPath(c[0]); e != nil {
			continue
		}
		if out, e := exec.Command(c[0], append(c[1:], "-c", "import sys; assert sys.version_info >= (3, 8)")...).CombinedOutput(); e == nil {
			return c
		} else {
			t.Logf("interpreter %v unusable: %v %s", c, e, out)
		}
	}
	if os.Getenv("CI") != "" || os.Getenv("GRAPH_REQUIRE_PYTHON") == "1" {
		t.Fatal("Python 3 is required (CI/GRAPH_REQUIRE_PYTHON); set GRAPH_PYTHON to override discovery")
	}
	t.Skip("no Python 3 interpreter; the generator drift check also runs in CI")
	return nil
}

func TestEmbeddedContractMatchesSpecAndGenerator(t *testing.T) {
	if !bytes.Equal(readNormalized(t, "../spec/tool-contracts.json"), readNormalized(t, "contracts.json")) {
		t.Fatal("embedded contract drift: run python toolkit/generate_contracts.py")
	}
	py := pythonCmd(t)
	out, e := exec.Command(py[0], append(py[1:], "generate_contracts.py", "--check")...).CombinedOutput()
	if e != nil {
		t.Fatalf("generator drift check failed: %v\n%s", e, out)
	}
}

// The input contract is frozen at G0. Changing it deliberately means changing this
// digest in the same reviewed contract change, together with docs/DECISIONS.md.
const frozenInputDigest = "83ff844da75a2375e80c53514362a89a837b63479f707eb6be1bfe0268b4662c"

func TestFrozenInputContractAndRolePresets(t *testing.T) {
	m := loadManifest(t)
	frozen := map[string]any{"roles": m.Roles, "gated": m.Gated}
	inputs := map[string]any{}
	for _, d := range m.Tools {
		inputs[d.Name] = d.InputSchema
	}
	frozen["inputs"] = inputs
	b, _ := json.Marshal(frozen) // map keys are emitted sorted: canonical enough for a pin
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != frozenInputDigest {
		t.Fatalf("frozen input/role contract changed: digest %s", got)
	}
}

func TestContractNamesCodesAndFormatsMatchSpecs(t *testing.T) {
	m := loadManifest(t)
	tools := readNormalized(t, "../spec/TOOLS.md")
	named := map[string]bool{}
	for _, n := range regexp.MustCompile("(?m)^\\d\\. `(graph_[a-z_]+)`").FindAllStringSubmatch(string(tools), -1) {
		named[n[1]] = true
	}
	if len(m.Tools) != 8 {
		t.Fatalf("expected eight tools, got %d", len(m.Tools))
	}
	seen := map[string]bool{}
	for _, d := range m.Tools {
		if seen[d.Name] || !named[d.Name] {
			t.Fatalf("tool %s duplicated or absent from spec/TOOLS.md", d.Name)
		}
		seen[d.Name] = true
		delete(named, d.Name)
	}
	if len(named) != 0 {
		t.Fatalf("spec/TOOLS.md names tools missing from the contract: %v", named)
	}
	// Error code enum equals the stable list in SPEC.md.
	line := regexp.MustCompile("Stable error codes: (.*)\n").FindSubmatch(readNormalized(t, "../spec/SPEC.md"))
	if line == nil {
		t.Fatal("SPEC.md stable error code list not found")
	}
	want := regexp.MustCompile("`([a-z_]+)`").FindAllStringSubmatch(strings.SplitN(string(line[1]), ". Errors", 2)[0], -1)
	specCodes := []string{}
	for _, w := range want {
		specCodes = append(specCodes, w[1])
	}
	sort.Strings(specCodes)
	for _, d := range m.Tools {
		var codes []string
		walk(d.OutputSchema, func(p string, n map[string]any) {
			if strings.HasSuffix(p, "/properties/code") {
				for _, c := range n["enum"].([]any) {
					codes = append(codes, c.(string))
				}
			}
		})
		sort.Strings(codes)
		if strings.Join(codes, ",") != strings.Join(specCodes, ",") {
			t.Fatalf("%s error codes %v differ from SPEC.md %v", d.Name, codes, specCodes)
		}
	}
	// Only formats the validator is known to assert may appear (an unknown format
	// name would be ignored silently), and the release-required ones must be declared.
	allowed := map[string]bool{"uuid": true, "uri": true, "date-time": true}
	for _, d := range m.Tools {
		for _, schema := range []map[string]any{d.InputSchema, d.OutputSchema} {
			walk(schema, func(_ string, n map[string]any) {
				if f, ok := n["format"].(string); ok && !allowed[f] {
					t.Fatalf("%s declares unasserted format %q", d.Name, f)
				}
			})
		}
		defs := d.OutputSchema["$defs"].(map[string]any)
		format := func(def string) any { return defs[def].(map[string]any)["format"] }
		if format("id") != "uuid" || format("time") != "date-time" || defs["resource"].(map[string]any)["properties"].(map[string]any)["uri"].(map[string]any)["format"] != "uri" {
			t.Fatalf("%s output schema lacks uuid/date-time/uri format declarations", d.Name)
		}
	}
	for _, name := range []string{"graph_get_node", "graph_propose_work", "graph_add_support", "graph_accept_goal"} {
		for _, d := range m.Tools {
			if d.Name == name && !bytes.Contains(mustJSON(d.InputSchema), []byte(`"format":"uuid"`)) {
				t.Fatalf("%s input must assert uuid format", name)
			}
		}
	}
	for _, name := range []string{"graph_record_observation", "graph_record_conclusion"} {
		for _, d := range m.Tools {
			if d.Name == name && !bytes.Contains(mustJSON(d.InputSchema), []byte(`"format":"uri"`)) {
				t.Fatalf("%s input must assert uri format", name)
			}
		}
	}
}

// Real, schema-valid envelope used as a mutation base for format assertions.
func sampleGetNode(t *testing.T, w *world) map[string]any {
	t.Helper()
	x := w.claim(w.work("reproduce"), 1)
	wk := w.worker(x)
	o := obs("Rejected")
	o["resource_refs"] = []any{map[string]any{"resource_type": "test_log", "uri": "fixture://parser/rejection-log"}}
	ob := call(wk, "graph_record_observation", o, "o-1")
	if !ob.OK {
		t.Fatal(ob.Error)
	}
	out := call(wk, "graph_get_node", map[string]any{"node_id": data(t, ob)["observation_id"]}, "")
	if !out.OK {
		t.Fatal(out.Error)
	}
	var m map[string]any
	_ = json.Unmarshal(mustJSON(out), &m)
	return m
}
func clone(m map[string]any) map[string]any {
	var c map[string]any
	_ = json.Unmarshal(mustJSON(m), &c)
	return c
}
func TestOutputSchemaAssertsFormatsAndIsStrict(t *testing.T) {
	eachStore(t, func(t *testing.T, svc *graph.Service) {
		w := newWorld(t, svc)
		base := sampleGetNode(t, w)
		if e := toolkit.ValidateOutput("graph_get_node", mustJSON(base)); e != nil {
			t.Fatalf("real output rejected: %v", e)
		}
		d := func(m map[string]any) map[string]any { return m["data"].(map[string]any) }
		edge := func(m map[string]any) map[string]any { return d(m)["edges"].([]any)[0].(map[string]any) }
		res := func(m map[string]any) map[string]any { return d(m)["resource_refs"].([]any)[0].(map[string]any) }
		mutations := map[string]func(map[string]any){
			"uuid node id":            func(m map[string]any) { d(m)["id"] = "not-a-uuid" },
			"uuid edge id":            func(m map[string]any) { edge(m)["id"] = "1234" },
			"uuid resource node":      func(m map[string]any) { res(m)["node_id"] = "nope" },
			"date-time edge":          func(m map[string]any) { edge(m)["created_at"] = "yesterday" },
			"date-time revision":      func(m map[string]any) { d(m)["node"].(map[string]any)["created_at"] = "2026-13-45 25:61" },
			"date-time resource":      func(m map[string]any) { res(m)["created_at"] = "" },
			"uri resource":            func(m map[string]any) { res(m)["uri"] = "not a URI" },
			"relative uri":            func(m map[string]any) { res(m)["uri"] = "relative/path" },
			"unknown top-level field": func(m map[string]any) { m["extra"] = true },
			"unknown data field":      func(m map[string]any) { d(m)["actor_id"] = "spoof" },
			"unknown nested field":    func(m map[string]any) { edge(m)["host_note"] = "x" },
			"missing data":            func(m map[string]any) { delete(m, "data") },
			"missing graph revision":  func(m map[string]any) { delete(m, "graph_revision") },
			"zero graph revision":     func(m map[string]any) { m["graph_revision"] = 0 },
			"ok true with error": func(m map[string]any) {
				m["error"] = map[string]any{"code": "not_found", "message": "m", "retryable": false}
			},
			"wrong freshness":           func(m map[string]any) { d(m)["freshness"] = "unknown" },
			"state incompatible w kind": func(m map[string]any) { d(m)["node"].(map[string]any)["state"] = "running" },
			"payload missing epistemics": func(m map[string]any) {
				delete(d(m)["node"].(map[string]any)["payload"].(map[string]any), "epistemic_status")
			},
		}
		for name, mutate := range mutations {
			m := clone(base)
			mutate(m)
			if e := toolkit.ValidateOutput("graph_get_node", mustJSON(m)); e == nil {
				t.Errorf("%s: invalid output accepted: %s", name, mustJSON(m))
			}
		}
		// A schema belongs to exactly one tool, and failure envelopes are closed.
		if e := toolkit.ValidateOutput("graph_list_nodes", mustJSON(base)); e == nil {
			t.Error("get_node output accepted by list_nodes schema")
		}
		fail := map[string]any{"ok": false, "error": map[string]any{"code": "not_found", "message": "Node not found in graph.", "retryable": false, "details": map[string]any{"current_revision": 3}}}
		if e := toolkit.ValidateOutput("graph_get_node", mustJSON(fail)); e != nil {
			t.Errorf("valid failure envelope rejected: %v", e)
		}
		for name, bad := range map[string]func(map[string]any){
			"unknown code":        func(m map[string]any) { m["error"].(map[string]any)["code"] = "sql_error" },
			"extra error field":   func(m map[string]any) { m["error"].(map[string]any)["sql"] = "SELECT 1" },
			"failure with data":   func(m map[string]any) { m["data"] = map[string]any{} },
			"missing retryable":   func(m map[string]any) { delete(m["error"].(map[string]any), "retryable") },
			"failure ok missing":  func(m map[string]any) { delete(m, "ok") },
			"empty error message": func(m map[string]any) { m["error"].(map[string]any)["message"] = "" },
		} {
			m := clone(fail)
			bad(m)
			if e := toolkit.ValidateOutput("graph_get_node", mustJSON(m)); e == nil {
				t.Errorf("%s: invalid failure accepted", name)
			}
		}
		if e := toolkit.ValidateOutput("graph_get_node", append(mustJSON(fail), []byte(` {}`)...)); e == nil {
			t.Error("trailing JSON accepted")
		}
		if e := toolkit.ValidateOutput("graph_nope", mustJSON(fail)); e == nil {
			t.Error("unknown tool accepted")
		}
	})
}
