package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/store/postgres"
	"ns-exploration-graph/toolkit"
)

// These tests build cmd/graph-mcp and run it as a real subprocess over stdio against a real
// PostgreSQL database, driven by the official SDK client.

var (
	buildOnce sync.Once
	binary    string
	buildErr  error
)

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

func graphMCP(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, e := os.MkdirTemp("", "graph-mcp-test")
		if e != nil {
			buildErr = e
			return
		}
		binary = filepath.Join(dir, "graph-mcp")
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		cmd := exec.Command(goTool(), "build", "-o", binary, "./cmd/graph-mcp")
		cmd.Dir = "../.."
		if out, e := cmd.CombinedOutput(); e != nil {
			buildErr = &buildFailure{e, string(out)}
		}
	})
	if buildErr != nil {
		t.Fatalf("cannot build cmd/graph-mcp: %v", buildErr)
	}
	return binary
}

type buildFailure struct {
	err error
	out string
}

func (b *buildFailure) Error() string { return b.err.Error() + "\n" + b.out }

func dsn(t *testing.T) string {
	t.Helper()
	v := os.Getenv("GRAPH_TEST_DATABASE_URL")
	if v == "" {
		if os.Getenv("CI") != "" || os.Getenv("GRAPH_REQUIRE_POSTGRES") == "1" {
			t.Fatal("GRAPH_TEST_DATABASE_URL is required; the stdio server only runs against PostgreSQL and cannot be skipped in CI")
		}
		t.Skip("set GRAPH_TEST_DATABASE_URL to run stdio MCP tests")
	}
	return v
}

type fixture struct {
	t    *testing.T
	svc  *graph.Service
	ns   string
	id   string
	goal graph.Ref
}

func (f *fixture) host() graph.CommandContext {
	return graph.CommandContext{Namespace: f.ns, GraphID: f.id, ActorID: "stdio-host", RequestID: graph.NewID()}
}
func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	store, e := postgres.Open(ctx, dsn(t))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	if e = store.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	f := &fixture{t: t, svc: graph.NewService(store), ns: "stdio-" + graph.NewID(), id: graph.NewID()}
	if _, e = f.svc.CreateGraph(ctx, f.host(), graph.CreateGraphInput{Title: "Parser investigation", Request: "Verify the parser rejects malformed input."}); e != nil {
		t.Fatal(e)
	}
	g, e := f.svc.AddGoal(ctx, f.host(), graph.AddGoalInput{Title: "Rejected", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"retain rejection evidence"}}})
	if e != nil {
		t.Fatal(e)
	}
	f.goal = graph.Ref{NodeID: g.Data["goal_id"].(string), Revision: 1}
	return f
}
func (f *fixture) work(title string) string {
	r, e := f.svc.ProposeWork(context.Background(), f.host(), graph.ProposeWorkInput{Title: title, WorkPayload: graph.WorkPayload{Instruction: "run it", Priority: 1}})
	if e != nil {
		f.t.Fatal(e)
	}
	return r.Data["work_item_id"].(string)
}
func (f *fixture) claim(work string, rev int) string {
	x := graph.NewID()
	if _, e := f.svc.TransitionWork(context.Background(), f.host(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: work, Revision: rev}, State: "running", ExecutionID: x}); e != nil {
		f.t.Fatal(e)
	}
	return x
}
func (f *fixture) snapshot() graph.Snapshot {
	s, e := f.svc.Snapshot(context.Background(), f.ns, f.id)
	if e != nil {
		f.t.Fatal(e)
	}
	return s
}
func (f *fixture) count(kind string) int {
	n := 0
	for _, x := range f.snapshot().Nodes {
		if x.Kind == kind {
			n++
		}
	}
	return n
}

// server is one graph-mcp process plus the SDK client session connected to it.
type server struct {
	cmd    *exec.Cmd
	stderr *bytes.Buffer
	sess   *sdk.ClientSession
}

func (f *fixture) args(extra ...string) []string {
	return append([]string{"-namespace", f.ns, "-graph", f.id, "-actor", "stdio-agent"}, extra...)
}
func (f *fixture) command(t *testing.T, args ...string) (*exec.Cmd, *bytes.Buffer) {
	cmd := exec.Command(graphMCP(t), args...)
	cmd.Env = append(os.Environ(), "EXPLORATION_DATABASE_URL="+dsn(t))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	return cmd, &stderr
}
func (f *fixture) start(t *testing.T, wrap func(sdk.Transport) sdk.Transport, args ...string) *server {
	t.Helper()
	cmd, stderr := f.command(t, args...)
	var tr sdk.Transport = &sdk.CommandTransport{Command: cmd, TerminateDuration: 5 * time.Second}
	if wrap != nil {
		tr = wrap(tr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := sdk.NewClient(&sdk.Implementation{Name: "stdio-conformance", Version: "1"}, nil)
	sess, e := client.Connect(ctx, tr, nil)
	if e != nil {
		t.Fatalf("connect: %v\nstderr: %s", e, stderr)
	}
	s := &server{cmd, stderr, sess}
	t.Cleanup(func() { sess.Close() })
	return s
}

func (s *server) call(t *testing.T, name string, args any) (toolkit.Envelope, *sdk.CallToolResult) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, e := s.sess.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if e != nil {
		t.Fatalf("%s: protocol error %v", name, e)
	}
	b, _ := json.Marshal(r.StructuredContent)
	var out toolkit.Envelope
	if e = json.Unmarshal(b, &out); e != nil {
		t.Fatal(e)
	}
	if r.IsError == out.OK {
		t.Fatalf("%s: MCP isError=%v disagrees with envelope ok=%v", name, r.IsError, out.OK)
	}
	if len(r.Content) != 1 {
		t.Fatalf("%s: missing text content", name)
	}
	if text, ok := r.Content[0].(*sdk.TextContent); !ok || !jsonEqual(b, []byte(text.Text)) {
		t.Fatalf("%s: structured and text content differ", name)
	}
	if e = toolkit.ValidateOutput(name, b); e != nil {
		t.Fatalf("%s: result violates schema: %v\n%s", name, e, b)
	}
	return out, r
}
func jsonEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return bytes.Equal(xb, yb)
}
func dataOf(t *testing.T, out toolkit.Envelope) map[string]any {
	t.Helper()
	if !out.OK {
		t.Fatalf("tool error %+v", out.Error)
	}
	var m map[string]any
	b, _ := json.Marshal(out.Data)
	_ = json.Unmarshal(b, &m)
	return m
}
func wantCode(t *testing.T, out toolkit.Envelope, r *sdk.CallToolResult, code string) {
	t.Helper()
	if out.OK || out.Error == nil || out.Error.Code != code || !r.IsError {
		t.Fatalf("want structured tool error %s, got %+v isError=%v", code, out.Error, r.IsError)
	}
}

// exited waits for the subprocess to end after the session closes and returns its exit state.
func (s *server) exitCode(t *testing.T) int {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for s.cmd.ProcessState == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s.cmd.ProcessState == nil {
		t.Fatal("server process did not exit")
	}
	return s.cmd.ProcessState.ExitCode()
}

func TestStdioDiscoveryCallsErrorsAndCleanShutdown(t *testing.T) {
	f := newFixture(t)
	s := f.start(t, nil, f.args("-role", "planner")...)
	init := s.sess.InitializeResult()
	if init.ServerInfo.Name != "ns-exploration-graph" || !strings.Contains(init.Instructions, "exact returned IDs") || !strings.Contains(init.Instructions, "ambiguous write") {
		t.Fatalf("initialize result %+v", init)
	}
	t.Logf("negotiated MCP protocol version %s; server %s %s", init.ProtocolVersion, init.ServerInfo.Name, init.ServerInfo.Version)
	if init.ProtocolVersion != "2025-11-25" && init.ProtocolVersion != "2026-07-28" {
		t.Fatalf("negotiated unsupported protocol version %q", init.ProtocolVersion)
	}
	ctx := context.Background()
	tools, e := s.sess.ListTools(ctx, &sdk.ListToolsParams{})
	if e != nil {
		t.Fatal(e)
	}
	got := map[string]bool{}
	for _, d := range tools.Tools {
		got[d.Name] = true
		if d.InputSchema == nil || d.OutputSchema == nil {
			t.Fatalf("%s discovered without schemas", d.Name)
		}
	}
	for _, want := range []string{"graph_overview", "graph_get_node", "graph_list_nodes", "graph_propose_work", "graph_record_conclusion", "graph_add_support"} {
		if !got[want] {
			t.Fatalf("planner discovery lacks %s: %v", want, got)
		}
	}
	if len(got) != 6 || got["graph_accept_goal"] || got["graph_record_observation"] {
		t.Fatalf("planner discovery %v", got)
	}
	ov, _ := s.call(t, "graph_overview", map[string]any{})
	if !ov.OK {
		t.Fatal(ov.Error)
	}
	// Valid write reaches PostgreSQL through the stdio process.
	prop, r := s.call(t, "graph_propose_work", map[string]any{"title": "Reproduce", "instruction": "Run twelve through the parser.", "priority": 5})
	if !prop.OK || r.IsError || f.count("work_item") != 1 {
		t.Fatalf("valid write failed: %+v", prop.Error)
	}
	// Invalid calls return structured tool errors, not protocol failures, and change nothing.
	before := f.snapshot().Graph.Revision
	bad, r := s.call(t, "graph_propose_work", map[string]any{"title": "t", "instruction": "i", "priority": 1, "graph_id": graph.NewID()})
	wantCode(t, bad, r, "invalid_argument")
	missing, r := s.call(t, "graph_get_node", map[string]any{"node_id": graph.NewID()})
	wantCode(t, missing, r, "not_found")
	hidden, r := s.call(t, "graph_record_observation", map[string]any{"title": "t", "statement": "s", "evidence": "e", "epistemic_status": "observed"})
	wantCode(t, hidden, r, "permission_denied")
	accept, r := s.call(t, "graph_accept_goal", map[string]any{"goal": map[string]any{"id": f.goal.NodeID, "revision": 1}, "support_edge_ids": []any{graph.NewID()}, "rationale": "x"})
	wantCode(t, accept, r, "permission_denied")
	if f.snapshot().Graph.Revision != before {
		t.Fatal("rejected calls changed the graph")
	}
	// Clean lifecycle shutdown: client closes stdin; the server exits 0 and prints nothing secret.
	if e := s.sess.Close(); e != nil {
		t.Fatalf("close: %v\nstderr: %s", e, s.stderr)
	}
	if code := s.exitCode(t); code != 0 {
		t.Fatalf("exit code %d after stdin close; stderr: %s", code, s.stderr)
	}
	if strings.Contains(s.stderr.String(), "postgres://") {
		t.Fatalf("stderr leaks connection string: %s", s.stderr)
	}
	// State survives the process: a new process sees the committed work.
	again := f.start(t, nil, f.args()...)
	lst, _ := again.call(t, "graph_list_nodes", map[string]any{"kind": "work_item"})
	if dataOf(t, lst)["total"].(float64) != 1 {
		t.Fatalf("committed work lost across processes: %v", lst)
	}
}

func TestStdioWorkerBindingStaleExecutionAndAcceptanceGrant(t *testing.T) {
	f := newFixture(t)
	wid := f.work("assigned")
	x := f.claim(wid, 1)
	w := f.start(t, nil, f.args("-role", "worker", "-work", wid, "-execution", x)...)
	tools, e := w.sess.ListTools(context.Background(), &sdk.ListToolsParams{})
	if e != nil || len(tools.Tools) != 5 {
		t.Fatalf("worker discovery %v %v", tools, e)
	}
	// Spoofed context is rejected; the producer is the host-bound work item.
	spoof, r := w.call(t, "graph_record_observation", map[string]any{"title": "t", "statement": "s", "evidence": "e", "epistemic_status": "observed", "execution_id": graph.NewID(), "work_item_id": wid})
	wantCode(t, spoof, r, "invalid_argument")
	obs, _ := w.call(t, "graph_record_observation", map[string]any{"title": "Rejected", "statement": "twelve rejected", "evidence": "exit 2", "epistemic_status": "observed", "polarity": "negative", "resource_refs": []any{map[string]any{"resource_type": "test_log", "uri": "fixture://parser/rejection-log"}}})
	d := dataOf(t, obs)
	producer := ""
	for _, ed := range f.snapshot().Edges {
		if ed.ID == d["yields_edge_id"] {
			producer = ed.SourceID
			if ed.ActorID != "stdio-agent" {
				t.Fatalf("actor %q is not the bound actor", ed.ActorID)
			}
		}
	}
	if producer != wid {
		t.Fatalf("observation producer %s, want bound work %s", producer, wid)
	}
	// The host stops the worker; the live session's next write is fenced.
	if _, e := f.svc.TransitionWork(context.Background(), f.host(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 2}, State: "stopped"}); e != nil {
		t.Fatal(e)
	}
	late, r := w.call(t, "graph_record_observation", map[string]any{"title": "late", "statement": "s", "evidence": "e", "epistemic_status": "observed"})
	wantCode(t, late, r, "stale_execution")
	if f.count("observation") != 1 {
		t.Fatal("stale worker write committed")
	}
	// Acceptance is an explicit launch-time grant; an invalid grant refuses to start.
	p := f.start(t, nil, f.args("-role", "planner", "-acceptance", "validator")...)
	pt, _ := p.sess.ListTools(context.Background(), &sdk.ListToolsParams{})
	names := map[string]bool{}
	for _, d := range pt.Tools {
		names[d.Name] = true
	}
	if len(names) != 7 || !names["graph_accept_goal"] {
		t.Fatalf("granted planner discovery %v", names)
	}
	cmd, stderr := f.command(t, f.args("-acceptance", "root")...)
	if e := cmd.Run(); e == nil || strings.Contains(stderr.String(), "postgres://") {
		t.Fatalf("unknown acceptance method started: %v %s", e, stderr)
	}
}

func TestStdioRefusesBadConfigurationWithoutLeakingSecrets(t *testing.T) {
	f := newFixture(t)
	for name, c := range map[string]struct {
		args []string
		env  string
	}{
		"unknown graph":     {[]string{"-namespace", f.ns, "-graph", graph.NewID(), "-actor", "a"}, ""},
		"unknown role":      {f.args("-role", "admin"), ""},
		"missing actor":     {[]string{"-namespace", f.ns, "-graph", f.id}, ""},
		"half execution":    {f.args("-role", "worker", "-work", graph.NewID()), ""},
		"guessed execution": {f.args("-role", "worker", "-work", graph.NewID(), "-execution", graph.NewID()), ""},
		"positional":        {append(f.args(), "extra"), ""},
		"unreachable db":    {f.args(), "postgres://svc:s3cretpw@127.0.0.1:1/nope?sslmode=disable&connect_timeout=2"},
		"no database":       {f.args(), "-"},
	} {
		cmd, stderr := f.command(t, c.args...)
		switch c.env {
		case "-":
			cmd.Env = append(cmd.Env, "EXPLORATION_DATABASE_URL=")
		case "":
		default:
			cmd.Env = append(cmd.Env, "EXPLORATION_DATABASE_URL="+c.env)
		}
		cmd.Stdin = strings.NewReader("")
		done := make(chan error, 1)
		go func() { done <- cmd.Run() }()
		select {
		case e := <-done:
			if e == nil {
				t.Errorf("%s: process started with invalid configuration", name)
			}
		case <-time.After(30 * time.Second):
			cmd.Process.Kill()
			t.Errorf("%s: process hung", name)
		}
		for _, secret := range []string{"s3cretpw", "postgres://", os.Getenv("GRAPH_TEST_DATABASE_URL")} {
			if secret != "" && strings.Contains(stderr.String(), secret) {
				t.Errorf("%s: stderr leaks %q: %s", name, secret, stderr)
			}
		}
	}
}

func TestStdioConnectionLossAndDisconnect(t *testing.T) {
	f := newFixture(t)
	s := f.start(t, nil, f.args()...)
	if out, _ := s.call(t, "graph_propose_work", map[string]any{"title": "Before crash", "instruction": "i", "priority": 1}); !out.OK {
		t.Fatal(out.Error)
	}
	// Abrupt server death: pending and later calls fail with a protocol error instead of hanging.
	if e := s.cmd.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, e := s.sess.CallTool(ctx, &sdk.CallToolParams{Name: "graph_overview", Arguments: map[string]any{}}); e == nil {
		t.Fatal("call on a dead server succeeded")
	}
	s.sess.Close()
	// A fresh process recovers the committed state: durable data does not depend on the connection.
	n := f.start(t, nil, f.args()...)
	lst, _ := n.call(t, "graph_list_nodes", map[string]any{"kind": "work_item"})
	if dataOf(t, lst)["total"].(float64) != 1 {
		t.Fatalf("state after server crash %v", lst)
	}
	// Client disconnect without a protocol goodbye: the server sees EOF and exits cleanly.
	c, _ := f.command(t, f.args()...)
	in, _ := c.StdinPipe()
	c.Stdout = &bytes.Buffer{}
	if e := c.Start(); e != nil {
		t.Fatal(e)
	}
	in.Close()
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case e := <-done:
		if e != nil {
			t.Fatalf("server did not exit cleanly on stdin EOF: %v", e)
		}
	case <-time.After(20 * time.Second):
		c.Process.Kill()
		t.Fatal("server ignored stdin EOF")
	}
}

// lossy drops the response to selected tools/call requests, as a flaky pipe or crashed
// client host would, after the server has already handled (and committed) the request.
type lossy struct {
	sdk.Transport
	tool string
	lost chan struct{}
}

func (l *lossy) Connect(ctx context.Context) (sdk.Connection, error) {
	c, e := l.Transport.Connect(ctx)
	if e != nil {
		return nil, e
	}
	return &lossyConn{Connection: c, tool: l.tool, lost: l.lost, ids: map[any]bool{}}, nil
}

type lossyConn struct {
	sdk.Connection
	tool string
	lost chan struct{}
	mu   sync.Mutex
	ids  map[any]bool
	once bool // only the first matching call loses its response
}

func (c *lossyConn) Write(ctx context.Context, m jsonrpc.Message) error {
	if r, ok := m.(*jsonrpc.Request); ok && r.Method == "tools/call" && r.ID.IsValid() && bytes.Contains(r.Params, []byte(`"`+c.tool+`"`)) {
		c.mu.Lock()
		if !c.once {
			c.once = true
			c.ids[r.ID.Raw()] = true
		}
		c.mu.Unlock()
	}
	return c.Connection.Write(ctx, m)
}
func (c *lossyConn) Read(ctx context.Context) (jsonrpc.Message, error) {
	for {
		m, e := c.Connection.Read(ctx)
		if e != nil {
			return nil, e
		}
		if r, ok := m.(*jsonrpc.Response); ok {
			c.mu.Lock()
			drop := c.ids[r.ID.Raw()]
			delete(c.ids, r.ID.Raw())
			c.mu.Unlock()
			if drop {
				select {
				case c.lost <- struct{}{}:
				default:
				}
				continue
			}
		}
		return m, nil
	}
}

// A client without a durable invocation identity cannot know whether a write whose response
// was lost committed. The server must not hide this: the safe client path is read-then-decide.
func TestStdioLostResponseIsAmbiguousAndRequiresReconciliation(t *testing.T) {
	f := newFixture(t)
	lost := make(chan struct{}, 1)
	s := f.start(t, func(tr sdk.Transport) sdk.Transport {
		return &lossy{Transport: tr, tool: "graph_propose_work", lost: lost}
	}, f.args()...)
	args := map[string]any{"title": "Reproduce parser failure", "instruction": "Run twelve through the parser.", "priority": 5}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, e := s.sess.CallTool(ctx, &sdk.CallToolParams{Name: "graph_propose_work", Arguments: args})
	cancel()
	if e == nil {
		t.Fatal("client received a response that the transport dropped")
	}
	select {
	case <-lost:
	default:
		t.Fatal("the response was never produced, so this did not exercise a committed-but-unacknowledged write")
	}
	// The write committed although the client saw an error: outcome unknown to the caller.
	if f.count("work_item") != 1 {
		t.Fatalf("expected the dropped-response write to be committed, got %d work items", f.count("work_item"))
	}
	// Reconcile first: a read on the same session shows the committed item, so the client must not retry.
	lst, _ := s.call(t, "graph_list_nodes", map[string]any{"kind": "work_item"})
	nodes := dataOf(t, lst)["nodes"].([]any)
	if len(nodes) != 1 || nodes[0].(map[string]any)["title"] != "Reproduce parser failure" {
		t.Fatalf("reconciliation read did not reveal the committed write: %v", nodes)
	}
	// Blind retry is a distinct operation (no durable identity): it duplicates. This documents
	// why the server instructs clients to stop and reconcile instead of retrying.
	blind, _ := s.call(t, "graph_propose_work", args)
	if !blind.OK || f.count("work_item") != 2 {
		t.Fatalf("expected the blind retry to create a second item, got %d", f.count("work_item"))
	}
}
