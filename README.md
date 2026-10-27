# ns-exploration-graph

A durable "exploration graph" library for agent harnesses, written in Go. It stores an investigation as typed, revisioned nodes (origin, goals, work items, observations, conclusions, hints) with typed edges, resource references and goal acceptances. A shared command service enforces the rules; two storage adapters persist it (in-memory reference store and PostgreSQL). Eight narrow, schema-validated tools expose it to models, in-process or over MCP stdio.

Status: pre-release. Contract version 0.2 (`"status": "proposed"` in `spec/tool-contracts.json`). Release gates and their evidence are tracked in `docs/RELEASE-CHECKLIST.md` and `docs/REQUIREMENTS.md`; nothing here is a claim that those gates are closed.

## What it is not

It does not run models, schedule or launch workers, fetch or authorize evidence, authenticate callers, serve remote clients, or provide a viewer. The embedded library is a trusted-host boundary. A generic MCP client without durable invocation identity gets no reconnect-retry guarantee (see below). No ARTEX/Norma code or service is used.

## Layout

| Path | Purpose |
|---|---|
| `graph/` | Semantic command service, reads/projection, snapshots; `graph/store/memory`, `graph/store/postgres` (+ migration) |
| `toolkit/` | The eight tools: embedded contracts (`contracts.json`), strict validation, role/capability binding, prompt assets |
| `adapters/mcp/` | MCP server over the official Go SDK |
| `cmd/graph-mcp/` | Stdio MCP server bound to one graph, actor and role (PostgreSQL) |
| `examples/scripted/` | Deterministic no-model scenario (direct, in-process MCP) and a host control CLI |
| `examples/python/` | Second consumer: Python standard-library MCP client for the same scenario |
| `spec/` | SPEC, TOOLS, development plan, JSON contracts and SQL |

## Install and build

Requirements: Go (module minimum `go 1.25.0` in `go.mod`) and, for the PostgreSQL store and `graph-mcp`, a reachable PostgreSQL server.

```sh
go build ./...
go build -o bin/graph-mcp ./cmd/graph-mcp
```

The module path is `ns-exploration-graph` (local; not published to a registry).

## Quick start (in process)

`toolkit/example_test.go` is a compiled, tested version of this flow: create a graph through the host API, bind an adapter to it, and call a tool with a host-persisted invocation identity.

```go
svc := graph.NewService(store) // memory.New(), or a *postgres.Store after Migrate
svc.CreateGraph(ctx, graph.CommandContext{Namespace: ns, GraphID: id, ActorID: actor, RequestID: requestID},
	graph.CreateGraphInput{Title: "Parser investigation", Request: "Verify the parser rejects malformed input."})
tools, err := toolkit.New(svc, toolkit.Config{Binding: toolkit.Binding{Namespace: ns, GraphID: id, ActorID: actor, Role: "planner"}})
out := tools.Invoke(ctx, "graph_propose_work", argsJSON, hostInvocationID) // toolkit.Envelope
```

Run the end-to-end scripted scenario (no model involved):

```sh
go run ./examples/scripted -transport direct   # or: -transport mcp (in-process MCP client/server)
```

`docs/INTEGRATION.md` covers the minimal host integration in full.

## Stdio MCP server

`graph-mcp` serves one pre-existing graph for one actor and role over stdio. It does not create graphs and does not migrate the database: apply `graph/store/postgres/migrations/001_initial.sql` (or call `postgres.Store.Migrate`) and create the graph through the host API first.

```sh
export EXPLORATION_DATABASE_URL='postgres://user:pass@host:5432/db?sslmode=...'
./bin/graph-mcp -namespace NS -graph GRAPH_UUID -actor ACTOR -role planner
```

| Flag | Meaning |
|---|---|
| `-namespace`, `-graph`, `-actor` | Required trusted binding (graph must exist) |
| `-role` | `planner` (default), `worker`, or `solo` |
| `-work`, `-execution` | Both together: host-assigned running work and its execution UUID (required for worker writes) |
| `-acceptance` | `human`, `validator` or `agent`: grants `graph_accept_goal`. Off by default. The process-level flag is the only policy this binary has; embed the library for per-request acceptance policy |

Configuration errors exit non-zero without echoing the connection string. When the client closes stdin the server exits 0.

## Tools, roles and acceptance

Eight tools: `graph_overview`, `graph_get_node`, `graph_list_nodes`, `graph_propose_work`, `graph_record_observation`, `graph_record_conclusion`, `graph_add_support`, `graph_accept_goal`. Inputs and result envelopes are defined by `spec/tool-contracts.json` (embedded copy `toolkit/contracts.json`; regenerate with `python toolkit/generate_contracts.py`, verify with `--check`). Schemas reject unknown fields.

| Role | Tools |
|---|---|
| planner | overview, get_node, list_nodes, propose_work, record_conclusion, add_support |
| worker | overview, get_node, list_nodes, record_observation, record_conclusion |
| solo | planner and worker tools together |

`graph_accept_goal` is never part of a preset; the host grants it with an acceptance method and a policy callback. Discovery lists only granted tools, but authorization runs on every invocation, so a directly called hidden tool returns `permission_denied`. Actor, graph, namespace, producer work item and execution are host binding, never tool arguments.

## Execution binding

A worker session is bound to a work item and an execution ID the host created by moving the work to `running`. Observation writes require that binding and attach the observation to the bound work item. If the host stops or reassigns the work, or completes it, that execution is fenced: later writes return `stale_execution` and the worker must stop. The host, not the worker, completes work and chooses what happens next.

## Invocation and retry behavior

- Every mutating call needs a host-persisted invocation identity (at most 200 characters). The adapter scopes it to graph and actor and derives the service request key; the same identity with the same arguments returns exactly the original success envelope, including its original graph revision, even after later writes or after the graph is closed. The same identity with different arguments returns `idempotency_conflict`. A different identity is a new operation even with identical arguments (no semantic de-duplication).
- Authorization and role checks run again on every retry.
- Only `storage_unavailable` is marked retryable, and only with the same identity.
- `graph-mcp` has no durable invocation identity: each call gets a fresh random identity. If a response is lost the outcome is ambiguous, and a blind retry creates a second operation. Clients must read the graph (`graph_list_nodes` / `graph_get_node`) to reconcile before writing again. Embedding hosts can supply `adapters/mcp.Options.ResolveInvocation` to map requests to identities they persisted; never derive the identity from model-controlled `_meta` or the JSON-RPC request ID.

## Evidence resolver responsibility

The graph stores inline evidence text plus resource references (type, URI, optional external revision and SHA-256). It does not fetch, authorize, hash-check or interpret what a URI points to. The host decides how a reference resolves, whether the reader may open it, and whether the content matches. A support edge records a claim about evidence; it does not verify it. Acceptance records the reviewed closure; the human or validator doing the review is outside the library.

## Limits

| Limit | Default | Where |
|---|---|---|
| Tool input | 256 KiB UTF-8 | `toolkit.Limits.InputBytes` |
| Tool result / mutation result | 64 KiB; adapter value must equal the service's | `toolkit.Limits.ResultBytes`, `graph.Limits.ResultBytes` |
| Overview budget | default 16 KiB; `byte_budget` 1024-65536 | `toolkit.Limits.OverviewBytes`, tool schema |
| `graph_list_nodes` `limit` | default 20, max 100 | `graph` |
| References per command (parents, supports, resources, acceptance edges) | 100 | schema, `graph.Limits.MaxReferences` |
| Invocation identity | 200 characters | `toolkit` |
| Snapshot import/export | 64 MiB | `graph.Limits.SnapshotBytes` (host API) |

Over-limit input fails before any write. Boundary behavior is covered by tests in `toolkit/`. The result cap measures the structured envelope; the MCP frame also carries it as text and is larger (see DECISIONS 008).

## Versions

| Component | Version | Evidence |
|---|---|---|
| Go | module minimum 1.25.0; developed and tested here with Go 1.27.3 | `go.mod`, test runs |
| PostgreSQL | tested against 17.11 (portable runtime); CI uses the `postgres:17` image | `docs/POSTGRES-VERIFICATION.md`, `.github/workflows/test.yml` |
| MCP Go SDK | `github.com/modelcontextprotocol/go-sdk` v1.8.0 | `go.mod` |
| MCP protocol | Go SDK client negotiates `2026-07-28`; a raw client requesting `2025-11-25` (or any other version) is answered `2025-11-25` | `cmd/graph-mcp/stdio_test.go`, `examples/python/parser_scenario.py` |
| JSON Schema | 2020-12, validated with `santhosh-tekuri/jsonschema/v6` v6.0.3 with format assertions (uuid, uri, date-time) | `toolkit/contracts.go` |
| Contract | tool contracts 0.2 (proposed); prompt assets version 1; snapshot schema version 1 | `spec/`, `toolkit/prompts` |

## Testing

```sh
export GRAPH_TEST_DATABASE_URL='postgres://postgres@127.0.0.1:5432/exploration_graph_test?sslmode=disable'
export GRAPH_REQUIRE_POSTGRES=1   # fail, do not skip, without a database
export GRAPH_REQUIRE_PYTHON=1     # fail, do not skip, without Python 3
go vet ./... && go test -count=1 ./...
python toolkit/generate_contracts.py --check
```

The stdio tests build `cmd/graph-mcp` and run it as a subprocess; the Python consumer test needs Python 3.8+ (standard library only). CI additionally runs `go test -race` and `govulncheck`; the race detector needs cgo and was not available in the local Windows environment used for development.

## Second consumer (Python, no Go API)

`examples/python/parser_scenario.py` completes the parser-failure scenario against the compiled `graph-mcp` over stdio and prints the normalized final state; see `examples/python/README.md`.

## License

Apache-2.0; see `LICENSE` and `NOTICE`.
