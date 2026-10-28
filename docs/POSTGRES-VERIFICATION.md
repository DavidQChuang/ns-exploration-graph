# PostgreSQL verification evidence

Runtime: Windows amd64; Go 1.27.3 (`.local/go/bin/go.exe`); PostgreSQL 17.11 EDB build 5 (`.local/pgsql/bin/`). Downloaded Go from the official release endpoint and checked SHA-256 against official release JSON: `f26081f438319757f8a1b46d7d0b2c5ec7583d4038e485e0aa725cfde7885fff`.

PostgreSQL was obtained through the current EDB binary link reached from the PostgreSQL Windows downloads page: `https://sbp.enterprisedb.com/getfile.jsp?fileid=1260616`, redirecting to `https://get.enterprisedb.com/postgresql/postgresql-17.11-5-windows-x64-binaries.zip`. Download SHA-256: `80379b2c04d51c30225532e0ae04509899141e9957ed096fe749d7fd9df8f82f`. EDB binaries are not Authenticode signed and `.sha256`/`.sha256sum` endpoints returned HTTP 403. This hash records the artifact obtained over official HTTPS; no independent publisher checksum was available. `scripts/provision-local.ps1` pins and checks this artifact for repeat provisioning.

Cluster: `.local/pgdata`; log: `.local/postgres.log`; loopback `127.0.0.1:55439`; database `exploration_graph_test`. DSN:

```
postgres://postgres@127.0.0.1:55439/exploration_graph_test?sslmode=disable
```

No global PATH, Windows service, or global database configuration changes. Startup/status/stop instructions are in `scripts/README.md`. `postgres-local.ps1` uses a hidden startup helper and waits only for `pg_ctl` itself rather than its PostgreSQL descendant process tree.

## Adapter contract

`graph/store/postgres` persists the seven normalized contract tables. Mutations use a SQL transaction and `SELECT ... FOR UPDATE` on the graph row. Creation inserts with `ON CONFLICT DO NOTHING` then acquires that same row lock. The shared semantic callback evaluates committed request history before preconditions. Only new immutable revisions/resources/provenance/events are inserted; dependency removal and acceptance invalidation are explicit updates. Commit completes before returning. Ordinary reads use a repeatable-read transaction and exclude graph events; the event feed is a separate bounded query.

`Migrate` embeds the canonical [initial migration](../graph/store/postgres/migrations/001_initial.sql) and serializes first migration through a transaction advisory lock. No denormalized whole-snapshot storage table is added.

## Actual test execution

The sandbox initially denied outbound download sockets and Windows restricted-token startup. Approved host execution downloaded tools and started the isolated database. An unprivileged sandbox `pg_ctl status` falsely reported no server while host inspection confirmed PID 46012; subsequent checks used host execution and did not restart the existing instance.

Initial command on October 27, 2026:

```powershell
$env:GRAPH_TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55439/exploration_graph_test?sslmode=disable'
$env:GRAPH_REQUIRE_POSTGRES='1'
& .local/go/bin/go.exe test -count=1 -v ./graph/store/postgres
```

Initial actual result: five injected rollback checkpoints passed; concurrent claims passed; opposing concurrent dependency additions passed. Lost-response retry and concurrent same-key creation preserved identical serialized results but initially exposed differing dynamic Go map types after SQL JSON decoding. The core lane is canonicalizing initial result map types; full rerun is required before claiming G1 complete.

CI (`.github/workflows/test.yml`) provisions PostgreSQL 17, sets the required database URL, and executes `go test -race -count=1 ./...`. CI or `GRAPH_REQUIRE_POSTGRES=1` turns missing PostgreSQL configuration into a failure. A configured unreachable database always fails. Developer runs without either requirement explicitly report a skipped database suite rather than claiming persistence coverage.

## Current scaling boundary

The initial store interface loads complete node/revision/edge/resource/acceptance aggregates for consistent reads, using six SELECT queries plus transaction begin/commit. Update also loads complete event history for request replay. This is an explicit performance limitation, not a scale-independent claim. G3 requires measurements at 100, 1,000 and 10,000 nodes and further work if those aggregate loads are unsuitable. No production throughput claim is made.

## Completed rerun and performance checkpoint

Root independently reran all PostgreSQL tests after canonical Result normalization: persistence/reconnect and lost-response replay, five rollback points, simultaneous claims, concurrent opposite dependency additions, same-key concurrent creation, and shared semantic conformance all passed against actual PostgreSQL 17.11. Root committed the storage checkpoint as `b5d2f53`.

Actual benchmark completed successfully (`go test -run '^$' -bench BenchmarkPostgresAggregate -benchtime=1x -count=1 -benchmem ./graph/store/postgres`), 20.183 seconds total. Raw output is ignored `.local/postgres-bench.txt`; source is `graph/store/postgres/benchmark_test.go`. Host: Windows amd64, Ryzen 9 7900X3D (12 cores/24 logical processors), 68,307,763,200 bytes physical RAM. These are single-operation development measurements, not statistical throughput estimates. Fixture setup is excluded and uses synthetic independent work rows.

- 100 nodes: overview 14.777 ms, 16,163 serialized JSON bytes, 9,345,920 allocated bytes; traversal 1.517 ms, 14,317 JSON bytes, 347,256 allocated bytes; first acceptance invalidation 5.091 ms, 96 JSON bytes, 983,824 allocated bytes.
- 1,000 nodes: overview 97.058 ms, 16,167 JSON bytes, 69,687,808 allocated bytes; traversal 4.285 ms, 14,979 JSON bytes, 2,348,024 allocated bytes; first acceptance invalidation 14.654 ms, 96 JSON bytes, 10,237,456 allocated bytes.
- 10,000 nodes: overview 1,093.837 ms, 16,171 JSON bytes, 683,922,928 allocated bytes; traversal 54.245 ms, 14,981 JSON bytes, 25,101,632 allocated bytes; first acceptance invalidation 209.018 ms, 96 JSON bytes, 107,439,632 allocated bytes.

Overview/traversal each issue six SELECTs. Invalidation benchmark includes a six-query revision lookup followed by the mutation's lock query, six aggregate queries, and event history query (14 SELECTs total). Counts follow the adapter's fixed query paths; no driver query profiler was installed. Allocations are Go benchmark bytes allocated per operation, not peak resident memory. The 10,000-node overview allocation (~684 MB) and latency require investigation before declaring G3 performance satisfactory; whole aggregate loading and repeated candidate serialization remain known limits.

`go vet ./...` and `go build ./...` completed successfully before the dependency upgrade. Local `go test -race` failed explicitly because `CGO_ENABLED=0`; no C compiler was found on PATH. Linux GitHub Actions is configured for race tests but has not run (no remote exists).

## Vulnerability checkpoint and outstanding verification

Installed official `golang.org/x/vuln/cmd/govulncheck@v1.9.0` into ignored `.local/bin/govulncheck.exe`. Actual scan found three reachable advisories:

- GO-2026-5004: pgx v5.7.6 SQL placeholder sanitization, fixed in v5.9.2.
- GO-2026-6629: x/text v0.24.0 PRECIS panic, fixed in v0.41.0.
- GO-2026-5970: x/text v0.24.0 normalization infinite loop, fixed in v0.39.0.

Upgraded `github.com/jackc/pgx/v5` to v5.9.2 and `golang.org/x/text` to v0.41.0; `go mod tidy` completed successfully (x/sync also advanced to v0.22.0). The bounded post-upgrade scan/PG/vet/build command was automatically rejected: approval review cited the active requirement to pause and preserve checkpoints at 89% usage. No workaround was attempted. Therefore the upgraded dependency checkpoint is **not yet regression-tested or rescanned**. Run the following after authorized resumption:

```powershell
$env:GOCACHE=(Join-Path $PWD '.local/gocache')
$env:PATH=(Join-Path $PWD '.local/go/bin')+';'+$env:PATH
$env:GRAPH_TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55439/exploration_graph_test?sslmode=disable'
$env:GRAPH_REQUIRE_POSTGRES='1'
& .local/bin/govulncheck.exe ./...
& .local/go/bin/go.exe test -count=1 ./graph/store/postgres
& .local/go/bin/go.exe vet ./...
& .local/go/bin/go.exe build ./...
```

No benchmark process remains running. The earlier startup command's host shell session 16273 may still be waiting on PostgreSQL descendants due to the previous `Start-Process -Wait` behavior; the script has been corrected. Do not terminate that process tree while the database is needed. PostgreSQL remains intentionally running; inspect `pg_ctl status` before resuming.

## 2026-10-27 — G3 bounded reads checkpoint (core_perf lane)

This section supersedes "Current scaling boundary" for ordinary reads and the per-mutation event-history load. See docs/DECISIONS.md 004. Base commit `41752a2`; changes are uncommitted in the shared checkout at the time of measurement.

Host, from `Get-CimInstance` and `go version`: AMD Ryzen 9 7900X3D, 12 cores/24 logical processors, max clock 4,401 MHz; 68,307,763,200 bytes physical RAM; Windows 11 Education 10.0.22621, 64-bit. Toolchain: go1.27.3 windows/amd64, invoked from WSL2. Database: the existing local PostgreSQL 17.11 instance on 127.0.0.1:55439. The server version was not re-queried in this run because `psql.exe` was not found next to the runtime. The database was not restarted.

### What changed

- **Ordinary reads:** these use `ReadScoped`. Every read issues BEGIN, the graph row, one current-revision join (`nodes` joined to `node_revisions` on the current-revision key), all edges, and COMMIT. GetNode/ExplainGoal add the detail node's history, its resource references and its acceptances. They do not read historical revisions of other nodes, unrelated resources/acceptances, or events.
- **Mutations:** these use `UpdateRequest`. They load only the event for the request ID, through the unique `(graph_id, request_id)` index. The persistence baseline is now an identity summary instead of a full JSON clone of the aggregate.
- **Projection and traversal:** these use a one-pass node/adjacency index (O(N+E)) instead of repeated linear scans. That covers eligibility, recursive freshness (memoized per read), traversal frontier expansion, and related records. Projection sizing is incremental with an exact-encoding fallback; work priorities are decoded once instead of inside the sort comparator. Acceptance invalidation indexes edges once per mutation.

### Verification commands (actual, WSL calling Windows Go)

```
export GOCACHE=$(wslpath -w $PWD/.local/gocache) GOMODCACHE=$(wslpath -w $PWD/.local/gomodcache) GRAPH_TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55439/exploration_graph_test?sslmode=disable' GRAPH_REQUIRE_POSTGRES=1 WSLENV=GOCACHE:GOMODCACHE:GRAPH_TEST_DATABASE_URL:GRAPH_REQUIRE_POSTGRES
./.local/go/bin/go.exe vet ./...                      # graph packages: clean
./.local/go/bin/go.exe test -count=1 ./...            # all packages ok, PostgreSQL suite executed (not skipped)
./.local/go/bin/go.exe test -run '^$' -bench BenchmarkPostgresAggregate -benchtime=20x -count=3 -benchmem ./graph/store/postgres
./.local/go/bin/go.exe test -run '^$' -bench BenchmarkPostgresAggregate -benchtime=1x  -count=3 -benchmem ./graph/store/postgres
```

The race detector was not run: local Go has `CGO_ENABLED=0` and no C compiler.

New tests:
- **Shared conformance suite** (`graph/conformance/bounded.go`), run on memory and PostgreSQL:
  - `ProjectionUTF8BudgetAnchorsAndRetrieval`
  - `CursorsCoverOnceAndRejectAfterMutation`
  - `EventsResumeAcrossReconnectAndConcurrentWrites`
  - `SnapshotRestoreRoundTripAndRejection`
  - `ScopedReadsMatchCompleteAggregateReads`
- **PostgreSQL only:**
  - `TestPostgresRestoreRejectionLeavesNoRows`: no rows in any of the seven tables after a rejected restore or an injected failure at each of the five persistence checkpoints, and exact row counts after a successful restore.
  - `TestPostgresReadsUseBoundedStatementSets`: statement counts are the same for 3-node and 150-node graphs; no event-history SELECT during mutations.
- **Internal:** `TestIncrementalProjectionMatchesExactEncoding`, randomized and including multi-byte, HTML-escaped and U+2028 text. It was checked to fail when the sizer was deliberately made off by one byte.

### Benchmark method

The fixture is unchanged: the same synthetic independent open work items under the origin, plus one goal accepted from one observation, with fixture insertion excluded. Statement counts are now measured with a pgx `QueryTracer` and include BEGIN/COMMIT; the baseline's "6 selects" was derived from the code path, not traced. Allocation figures are Go benchmark bytes per operation, not peak RSS. These are local development measurements, not throughput promises.

Invalidation methodology changed:
- **Old benchmark:** timed a six-query full `Read` (used to look up the revision) plus the `ReviseNode`.
- **New benchmark:** times only the `ReviseNode` that invalidates a current acceptance. It re-establishes a current acceptance with the timer stopped, so every iteration is a first invalidation. The graph grows by one revision per iteration, which explains the slight allocation drift across counts.

### Results

The baseline is HEAD `41752a2`, exported with `git archive` into ignored `.local/baseline-head`, run with `-benchtime=1x -count=3`. Raw output for all runs: `.local/bench-baseline-head.txt`, `.local/bench-g3-1x.txt`, `.local/bench-g3-20x.txt`. Values are the median, with the range in parentheses.

| Nodes | Operation | Baseline 1x×3 | New 1x×3 | New 20x×3 | Result JSON bytes | Statements (SELECTs), new |
|---|---|---|---|---|---|---|
| 100 | overview | 14.29 ms (13.70–14.33), 9.04 MB | 2.89 ms (2.42–4.49), 0.56 MB | 1.70 ms (1.66–1.76), 0.55 MB, 6.9k allocs | 16,163 | 5 (3) |
| 100 | traversal | 1.30 ms (1.12–1.49), 0.27 MB | 1.47 ms (1.47–1.51), 0.27 MB | 1.05 ms (1.01–1.05), 0.26 MB | 14,317 | 5 (3) |
| 100 | get_node | not measured | 2.36 ms (1.86–2.73), 0.26 MB | 1.31 ms (1.20–1.38), 0.23 MB | 371–372 | 8 (6) |
| 100 | invalidation | 3.55 ms (3.50–4.45), 0.99 MB | 4.02 ms (3.27–4.09), 0.30 MB | 2.70 ms (2.56–2.83), 0.34–0.57 MB | 96–99 | 17 (8) |
| 1,000 | overview | 108.70 ms (98.26–179.44), 70.9 MB | 9.97 ms (7.58–11.19), 3.69 MB | 8.04 ms (7.48–8.37), 3.70 MB, 53.8k allocs | 16,167 | 5 (3) |
| 1,000 | traversal | 4.80 ms (4.55–4.94), 2.34 MB | 4.60 ms (4.53–4.96), 2.61 MB | 4.49 ms (4.48–4.80), 2.61 MB | 14,979–14,984 | 5 (3) |
| 1,000 | get_node | not measured | 4.76 ms (4.74–5.45), 2.43 MB | 4.62 ms (4.18–4.77), 2.42 MB | 372 | 8 (6) |
| 1,000 | invalidation | 15.28 ms (13.54–17.57), 10.25 MB | 6.82 ms (6.77–7.47), 2.91 MB | 7.28 ms (6.71–7.49), 2.93–3.11 MB | 96–99 | 17 (8) |
| 10,000 | overview | 1,042.99 ms (1,039.34–1,092.55), 683.3 MB, 969k allocs | 77.22 ms (66.23–83.33), 36.9 MB | 60.35 ms (59.09–62.41), 36.9 MB, 522k allocs | 16,171 | 5 (3) |
| 10,000 | traversal | 37.86 ms (36.73–37.89), 25.1 MB, 341k allocs | 47.39 ms (46.89–50.91), 28.2 MB | 34.15 ms (34.10–35.06), 28.2 MB, 331k allocs | 14,981 | 5 (3) |
| 10,000 | get_node | not measured | 43.00 ms (38.86–44.49), 26.0 MB | 30.82 ms (30.27–33.62), 26.0 MB | 372 | 8 (6) |
| 10,000 | invalidation | 115.89 ms (108.77–117.55), 107.5 MB, 733k allocs | 59.45 ms (55.01–62.18), 29.5 MB | 39.49 ms (37.85–40.88), 29.5–29.7 MB, 352–355k allocs | 96–99 | 17 (8) |

Statement counts:
- **Baseline:** overview/traversal ran 6 SELECTs plus BEGIN/COMMIT. Invalidation ran 14 SELECTs: the timed lookup Read, the lock, six aggregate queries, and the full event history.
- **New invalidation:** 17 statements per operation. The 8 SELECTs are the lock, graph, nodes, revisions, edges, resources, acceptances and request event. The other 9 are BEGIN, two revision inserts, two current-pointer updates, one acceptance update, one graph update, one event insert and COMMIT.
- **Size independence:** statement counts no longer depend on graph size; the PostgreSQL test above asserts this.

### Remaining bounds (not solved in this unit)

- **Linear reads:** ordinary reads remain O(N+E) in rows transferred. They load every node's current revision, including the payload, and every edge in the graph partition. At 10,000 nodes, traversal and GetNode cost about 30–35 ms and 26–28 MB each, and did not improve over baseline. The 10k traversal allocates about 12% more because of the adjacency index, and the single-shot 1x median was slower (47 vs 38 ms).
  - **Why not bounded:** a bounded traversal would need recursive SQL plus per-result recursive freshness (support closure) in SQL. That would create a second implementation of semantic rules, so it was not attempted without a separate equivalence plan.
- **Overview:** a complete candidate scan is still required. The specified greedy whole-node omission can admit a later, smaller candidate after skipping a larger one. Totals and omitted counts need counts over the whole graph. Eligibility needs every prerequisite's current state. At 10,000 nodes the overview is now about 60 ms and 37 MB, dominated by the row load and summary construction.
  - **Possible exact optimization (not implemented):** stop the scan when the remaining budget is below the minimum possible candidate size.
- **Token counting:** when `CountTokens` is supplied, each candidate that passes the byte check is still encoded in full for token counting, which is O(candidates × result size).
- **Mutations:** these still load the complete node history, edges, resources and acceptances per Decision 001. TransitionWork's execution-ID uniqueness check needs every historical work revision; it is the event history that is no longer loaded.
- **Snapshots:** snapshot/export reads are whole-graph by definition. `SnapshotBytes` is enforced after loading, not before.
- **Unbounded results:** ListEligibleWork has no limit or cursor in the current public API, so its result size is unbounded. Traversal results (up to 1,000 summaries) are not checked against the result byte limit in the core. Both would need contract decisions; neither was changed here.
- **Memory store:** `UpdateRequest` still JSON-clones the aggregate. It exists for semantic coverage, not performance.
