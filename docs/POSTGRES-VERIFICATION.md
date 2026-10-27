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

`Migrate` embeds `migrations/001_initial.sql`, copies the frozen spec DDL, and serializes first migration through a transaction advisory lock. No denormalized whole-snapshot storage table is added.

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
