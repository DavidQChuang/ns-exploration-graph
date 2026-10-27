# Local runtimes and database checks

Tools stay in ignored `.local/`; no PATH, Windows service, firewall, or machine configuration changes are required. Download Go's Windows amd64 ZIP from https://go.dev/dl/ and verify its SHA-256 against the release JSON at https://go.dev/dl/?mode=json before extraction. Download PostgreSQL Windows binaries via https://www.postgresql.org/download/windows/ → EDB → binaries. Extract the `pgsql/` directory into `.local/`.

In PowerShell at the repository root:

```powershell
. ./scripts/postgres-local.ps1 start
$env:GRAPH_REQUIRE_POSTGRES='1'
& .local/go/bin/go.exe test -count=1 -v ./graph/store/postgres
& .local/go/bin/go.exe test ./...
& ./scripts/postgres-local.ps1 status
& ./scripts/postgres-local.ps1 stop
```

The script binds `127.0.0.1:55439`, stores its cluster in `.local/pgdata`, creates only `exploration_graph_test`, and sets `GRAPH_TEST_DATABASE_URL` in the invoking session when dot sourced. Trust authentication applies only to this isolated loopback development cluster. Never expose that cluster externally. Tests add UUID graph identities and retain data for inspection; they do not drop an existing database.

On a host with installed PostgreSQL or CI, set `GRAPH_TEST_DATABASE_URL` to a dedicated test database. `GRAPH_REQUIRE_POSTGRES=1` requires database execution; CI also requires it automatically. Missing configuration fails required runs. A configured but unavailable database always fails. CI starts a PostgreSQL 17 container and runs the complete test suite with the race detector.

The Windows restricted execution sandbox cannot access the server's loopback socket and may report the running cluster as stopped. Use an approved ordinary host execution context for starting/inspecting PostgreSQL and running database tests; inspect `pg_ctl status` and the log before restarting anything. All background startup uses a hidden helper.
