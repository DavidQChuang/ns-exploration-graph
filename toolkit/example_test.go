package toolkit_test

import (
	"context"
	"encoding/json"
	"fmt"

	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/store/memory"
	"ns-exploration-graph/toolkit"
)

// A minimal host: create a graph through the host API, bind a planner session to it,
// and invoke a tool with a host-persisted invocation identity. The same identity
// replays the original envelope instead of writing twice.
func Example() {
	ctx := context.Background()
	svc := graph.NewService(memory.New()) // production hosts pass a *postgres.Store
	binding := toolkit.Binding{Namespace: "demo", GraphID: graph.NewID(), ActorID: "host:planner-1", Role: "planner"}
	_, err := svc.CreateGraph(ctx, graph.CommandContext{Namespace: binding.Namespace, GraphID: binding.GraphID, ActorID: binding.ActorID, RequestID: graph.NewID()},
		graph.CreateGraphInput{Title: "Parser investigation", Request: "Verify the parser rejects malformed input."})
	if err != nil {
		panic(err)
	}
	tools, err := toolkit.New(svc, toolkit.Config{Binding: binding})
	if err != nil {
		panic(err)
	}
	args := json.RawMessage(`{"title":"Reproduce","instruction":"Run the parser on twelve.","priority":5}`)
	first := tools.Invoke(ctx, "graph_propose_work", args, "host-ledger-entry-1")
	retry := tools.Invoke(ctx, "graph_propose_work", args, "host-ledger-entry-1")
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(retry)
	fmt.Println(first.OK, first.GraphRevision, string(a) == string(b))
	denied := tools.Invoke(ctx, "graph_accept_goal", json.RawMessage(`{}`), "host-ledger-entry-2")
	fmt.Println(denied.OK, denied.Error.Code)
	// Output:
	// true 2 true
	// false permission_denied
}
