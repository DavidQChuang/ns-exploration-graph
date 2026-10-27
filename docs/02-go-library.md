# Go library guide

[Documentation](README.md) · [Agents](01-agents.md) · [Go library](02-go-library.md) · [MCP](03-mcp.md) · [Workflow](04-workflow.md) · [Architecture](05-architecture.md) · [Storage](06-storage.md) · [Operations](07-operations.md) · [Roadmap](08-roadmap.md)

Embed the service to create graphs, manage work lifecycle and expose a narrow tool adapter to agents. The module requires Go 1.25.0 or newer. Its module path is `ns-exploration-graph`; the local source release is not a published package-registry installation.

## Run a complete memory quickstart

For a local checkout, save the program below as `.local/docs-quickstart/main.go` in the repository root and run:

```sh
go run ./.local/docs-quickstart
```

For a separate application, create your own Go module and point its dependency at the local source checkout. Replace the example path with your absolute checkout path (use forward slashes on Windows):

```sh
go mod init example.com/graph-demo
go mod edit "-require=ns-exploration-graph@v0.0.0"
go mod edit "-replace=ns-exploration-graph=/absolute/path/ns-exploration-graph"
# Save the program as main.go, then:
go mod tidy
go run .
```

The following program creates a graph, binds planner tools, proposes work and verifies replay of one invocation. Memory storage lasts only for this process. The fixed invocation string illustrates replay inside this run; a production host must persist each invocation identity and original arguments before sending a mutation.

```go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "log"

    "ns-exploration-graph/graph"
    "ns-exploration-graph/graph/store/memory"
    "ns-exploration-graph/toolkit"
)

func main() {
    if err := run(); err != nil {
        log.Fatal(err)
    }
}

func run() error {
    ctx := context.Background()
    svc := graph.NewService(memory.New())
    binding := toolkit.Binding{
        Namespace: "demo", GraphID: graph.NewID(),
        ActorID: "host:planner-1", Role: "planner",
    }
    command := graph.CommandContext{
        Namespace: binding.Namespace, GraphID: binding.GraphID,
        ActorID: binding.ActorID, RequestID: graph.NewID(),
    }
    if _, err := svc.CreateGraph(ctx, command, graph.CreateGraphInput{
        Title: "Parser investigation",
        Request: "Verify the parser rejects malformed input.",
    }); err != nil {
        return fmt.Errorf("create graph: %w", err)
    }
    tools, err := toolkit.New(svc, toolkit.Config{Binding: binding})
    if err != nil {
        return fmt.Errorf("bind tools: %w", err)
    }
    args := json.RawMessage(`{"title":"Reproduce","instruction":"Run the parser on twelve.","priority":5}`)
    invocation := "host-ledger-entry-1"
    first := tools.Invoke(ctx, "graph_propose_work", args, invocation)
    if !first.OK {
        return first.Error
    }
    retry := tools.Invoke(ctx, "graph_propose_work", args, invocation)
    if !retry.OK {
        return retry.Error
    }
    a, err := json.Marshal(first)
    if err != nil {
        return err
    }
    b, err := json.Marshal(retry)
    if err != nil {
        return err
    }
    if !bytes.Equal(a, b) {
        return fmt.Errorf("replay differed from first success")
    }
    fmt.Printf("ok=%t revision=%d replay=%t\n",
        first.OK, first.GraphRevision, bytes.Equal(a, b))
    fmt.Println(string(a)) // includes the returned work_item_id and node_revision
    return nil
}
```

The first output line is `ok=true revision=2 replay=true`. The second contains generated IDs, so its exact text varies. [toolkit/example_test.go](../toolkit/example_test.go) is the compiled repository example of this flow, including a rejected acceptance call.

## Switch to PostgreSQL

Provision a reachable PostgreSQL database and give the host its connection string through `EXPLORATION_DATABASE_URL`. In the quickstart, replace the memory import with these imports:

```go
"os"
"ns-exploration-graph/graph/store/postgres"
```

Replace `svc := graph.NewService(memory.New())` with this block inside `run`:

```go
dsn := os.Getenv("EXPLORATION_DATABASE_URL")
if dsn == "" {
    return fmt.Errorf("EXPLORATION_DATABASE_URL is required")
}
store, err := postgres.Open(ctx, dsn)
if err != nil {
    return fmt.Errorf("cannot open graph database")
}
defer store.Close()
if err := store.Migrate(ctx); err != nil {
    return fmt.Errorf("cannot migrate graph database")
}
svc := graph.NewService(store)
```

`Open` connects and checks reachability. `Migrate` applies the embedded initial schema if absent and takes a transaction-scoped advisory lock. The graph still must be created through `CreateGraph`; opening the store does not create it. Each run above generates a new graph ID. A resumable host persists its graph binding and reopens that existing graph rather than recreating it.

This replacement uses the current API; running it requires your database. The repository's prior PostgreSQL verification is recorded in [POSTGRES-VERIFICATION.md](POSTGRES-VERIFICATION.md) and [CLEAN-ENVIRONMENT.md](CLEAN-ENVIRONMENT.md). This guide does not provision PostgreSQL or rerun that release matrix.

## Keep lifecycle operations in the host

Use `graph.CommandContext` for host writes: namespace, graph ID, actor and a stable `RequestID`. Persist the request ID before a write and reuse it on retry. Read node IDs and expected revisions from service results; host references are `graph.Ref{NodeID: returnedID, Revision: returnedRevision}`.

`CreateGraph`, `AddGoal`, `TransitionWork`, revisions/retractions, snapshots and graph administration are host APIs. The toolkit exposes only the eight agent operations. A typical claim uses `TransitionWorkInput{Node: ref, State: "running", ExecutionID: graph.NewID()}`; the host then binds a worker with:

```go
// assignedWorkID and executionID come from the host's successful claim.
binding := toolkit.Binding{
    Namespace: ns, GraphID: graphID, ActorID: workerActor, Role: "worker",
    Execution: &graph.Execution{
        WorkItemID: assignedWorkID, ExecutionID: executionID,
    },
}
```

This is a binding fragment, not a standalone program. The worker cannot choose the binding in tool arguments. Wait for pending writes to settle before completing work; stopped, completed or reassigned executions reject new worker writes with `stale_execution`. An already committed invocation can still replay its original success after authorization is checked.

## Choose policy and transport at the boundary

`toolkit.Config.Authorize` optionally checks live authorization before every invocation, including retries. To grant `graph_accept_goal`, set `Binding.AcceptGoal`, set `Binding.AcceptanceMethod`, and provide `Config.AcceptancePolicy`. The policy receives `graph.AcceptGoalInput` and may refuse before the service evaluates the evidence. Prompts and discovery are guidance; they do not replace these checks.

For MCP, `adapters/mcp.NewServer(ctx, tools, mcp.Options{ResolveInvocation: resolver})` returns an official SDK server. The resolver must obtain the trusted identity your host persisted for that mutation. With no resolver, each mutation gets a new random identity, so reconnect replay is unsafe. See [MCP](03-mcp.md) for the stdio binary and result handling.

## Use the compiled scenarios as integration references

Run `go test ./toolkit -run Example -v` for the memory example. Run `go run ./examples/scripted -transport direct` or `-transport mcp` for the deterministic scenario, including negative evidence, verification and acceptance. The MCP option uses an in-process transport with memory storage.

[examples/scripted/main.go](../examples/scripted/main.go) shows direct and MCP calls with returned IDs. [examples/scripted/host.go](../examples/scripted/host.go) shows PostgreSQL setup, graph creation and work claims. The [Python consumer](../examples/python/README.md) exercises the separate stdio process and reconciles a deliberately lost response. These are tested repository paths; they do not test a live model or a named third-party harness.

Continue with [MCP setup and tools](03-mcp.md), [host orchestration](04-workflow.md), or [storage](06-storage.md).
