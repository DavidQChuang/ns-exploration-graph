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

## Full release verification on Linux

With Go, a C compiler, Python 3.8+, and a dedicated PostgreSQL database available:

```sh
export GRAPH_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/exploration_graph_test?sslmode=disable'
bash scripts/verify-release.sh
```

This is the payload used by `.github/workflows/test.yml`: dependency download and verification, generator drift, vet, build/install, the full race suite with PostgreSQL and Python required, zero-skips assertion, explicit stdio tests, direct/MCP/Python example comparison, and a pinned `govulncheck` scan. `GOTOOLCHAIN=local` prevents a dependency from silently selecting a newer compiler. Evidence defaults to ignored `.local/release-evidence/`; `GRAPH_EVIDENCE_DIR` overrides it. Use `GRAPH_SCAN_VULNERABILITIES=0` only for the deliberately old Go 1.25.0 minimum-compiler lane; the patched Go 1.27.3 lane performs the vulnerability scan.

For independent provisioning, run from Linux amd64 (including WSL2) with Docker available:

```sh
bash scripts/verify-clean-environment.sh HEAD 1.25.0
bash scripts/verify-clean-environment.sh HEAD 1.27.3
```

The revision must include these scripts. Each run exports only that Git commit into ignored `.local/clean-environment/`, records the source archive SHA-256, provisions a pinned Ubuntu 24.04 image and checksum-verified official Go compiler, starts a new digest-pinned PostgreSQL 17.11 container, and copies the archive into a new runner with empty Go module/build caches. GCC, libc headers, certificates and Python 3.12 come from Ubuntu's repositories. Provisioning needs network access. No developer toolchain, database state, module/build cache, or local configuration is copied. The container network has no published ports. Containers, their anonymous volumes and the private network are removed when the command exits; source archives, test results, versions and logs remain in the ignored run directory. Tool images remain as an optional local Docker cache.

This local execution proves the shared workflow payload on the recorded environment. It does not execute GitHub Actions checkout/setup/upload operations, and makes no claim about an unrun hosted workflow. Actual evidence and remaining distinctions are recorded in `docs/CLEAN-ENVIRONMENT.md`.
