package mcp_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	transport "ns-exploration-graph/adapters/mcp"
)

// The versions the README declares must be the versions the module actually pins and the
// adapter actually advertises; otherwise the documentation has drifted from the code.
func TestDocumentedVersionsMatchModuleAndAdapter(t *testing.T) {
	mod, e := os.ReadFile("../../go.mod")
	if e != nil {
		t.Fatal(e)
	}
	readme, e := os.ReadFile("../../README.md")
	if e != nil {
		t.Fatal(e)
	}
	sdkLine := regexp.MustCompile(`github.com/modelcontextprotocol/go-sdk (v[0-9.]+)`).FindSubmatch(mod)
	goLine := regexp.MustCompile(`(?m)^go ([0-9.]+)$`).FindSubmatch(mod)
	if sdkLine == nil || goLine == nil {
		t.Fatal("go.mod lacks SDK or Go version")
	}
	text := string(readme)
	for _, want := range []string{"go-sdk` " + string(sdkLine[1]), "go " + string(goLine[1]), transport.ProtocolVersion, "2025-11-25"} {
		if !strings.Contains(text, want) {
			t.Errorf("README.md does not state %q", want)
		}
	}
}
