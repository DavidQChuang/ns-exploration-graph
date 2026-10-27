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
