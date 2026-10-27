package spec_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"ns-exploration-graph/spec"
)

func compile(t *testing.T) *jsonschema.Schema {
	t.Helper()
	doc, e := jsonschema.UnmarshalJSON(bytes.NewReader(spec.SnapshotSchema))
	if e != nil {
		t.Fatal(e)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat() // uuid, uri and date-time are asserted, not just annotated
	if e = c.AddResource("https://ns-exploration-graph.invalid/snapshot.schema.json", doc); e != nil {
		t.Fatal(e)
	}
	s, e := c.Compile("https://ns-exploration-graph.invalid/snapshot.schema.json")
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func TestExampleSnapshotValidatesAndFormatsAreAsserted(t *testing.T) {
	s := compile(t)
	raw, e := os.ReadFile("example.snapshot.json")
	if e != nil {
		t.Fatal(e)
	}
	load := func() map[string]any {
		var m map[string]any
		if e := json.Unmarshal(raw, &m); e != nil {
			t.Fatal(e)
		}
		return m
	}
	check := func(m map[string]any) error {
		doc, e := jsonschema.UnmarshalJSON(bytes.NewReader(mustJSON(m)))
		if e != nil {
			t.Fatal(e)
		}
		return s.Validate(doc)
	}
	if e := check(load()); e != nil {
		t.Fatalf("example snapshot invalid: %v", e)
	}
	for name, mutate := range map[string]func(map[string]any){
		"invalid uuid": func(m map[string]any) { m["graph"].(map[string]any)["id"] = "invalid" },
		"invalid date": func(m map[string]any) { m["graph"].(map[string]any)["created_at"] = "yesterday" },
		"invalid uri": func(m map[string]any) {
			m["resource_refs"].([]any)[0].(map[string]any)["uri"] = "not a URI"
		},
		"unknown envelope field": func(m map[string]any) { m["unexpected"] = true },
		"empty nodes":            func(m map[string]any) { m["nodes"] = []any{} },
	} {
		m := load()
		mutate(m)
		if check(m) == nil {
			t.Errorf("%s: invalid snapshot accepted", name)
		}
	}
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
