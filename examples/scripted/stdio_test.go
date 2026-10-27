package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Second consumer: a Python (standard library only) MCP client drives the compiled
// graph-mcp over stdio against real PostgreSQL and finishes the failure-to-passing-verification scenario.
// Its persisted end state must equal the in-process (direct and MCP) normalized state.

func goTool() string {
	exe := "go"
	if runtime.GOOS == "windows" {
		exe = "go.exe"
	}
	if p := filepath.Join(runtime.GOROOT(), "bin", exe); fileExists(p) {
		return p
	}
	p, _ := exec.LookPath("go")
	return p
}
func fileExists(p string) bool { _, e := os.Stat(p); return e == nil }

func build(t *testing.T, dir, pkg, name string) string {
	t.Helper()
	out := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command(goTool(), "build", "-o", out, pkg)
	cmd.Dir = "../.."
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("build %s: %v\n%s", pkg, e, b)
	}
	return out
}

func python(t *testing.T) []string {
	t.Helper()
	candidates := [][]string{{"python3"}, {"python"}, {"py", "-3"}}
	if p := os.Getenv("GRAPH_PYTHON"); p != "" {
		candidates = [][]string{strings.Fields(p)}
	}
	for _, c := range candidates {
		if _, e := exec.LookPath(c[0]); e != nil {
			continue
		}
		if e := exec.Command(c[0], append(c[1:], "-c", "import sys; assert sys.version_info >= (3, 8)")...).Run(); e == nil {
			return c
		}
	}
	if os.Getenv("CI") != "" || os.Getenv("GRAPH_REQUIRE_PYTHON") == "1" {
		t.Fatal("Python 3 is required for the second-consumer test (CI/GRAPH_REQUIRE_PYTHON); set GRAPH_PYTHON to override discovery")
	}
	t.Skip("no Python 3 interpreter available; CI runs this test")
	return nil
}

func TestPythonStdioConsumerCompletesParserScenario(t *testing.T) {
	dsn := os.Getenv("GRAPH_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" || os.Getenv("GRAPH_REQUIRE_POSTGRES") == "1" {
			t.Fatal("GRAPH_TEST_DATABASE_URL is required; the stdio consumer test cannot be skipped in CI")
		}
		t.Skip("set GRAPH_TEST_DATABASE_URL to run the stdio consumer test")
	}
	py := python(t)
	dir := t.TempDir()
	mcp := build(t, dir, "./cmd/graph-mcp", "graph-mcp")
	host := build(t, dir, "./examples/scripted", "scripted")
	want, e := run(context.Background(), "direct")
	if e != nil {
		t.Fatal(e)
	}
	for name, extra := range map[string][]string{"with lost-response drill": nil, "without drill": {"--no-drill"}} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			args := append(append([]string{}, py[1:]...), "examples/python/parser_scenario.py", "--graph-mcp", mcp, "--host", host)
			cmd := exec.CommandContext(ctx, py[0], append(args, extra...)...)
			cmd.Dir = "../.."
			cmd.Env = append(os.Environ(), "EXPLORATION_DATABASE_URL="+dsn)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if e := cmd.Run(); e != nil {
				t.Fatalf("python consumer failed: %v\nstderr:\n%s\nstdout:\n%s", e, stderr.String(), stdout.String())
			}
			var got Report
			if e := json.Unmarshal(stdout.Bytes(), &got); e != nil {
				t.Fatalf("report: %v\n%s", e, stdout.String())
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("Python scenario differs from reviewed golden report: %+v", got)
			}
			if !reflect.DeepEqual(got, want) {
				g, _ := json.MarshalIndent(got, "", " ")
				w, _ := json.MarshalIndent(want, "", " ")
				t.Fatalf("stdio/Python state differs from in-process state\npython: %s\ndirect: %s", g, w)
			}
			if name == "with lost-response drill" && !strings.Contains(stderr.String(), "NOT retrying") {
				t.Fatalf("drill did not surface an ambiguous write:\n%s", stderr.String())
			}
		})
	}
}
