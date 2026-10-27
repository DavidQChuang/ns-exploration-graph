// Package toolkit binds the narrow agent contract to trusted host context.
package toolkit

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed contracts.json prompts/*.md
var assets embed.FS

type Definition struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema"`
}
type manifest struct {
	Tools []Definition        `json:"tools"`
	Roles map[string][]string `json:"role_presets"`
}

var contract struct {
	sync.Once
	manifest
	inputs  map[string]*jsonschema.Schema
	outputs map[string]*jsonschema.Schema
	err     error
}

func load() error {
	contract.Do(func() {
		b, e := assets.ReadFile("contracts.json")
		if e != nil {
			contract.err = e
			return
		}
		if e = json.Unmarshal(b, &contract.manifest); e != nil {
			contract.err = e
			return
		}
		contract.inputs = map[string]*jsonschema.Schema{}
		contract.outputs = map[string]*jsonschema.Schema{}
		for _, d := range contract.Tools {
			for direction, doc := range map[string]map[string]any{"input": d.InputSchema, "output": d.OutputSchema} {
				c := jsonschema.NewCompiler()
				c.AssertFormat()
				u := "https://exploration.invalid/" + d.Name + "/" + direction
				if e = c.AddResource(u, doc); e != nil {
					contract.err = e
					return
				}
				s, e := c.Compile(u)
				if e != nil {
					contract.err = e
					return
				}
				if direction == "input" {
					contract.inputs[d.Name] = s
				} else {
					contract.outputs[d.Name] = s
				}
			}
		}
	})
	return contract.err
}

// Definitions returns independent schema objects, safe for transport registration.
func Definitions() ([]Definition, error) {
	if e := load(); e != nil {
		return nil, e
	}
	b, _ := json.Marshal(contract.Tools)
	var out []Definition
	e := json.Unmarshal(b, &out)
	return out, e
}
func validate(name string, b []byte, output bool) error {
	if e := load(); e != nil {
		return e
	}
	ss := contract.inputs
	if output {
		ss = contract.outputs
	}
	s, ok := ss[name]
	if !ok {
		return fmt.Errorf("unknown tool")
	}
	var v any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if e := d.Decode(&v); e != nil {
		return e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return s.Validate(v)
}
func ValidateInput(name string, b []byte) error  { return validate(name, b, false) }
func ValidateOutput(name string, b []byte) error { return validate(name, b, true) }
func Prompt(role string) (string, error) {
	if e := load(); e != nil {
		return "", e
	}
	if _, ok := contract.Roles[role]; !ok {
		return "", fmt.Errorf("unknown role")
	}
	var out string
	for _, p := range []string{"shared", role, "finish"} {
		b, e := assets.ReadFile("prompts/" + p + ".md")
		if e != nil {
			return "", e
		}
		out += string(b) + "\n"
	}
	return out, nil
}
