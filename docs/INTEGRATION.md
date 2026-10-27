# Minimal host integration

This describes what a host (the program that runs agents) must do to use the exploration graph, and what the library does for it. It states only behavior covered by tests in this repository; the normative contract is `spec/SPEC.md` and `spec/TOOLS.md`.

## Responsibility split

| The host does | The library does |
|---|---|
| Create the graph, goals, hints; migrate PostgreSQL | Validate every command; enforce graph rules in one shared service |
| Authenticate the caller and choose namespace, graph, actor, role, grants | Refuse every call outside the bound role, on every invocation |
| Claim work (`running` with a new execution ID), complete or stop it | Fence stale executions (`stale_execution`) |
| Persist an invocation identity before each write; reconcile ambiguous outcomes | Return the exact original envelope for the same identity and arguments |
| Resolve and authorize evidence URIs; review before acceptance | Record references, support edges and the reviewed closure |
| Run models, tools, scheduling, completion and cancellation policy | Nothing of these |

Prompts are guidance, not enforcement. The shipped prompt assets (`toolkit/prompts`, version 1) describe correct use, but they grant and restrict nothing. Authorization is checked by the adapter and graph rules by the service, whatever the model was told or reads. Graph text is returned as untrusted task data and cannot change a session's tools (`toolkit/prompts_test.go` exercises this with injected instructions).

## 1. Create and migrate

```go
store, _ := postgres.Open(ctx, dsn)   // or memory.New() for tests
_ = store.Migrate(ctx)                // idempotent; takes an advisory lock
svc := graph.NewService(store)
svc.CreateGraph(ctx, graph.CommandContext{Namespace: ns, GraphID: id, ActorID: host, RequestID: requestID}, graph.CreateGraphInput{Title: "...", Request: "..."})
svc.AddGoal(ctx, cmd(), graph.AddGoalInput{Title: "...", GoalPayload: graph.GoalPayload{AcceptanceCriteria: []string{"..."}}})
```

Host commands carry their own `RequestID`; reuse it when retrying a host write.

## 2. Bind a session

```go
tools, err := toolkit.New(svc, toolkit.Config{
	Binding: toolkit.Binding{Namespace: ns, GraphID: id, ActorID: actor, Role: "planner"},
	Authorize: func(ctx context.Context, b toolkit.Binding, tool string) error { /* live check; optional */ },
})
```

`Binding` is trusted configuration, never tool arguments. `toolkit.New` refuses an unknown role, a missing actor/namespace, a bad graph or execution UUID, limits that disagree with the service, and an acceptance grant without a method (`human`, `validator`, `agent`) and an `AcceptancePolicy`. `Authorize` runs before every invocation, including retries. `tools.Definitions()` lists only granted tools; that is convenience, not the boundary.

Present `toolkit.Prompt(role)` plus an initial `graph_overview` to the model. The MCP adapter does both in the server instructions.

## 3. Invocation identity and retries

Call `tools.Invoke(ctx, name, argsJSON, invocation)`. For mutating tools `invocation` is required (at most 200 characters), must be persisted by the host before sending, and must be reused to retry. The service request key is a hash of graph, actor and invocation.

- Same identity and arguments: the first success envelope, byte for byte, including its original `graph_revision`, even after newer writes or after the graph closed.
- Same identity, different arguments: `idempotency_conflict`.
- New identity: a new operation, even for identical arguments.
- Authorization and role are rechecked on a retry; a revoked retry returns `permission_denied`.
- A failed write leaves the graph unchanged. Only `storage_unavailable` is `retryable: true`.

Check `ok`, not transport success. Error codes: `invalid_argument`, `not_found`, `permission_denied`, `revision_conflict`, `invalid_transition`, `invalid_relation`, `cycle_detected`, `stale_evidence`, `stale_execution`, `idempotency_conflict`, `already_exists`, `graph_read_only`, `budget_too_small`, `stale_cursor`, `limit_exceeded`, `storage_unavailable`. Messages are safe; storage failures are replaced by a fixed message so driver or SQL text never reaches a model.

## 4. Execution binding and worker lifecycle

1. The host reads work, then moves it to `running` with its expected node revision and a new execution UUID: `svc.TransitionWork(... State: "running", ExecutionID: x)`.
2. Start the worker with `Binding{Role: "worker", Execution: &graph.Execution{WorkItemID: w, ExecutionID: x}}`. Observations attach to that work item; a worker without a binding cannot record observations.
3. The worker reads, records observations and conclusions, and ends its turn.
4. After pending writes settle, the host completes the work (`done`). A successful model turn without evidence is not success: a work item with no outgoing `yields` edge can be marked `exhausted` instead, and a worker that found nothing records an explicit negative observation (empty evidence is rejected).
5. To stop or reassign: `stopped` -> `open` -> `running` with a new execution ID. The old worker's next write returns `stale_execution`; it must stop and return control. Replaying one of its already committed invocations still returns that original success.

## 5. Acceptance

`graph_accept_goal` is available only when the binding sets `AcceptGoal`, an acceptance method and a policy. The method comes from the binding. The policy callback sees the request and can refuse with `permission_denied` before the service runs. The service still requires fresh, explicitly selected direct support edges and records the reviewed closure. Acceptance does not close the graph or finish the task; the host decides that.

## 6. Evidence resolver

References are data: type, URI, optional external revision and SHA-256. The host resolves them, enforces who may read them, and checks any digest. Tool results never contain artifact bodies. Hosts that accept goals should resolve and review the cited evidence before granting or approving acceptance (`examples/scripted` and the Python consumer do this with a local resolver).

## 7. MCP

In process: `adapters/mcp.NewServer(ctx, tools, mcp.Options{ResolveInvocation: fn})` returns an SDK server for any transport. `fn` must return the identity the host persisted for that call; without it every call gets a fresh random identity (no retry safety). Do not derive identity from the JSON-RPC request ID or from model-visible `_meta`.

Stdio: `graph-mcp` (see `README.md`) binds one existing graph, actor and role from flags and `EXPLORATION_DATABASE_URL`. It exits 0 when the client closes stdin. Tool failures are returned as MCP tool errors (`isError: true`) with the same structured envelope in `structuredContent` and as JSON text.

A client without durable invocation identity must treat a lost response or disconnect during a write as an ambiguous outcome: stop, read the graph, and write again only if the effect is absent. `examples/python/parser_scenario.py` implements this; `cmd/graph-mcp/stdio_test.go` shows that a blind retry duplicates the write.

## 8. Limits and compatibility

Limits and versions are listed in `README.md`. Unknown tool fields and over-limit input are rejected rather than repaired. Normative contract version 0.2 is approved by `docs/DECISIONS.md` 012. Changes to its schemas, error codes or prompt assets require a recorded contract decision; intentional breaking changes require a new minor contract/release version, migration or compatibility notes, and fixtures. Candidate commit `62c083f` passed the Go 1.25.0 and 1.27.3 runtime matrix; source-archive inspection and root delivery sign-off remain pending. `python toolkit/generate_contracts.py --check` and the tests in `toolkit/` fail on generated contract drift.

## Not provided

Model calls, a scheduler, worker launching, authentication, remote or multi-tenant MCP serving, per-node ACLs, automatic evidence retrieval or authorization, external-effect fencing, transparent exactly-once behavior for clients without durable invocation identity, ARTEX migration, and the optional web viewer.
