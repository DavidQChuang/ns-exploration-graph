// A deterministic no-model harness. All scheduling, parsing, evidence resolution,
// invocation bookkeeping and acceptance policy remain in the host.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	transport "ns-exploration-graph/adapters/mcp"
	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/store/memory"
	"ns-exploration-graph/toolkit"
)

type session struct {
	Call  func(string, any, string) (toolkit.Envelope, error)
	Close func()
}

func newSession(ctx context.Context, mode string, s *graph.Service, b toolkit.Binding, policy toolkit.AcceptancePolicy) (session, error) {
	a, e := toolkit.New(s, toolkit.Config{Binding: b, AcceptancePolicy: policy})
	if e != nil {
		return session{}, e
	}
	if mode == "direct" {
		return session{Call: func(n string, args any, id string) (toolkit.Envelope, error) {
			raw, _ := json.Marshal(args)
			return a.Invoke(ctx, n, raw, id), nil
		}, Close: func() {}}, nil
	}
	if mode != "mcp" {
		return session{}, fmt.Errorf("transport must be direct or mcp")
	}
	// This sequential harness keeps an invocation ledger in memory. A production
	// host must persist it before sending a write if it needs process-crash retry.
	activeInvocation := ""
	server, e := transport.NewServer(ctx, a, transport.Options{ResolveInvocation: func(context.Context, *sdk.CallToolRequest) (string, error) { return activeInvocation, nil }})
	if e != nil {
		return session{}, e
	}
	ct, st := sdk.NewInMemoryTransports()
	ss, e := server.Connect(ctx, st, nil)
	if e != nil {
		return session{}, e
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "scripted-host", Version: "1"}, nil)
	cs, e := client.Connect(ctx, ct, nil)
	if e != nil {
		ss.Close()
		return session{}, e
	}
	return session{Call: func(n string, args any, id string) (toolkit.Envelope, error) {
		activeInvocation = id
		r, e := cs.CallTool(ctx, &sdk.CallToolParams{Name: n, Arguments: args})
		if e != nil {
			return toolkit.Envelope{}, e
		}
		raw, _ := json.Marshal(r.StructuredContent)
		var out toolkit.Envelope
		e = json.Unmarshal(raw, &out)
		return out, e
	}, Close: func() { cs.Close(); ss.Close() }}, nil
}
func invoke(s session, name string, args any, id string) (map[string]any, error) {
	out, e := s.Call(name, args, id)
	if e != nil {
		return nil, e
	}
	if !out.OK {
		return nil, out.Error
	}
	b, _ := json.Marshal(out.Data)
	var data map[string]any
	e = json.Unmarshal(b, &data)
	return data, e
}

// Report is the normalized comparison form of a finished scenario: generated IDs and
// timestamps are replaced by titles, states and counts, so independent runs (direct,
// in-process MCP, stdio subprocess, Python consumer) can be compared for equality.
type Report struct {
	GraphRevision     int64    `json:"graph_revision"`
	GoalState         string   `json:"goal_state"`
	Nodes             []string `json:"nodes"`
	Lifecycle         []string `json:"lifecycle"`
	Relations         []string `json:"relations"`
	Events            []string `json:"events"`
	Acceptances       []string `json:"acceptances"`
	ReviewedEvidence  int      `json:"reviewed_evidence"`
	ReviewedResources int      `json:"reviewed_resources"`
	ResumedFromGraph  bool     `json:"resumed_from_graph"`
}

func run(ctx context.Context, mode string) (Report, error) {
	store := memory.New()
	svc := graph.NewService(store)
	b := toolkit.Binding{Namespace: "scripted", GraphID: graph.NewID(), ActorID: "scripted-host", Role: "planner"}
	command := func() graph.CommandContext {
		return graph.CommandContext{Namespace: b.Namespace, GraphID: b.GraphID, ActorID: b.ActorID, RequestID: graph.NewID()}
	}
	_, e := svc.CreateGraph(ctx, command(), graph.CreateGraphInput{Title: "Parser investigation", Request: "Verify the integer parser rejects malformed input."})
	if e != nil {
		return Report{}, e
	}
	_, e = svc.AddGoal(ctx, command(), graph.AddGoalInput{Title: "Malformed input rejected", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Run malformed input and retain rejection evidence."}}})
	if e != nil {
		return Report{}, e
	}
	planner, e := newSession(ctx, mode, svc, b, nil)
	if e != nil {
		return Report{}, e
	}
	work, e := invoke(planner, "graph_propose_work", map[string]any{"title": "Reproduce malformed input", "instruction": "Run integer parser with twelve; record the negative result.", "priority": 50}, "propose-1")
	planner.Close()
	if e != nil {
		return Report{}, e
	}
	wid := work["work_item_id"].(string)
	execution := graph.NewID()
	_, e = svc.TransitionWork(ctx, command(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 1}, State: "running", ExecutionID: execution})
	if e != nil {
		return Report{}, e
	}
	wb := b
	wb.Role = "worker"
	wb.Execution = &graph.Execution{WorkItemID: wid, ExecutionID: execution}
	worker, e := newSession(ctx, mode, svc, wb, nil)
	if e != nil {
		return Report{}, e
	}
	_, parseErr := strconv.Atoi("twelve")
	if parseErr == nil {
		worker.Close()
		return Report{}, fmt.Errorf("parser unexpectedly accepted malformed input")
	}
	log := "fixture integer parser: malformed input rejected"
	resourceURI := "fixture://parser/rejection-log"
	resolver := map[string]string{resourceURI: log}
	observationArgs := map[string]any{"title": "Malformed input rejection observed", "statement": "The parser rejected twelve.", "evidence": log, "epistemic_status": "observed", "polarity": "negative", "resource_refs": []any{map[string]any{"resource_type": "test_log", "uri": resourceURI}}}
	ob, e := invoke(worker, "graph_record_observation", observationArgs, "observation-1")
	if e != nil {
		worker.Close()
		return Report{}, e
	}
	retry, e := invoke(worker, "graph_record_observation", observationArgs, "observation-1")
	if e != nil || retry["observation_id"] != ob["observation_id"] {
		worker.Close()
		return Report{}, fmt.Errorf("lost-response retry did not preserve observation")
	}
	_, e = invoke(worker, "graph_record_conclusion", map[string]any{"title": "Parser rejection verified", "statement": "Malformed text is rejected by the integer parser.", "epistemic_status": "inferred", "supports": []any{map[string]any{"id": ob["observation_id"], "revision": ob["node_revision"]}}}, "conclusion-1")
	worker.Close()
	if e != nil {
		return Report{}, e
	}
	_, e = svc.TransitionWork(ctx, command(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: wid, Revision: 2}, State: "done"})
	if e != nil {
		return Report{}, e
	}
	// Discard session state and all tool-returned IDs. The fresh service/session
	// discovers the goal and evidence exclusively through graph reads.
	svc = graph.NewService(store)
	return resume(ctx, mode, svc, b, resolver)
}
func resume(ctx context.Context, mode string, svc *graph.Service, b toolkit.Binding, resolver map[string]string) (Report, error) {
	b.AcceptGoal = true
	b.AcceptanceMethod = "validator"
	policy := func(ctx context.Context, b toolkit.Binding, in graph.AcceptGoalInput) error {
		if in.Rationale != "Host reviewed the stored parser rejection log." {
			return fmt.Errorf("rationale not approved")
		}
		return nil
	}
	planner, e := newSession(ctx, mode, svc, b, policy)
	if e != nil {
		return Report{}, e
	}
	defer planner.Close()
	_, e = invoke(planner, "graph_overview", map[string]any{}, "")
	if e != nil {
		return Report{}, e
	}
	all, e := invoke(planner, "graph_list_nodes", map[string]any{"limit": 100}, "")
	if e != nil {
		return Report{}, e
	}
	var goal, conclusion map[string]any
	for _, v := range all["nodes"].([]any) {
		n := v.(map[string]any)
		if n["kind"] == "goal" {
			goal = n
		}
		if n["kind"] == "conclusion" {
			conclusion = n
		}
	}
	if goal == nil || conclusion == nil {
		return Report{}, fmt.Errorf("resume anchors missing")
	}
	con, e := invoke(planner, "graph_get_node", map[string]any{"node_id": conclusion["id"], "revision": conclusion["revision"]}, "")
	if e != nil {
		return Report{}, e
	}
	verified := false
	for _, v := range con["edges"].([]any) {
		ed := v.(map[string]any)
		if ed["relation"] != "supports" || ed["target_id"] != conclusion["id"] {
			continue
		}
		ob, e := invoke(planner, "graph_get_node", map[string]any{"node_id": ed["source_id"], "revision": ed["source_revision"]}, "")
		if e != nil {
			return Report{}, e
		}
		for _, r := range ob["resource_refs"].([]any) {
			uri := r.(map[string]any)["uri"].(string)
			if resolver[uri] == "fixture integer parser: malformed input rejected" {
				verified = true
			}
		}
	}
	if !verified {
		return Report{}, fmt.Errorf("host could not resolve reviewed evidence")
	}
	support, e := invoke(planner, "graph_add_support", map[string]any{"source": map[string]any{"id": conclusion["id"], "revision": conclusion["revision"]}, "target": map[string]any{"id": goal["id"], "revision": goal["revision"]}}, "support-resume-1")
	if e != nil {
		return Report{}, e
	}
	accepted, e := invoke(planner, "graph_accept_goal", map[string]any{"goal": map[string]any{"id": goal["id"], "revision": goal["revision"]}, "support_edge_ids": []any{support["edge_id"]}, "rationale": "Host reviewed the stored parser rejection log."}, "accept-resume-1")
	if e != nil {
		return Report{}, e
	}
	report, e := buildReport(ctx, svc, b.Namespace, b.GraphID)
	if e != nil {
		return Report{}, e
	}
	if report.ReviewedEvidence != len(accepted["reviewed_evidence"].([]any)) || report.ReviewedResources != len(accepted["reviewed_resource_ids"].([]any)) {
		return Report{}, fmt.Errorf("acceptance response disagrees with persisted closure")
	}
	report.ResumedFromGraph = true
	return report, nil
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "host" {
		if e := hostMain(context.Background(), os.Args[2:], os.Stdout); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		return
	}
	mode := flag.String("transport", "direct", "direct or mcp")
	flag.Parse()
	out, e := run(context.Background(), *mode)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}
