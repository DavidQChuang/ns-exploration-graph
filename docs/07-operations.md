# Operations guide

[Documentation](README.md) · [Agents](01-agents.md) · [Go library](02-go-library.md) · [MCP](03-mcp.md) · [Workflow](04-workflow.md) · [Architecture](05-architecture.md) · [Storage](06-storage.md) · [Operations](07-operations.md) · [Roadmap](08-roadmap.md)

This guide covers local builds, PostgreSQL-backed use, stdio process setup, and recovery. The project is a local source release; it has no hosted service or installer.

## 1. Prerequisites and build

The module minimum is Go 1.25.0; the release was verified with Go 1.25.0 and 1.27.3. For routine development, prefer the patched Go 1.27.3 lane. PostgreSQL is required for the persistent store and `graph-mcp`; the in-memory store does not need it. The Python example requires Python 3.8 or later and uses only the standard library.

From the repository root:

```sh
go build ./...
go build -o bin/graph-mcp ./cmd/graph-mcp
```

For the release payload using already-installed tools, run `bash scripts/verify-release.sh` with Go, a C compiler, Python 3.8+, and a dedicated PostgreSQL test database available. For independent clean-environment verification, run `scripts/verify-clean-environment.sh` from Linux amd64 (including WSL2) with Git, Docker, and network access; it provisions pinned Go and PostgreSQL tool images and an isolated database, so no existing host Go installation or database is required. See [CLEAN-ENVIRONMENT.md](CLEAN-ENVIRONMENT.md) for the exact commands and scope of each route. Both procedures build the scripted example.

## 2. Choose storage for the job

The memory store is useful for embedding, deterministic examples, and semantic tests. Its state lasts only for the lifetime of the process and it does not establish PostgreSQL durability, database concurrency, or rollback behavior.

The PostgreSQL store persists graphs and serializes mutations by graph. The library's `postgres.Store.Migrate` method applies the embedded initial migration. `graph-mcp` does not run migrations and does not create a graph: prepare the schema and create the graph through the trusted host API before launching it. The SQL migration is [001_initial.sql](../graph/store/postgres/migrations/001_initial.sql).

## 3. Configure the environment

`graph-mcp` reads `EXPLORATION_DATABASE_URL`. The test suite reads `GRAPH_TEST_DATABASE_URL`. Set the latter to a dedicated test database; tests retain data and do not drop an existing database. `GRAPH_REQUIRE_POSTGRES=1` makes missing database configuration fail instead of skipping PostgreSQL coverage. A configured but unreachable database always fails.

For the Python consumer, `GRAPH_REQUIRE_PYTHON=1` makes CI/release checks fail if Python is unavailable. `GRAPH_PYTHON` selects the Python executable for `scripts/verify-release.sh`; its default is `python3`. The release script also accepts `GRAPH_EVIDENCE_DIR` to choose where it writes verification evidence, defaulting to ignored `.local/release-evidence/`. `GRAPH_SCAN_VULNERABILITIES=0` is documented for the minimum compiler lane; the patched Go lane performs the scan.

Do not put credentials in logs or example output. Startup errors from `graph-mcp` avoid echoing the database URL.

## 4. Start the stdio server

The process is bound to one existing namespace, graph UUID, actor, and role. Example:

```sh
export EXPLORATION_DATABASE_URL='postgres://user:pass@host:5432/db?sslmode=verify-full'
./bin/graph-mcp -namespace team-a -graph GRAPH_UUID -actor host-service -role planner
```

Flags implemented by the command:

| Flag | Behavior |
|---|---|
| `-namespace`, `-graph`, `-actor` | Trusted binding; all are required and the graph must already exist. |
| `-role` | `planner` (default), `worker`, or `solo`. |
| `-work`, `-execution` | Must be supplied together as valid UUIDs. Bind a host-assigned running work item and its execution; required for worker writes. |
| `-acceptance` | Grants `graph_accept_goal` with method `human`, `validator`, or `agent`. Off by default. |

The binary's acceptance grant is process-wide and has no per-request policy callback. An embedding host should use the library when acceptance needs a policy decision for each request. Actor, graph, namespace, work, and execution are not model-supplied tool fields.

The program uses stdin/stdout for MCP stdio. Keep stdout reserved for protocol traffic; send process diagnostics to stderr. Closing stdin ends the server with exit status 0. Ctrl-C cancels its signal context. Startup configuration failures exit nonzero, and the connection string is not included in the error. The process serves one graph; it does not launch workers, authorize remote clients, or migrate/create data.

## 5. Know the limits

Default tool limits and fixed schema limits:

| Input/output | Default or maximum |
|---|---:|
| Tool input | 256 KiB UTF-8 by default |
| Tool result | 64 KiB; configured adapter limit must match service limit |
| Overview | 16 KiB default; `byte_budget` accepts 1–64 KiB |
| List nodes | 20 default, 100 maximum |
| Parent, support, or resource references per command | 100 |
| Invocation identity | 200 characters |
| Host snapshot import/export | 64 MiB default |
| Traversal | Depth 8/results 100 default; caps 64/1,000 |

The service measures the structured result envelope in UTF-8 bytes. MCP carries the structured envelope as text too, so the complete protocol frame is larger than that result limit. The 64 KiB cap is not a maximum frame size. The stdio transport line cap is the configured input limit plus 16 KiB of protocol allowance.

The graph is intended for modest investigation graphs. PostgreSQL ordinary reads still transfer O(N+E) graph rows; mutations load graph state/history. The measured 10,000-node cases and unresolved unbounded read results are documented in [POSTGRES-VERIFICATION.md](POSTGRES-VERIFICATION.md). These figures describe a test environment, not a throughput guarantee.

## 6. Diagnose failures and recover safely

Tool failures use stable codes: `invalid_argument`, `not_found`, `permission_denied`, `revision_conflict`, `invalid_transition`, `invalid_relation`, `cycle_detected`, `stale_evidence`, `stale_execution`, `idempotency_conflict`, `already_exists`, `graph_read_only`, `budget_too_small`, `stale_cursor`, `limit_exceeded`, and `storage_unavailable`.

| Symptom | Response |
|---|---|
| `storage_unavailable` | Check database reachability and schema. It is the only automatically retryable code; retry a mutation only with the same persisted invocation identity. |
| `stale_execution` | Stop the old worker. The host must stop/reassign the work and create a new execution binding. This fence protects graph writes; the host must fence external side effects separately. |
| `revision_conflict`, `stale_evidence`, or `stale_cursor` | Reread the affected nodes/current graph and make a new decision. A cursor is bound to the graph revision; restart the query after a mutation. |
| `idempotency_conflict` | The identity was reused with different input or trusted context. Do not alter arguments under that key; reconcile the original invocation. |
| `permission_denied` | Check the bound role and host grant. Hidden tool discovery is not authorization; each call is checked. |
| `budget_too_small` | For `graph_overview`, increase `byte_budget` within 1–64 KiB (default 16 KiB). If the error includes `required_bytes`, use it to choose a sufficient budget. Required overview anchors, including mandatory goals, cannot be dropped. |
| `limit_exceeded` | Reduce the payload or retrieve data in smaller pages. For a single oversized node, use the authorized host API/export path; do not treat it as an overview-budget problem. |
| `invalid_argument`, `invalid_transition`, `invalid_relation`, `cycle_detected`, `already_exists`, `graph_read_only`, `not_found` | Check UUIDs, references, lifecycle state, and graph identity before issuing a corrected operation. |

`graph-mcp` assigns a fresh random invocation identity to each call. If its response is lost, the write outcome is ambiguous; do not blindly repeat the mutation. Read with `graph_list_nodes` or `graph_get_node` to reconcile first. A host embedding the toolkit can persist and resolve identities for safe same-identity retry. The same identity must keep the original arguments and binding.

## 7. Separate snapshots from database backups

The host API provides a consistent JSON graph snapshot and import into a new, unused graph identity. Snapshot schema version 1 is the only supported version; restore rejects running work and does not transfer request-id history. The default snapshot size limit is 64 MiB. The snapshot contract is for graph portability; it does not back up PostgreSQL roles, database configuration, or unrelated databases.

PostgreSQL backup, retention, restore drills, access control, and disaster recovery belong to the operator's database procedures. This repository does not provide a `graph-mcp` backup/export command. Resource references point outside the graph; the host that owns those artifacts is responsible for their authorization, availability, integrity checking, and retention.

## 8. Verify the release evidence

The release payload is `bash scripts/verify-release.sh` with a dedicated test database. It requires PostgreSQL and Python, runs vet/build, the race suite with zero skips, stdio and consumer checks, and (in the patched compiler lane) the pinned vulnerability scan. [POSTGRES-VERIFICATION.md](POSTGRES-VERIFICATION.md) records database-specific checks; [RELEASE-ARTIFACTS.md](RELEASE-ARTIFACTS.md) records archive inventory and checksums.

The current workflow pins PostgreSQL 17.11 by image digest. The release matrix was executed locally on the documented clean environment. Hosted GitHub Actions has not run; see [CLEAN-ENVIRONMENT.md](CLEAN-ENVIRONMENT.md). Do not describe local execution of the shared payload as a hosted CI run.
