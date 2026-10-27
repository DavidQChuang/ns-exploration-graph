package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/store/postgres"
)

// buildReport derives the normalized Report from persisted graph state only.
func buildReport(ctx context.Context, svc *graph.Service, ns, id string) (Report, error) {
	snapshot, e := svc.Snapshot(ctx, ns, id)
	if e != nil {
		return Report{}, e
	}
	report := Report{GraphRevision: snapshot.Graph.Revision, Nodes: []string{}, Lifecycle: []string{}, Relations: []string{}, Events: []string{}, Acceptances: []string{}}
	titles := map[string]string{}
	for _, n := range snapshot.Nodes {
		titles[n.ID] = n.Current().Title
		report.Nodes = append(report.Nodes, n.Kind+":"+n.Current().Title+":"+n.Current().State)
		states := []string{}
		for _, r := range n.Revisions {
			states = append(states, r.State)
		}
		report.Lifecycle = append(report.Lifecycle, n.Kind+":"+n.Current().Title+":"+strings.Join(states, ">"))
		if n.Kind == "goal" {
			report.GoalState = n.Current().State
		}
	}
	for _, ed := range snapshot.Edges {
		report.Relations = append(report.Relations, ed.Relation+":"+titles[ed.SourceID]+"→"+titles[ed.TargetID])
	}
	evidence := map[string]bool{}
	for _, a := range snapshot.Acceptances {
		report.Acceptances = append(report.Acceptances, fmt.Sprintf("%s:%s:%s:goal=%s@%d:edges=%d", a.Method, a.State, a.Rationale, titles[a.GoalID], a.EvaluatedRevision, len(a.SupportEdgeIDs)))
		report.ReviewedResources += len(a.ReviewedResourceIDs)
		for _, eid := range a.SupportEdgeIDs {
			for _, ed := range snapshot.Edges {
				if ed.ID == eid {
					evidence[ed.SourceID] = true
				}
			}
		}
	}
	report.ReviewedEvidence = len(evidence)
	events, e := svc.EventsAfter(ctx, ns, id, 0, 1000)
	if e != nil {
		return Report{}, e
	}
	for _, ev := range events.Events {
		report.Events = append(report.Events, ev.CommandKind)
	}
	sort.Strings(report.Nodes)
	sort.Strings(report.Lifecycle)
	sort.Strings(report.Relations)
	sort.Strings(report.Acceptances)
	return report, nil
}

// hostMain is the host-side control CLI used when the agent side is not Go (for example
// the Python consumer in examples/python). It owns graph creation, claims and lifecycle:
// exactly the operations the toolkit does not expose to models. It talks to PostgreSQL
// through EXPLORATION_DATABASE_URL and prints one JSON document per command.
//
//	host init   -namespace NS          migrate, create graph + goal
//	host claim  -namespace NS -graph G -work W -revision R    open -> running with a new execution
//	host finish -namespace NS -graph G -work W -revision R -state done|exhausted|stopped
//	host report -namespace NS -graph G                         normalized Report from persisted state
func hostMain(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: host init|claim|finish|report")
	}
	fs := flag.NewFlagSet("host "+args[0], flag.ContinueOnError)
	ns := fs.String("namespace", "", "graph namespace")
	id := fs.String("graph", "", "graph UUID")
	work := fs.String("work", "", "work item UUID")
	revision := fs.Int("revision", 0, "expected current work revision")
	state := fs.String("state", "done", "target state for finish")
	if e := fs.Parse(args[1:]); e != nil {
		return e
	}
	dsn := os.Getenv("EXPLORATION_DATABASE_URL")
	if dsn == "" || *ns == "" {
		return fmt.Errorf("EXPLORATION_DATABASE_URL and -namespace are required")
	}
	store, e := postgres.Open(ctx, dsn)
	if e != nil {
		return fmt.Errorf("cannot open graph database")
	}
	defer store.Close()
	if e = store.Migrate(ctx); e != nil {
		return fmt.Errorf("cannot migrate graph database")
	}
	svc := graph.NewService(store)
	command := func() graph.CommandContext {
		return graph.CommandContext{Namespace: *ns, GraphID: *id, ActorID: "scripted-host", RequestID: graph.NewID()}
	}
	var result any
	switch args[0] {
	case "init":
		*id = graph.NewID()
		if _, e = svc.CreateGraph(ctx, command(), graph.CreateGraphInput{Title: "Parser investigation", Request: "Reproduce malformed parser input, then verify the corrected input fixture."}); e != nil {
			return e
		}
		g, e := svc.AddGoal(ctx, command(), graph.AddGoalInput{Title: "Corrected parser input verified", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"Retain failure evidence and a passing verification of corrected input 12."}}})
		if e != nil {
			return e
		}
		result = map[string]any{"namespace": *ns, "graph_id": *id, "goal_id": g.Data["goal_id"]}
	case "claim":
		x := graph.NewID()
		if _, e = svc.TransitionWork(ctx, command(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: *work, Revision: *revision}, State: "running", ExecutionID: x}); e != nil {
			return e
		}
		result = map[string]any{"work_item_id": *work, "execution_id": x}
	case "finish":
		if _, e = svc.TransitionWork(ctx, command(), graph.TransitionWorkInput{Node: graph.Ref{NodeID: *work, Revision: *revision}, State: *state}); e != nil {
			return e
		}
		result = map[string]any{"work_item_id": *work, "state": *state}
	case "report":
		r, e := buildReport(ctx, svc, *ns, *id)
		if e != nil {
			return e
		}
		r.ResumedFromGraph = true
		result = r
	default:
		return fmt.Errorf("unknown host command %q", args[0])
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
