# Clean environment and CI payload verification

The reproducible command is `bash scripts/verify-clean-environment.sh REVISION GO_VERSION` from Linux amd64 with Docker. Supported verification compilers are the declared minimum Go 1.25.0 and the patched development compiler Go 1.27.3. `GOTOOLCHAIN=local` prevents automatic compiler upgrades. See `scripts/README.md` for commands and cleanup behavior.

Each run exports the named Git commit with `git archive`, records its SHA-256, and copies only that source into a new Ubuntu container. It starts a new PostgreSQL container on a private Docker network, without published ports. No developer configuration, binaries, database contents, module caches, or build caches enter the runner. The compiler is independently downloaded from `go.dev` and checked before extraction. Apt provides GCC, libc development headers, Python and certificates inside the tool image. The tool image is reusable; the source, test database, Go caches and runtime container are fresh for each run.

Pinned inputs (official Go release JSON: `https://go.dev/dl/?mode=json&include=all`):

- Ubuntu 24.04: `ubuntu:24.04@sha256:24737cef17f4e22b1977df7d9a05e77c4e28b7da5b8911f8d8ccda136bf3573c`.
- PostgreSQL 17.11: `postgres:17.11@sha256:b8faeff7712663c2773a190181e1246e37e364b887611c04658c28b44e2408fe`.
- `go1.25.0.linux-amd64.tar.gz`: SHA-256 `2852af0cb20a13139b3448992e69b868e50ed0f8a1e5940ee1de9e19a123b613`.
- `go1.27.3.linux-amd64.tar.gz`: SHA-256 `65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40`.

Provisioned on 2026-10-27 using WSL2 Ubuntu 22.04 and Docker Engine 28.3.2. Inside the verification images: Ubuntu 24.04, Python 3.12.3, GCC 13.3.0 (`13.3.0-6ubuntu2~24.04.1`), and glibc 2.39 (`2.39-0ubuntu8.9`). Ubuntu repository packages are resolved during provisioning; the Dockerfile does not claim a permanently reproducible package mirror. The recorded image IDs and package versions identify the environment actually tested.

## Verification record

Initial committed-source verification passed at `01a1bf1cf69b` on 2026-10-27. Both commands exited 0:

```sh
bash scripts/verify-clean-environment.sh 01a1bf1 1.25.0
bash scripts/verify-clean-environment.sh 01a1bf1 1.27.3
```

Both source archives have SHA-256 `74322e74d91dffd5e7ff1ce5f56a3fa7a09beb13383b6fd74ee92d1a228ff43d`. Tool image IDs: Go 1.25.0 `sha256:67b4151ef689e091606acc3a7fbbd63ef171aa02a5336d2eadb5317a9c473084`; Go 1.27.3 `sha256:eb1f98d818d6bb6c02c19c7327675f84bc5383ea22b42e6e43e5fff86fe3a88f`. The fresh server identified itself as PostgreSQL 17.11 (`Debian 17.11-1.pgdg13+2`, x86_64, compiled by GCC 14.2.0). The runner used the versions above with `CGO_ENABLED=1`, `GOTOOLCHAIN=local`, `CI=1`, `GRAPH_REQUIRE_POSTGRES=1` and `GRAPH_REQUIRE_PYTHON=1`; both Go cache paths were absent before the first command.

Each compiler lane passed dependency download/verification, generator drift, `go vet ./...`, `go build ./...`, `go install ./cmd/graph-mcp`, and `go test -race -json -count=1 ./...`: **142 test/subtest passes, 0 failed tests, 0 skipped tests, 7 test-bearing packages passed**. The shared conformance and memory helper packages have no standalone test files; Go emits two package-level skip events for those, which are distinct from skipped tests. Their behavior is exercised through the memory and PostgreSQL conformance entry points. No race was reported.

The explicit stdio suite and both Python consumer modes (with and without the lost-response drill) passed. The separately executed documented direct, in-process MCP and Python stdio commands produced equal normalized reports, including a met goal and graph-only resume. In the patched Go 1.27.3 lane, govulncheck v1.9.0 reported 0 affecting vulnerabilities, 0 findings in imported packages, and 1 finding in a required module whose vulnerable code is not called. The minimum-compiler lane intentionally does not run the vulnerability scan.

Retained local evidence directories (relative to the repository; ignored):

- `.local/clean-environment/20261027T121908Z-01a1bf1cf69b-go1.25.0-257200/`.
- `.local/clean-environment/20261027T121909Z-01a1bf1cf69b-go1.27.3-257215/`.

Each contains `source.tar`, `source.sha256`, `environment.txt`, `provision.log`, `verification.log`, `postgres.log`, and `evidence/` with `race-tests.jsonl`, `stdio-tests.log`, the three example reports and Python transcript; the patched lane also retains `govulncheck.log`. Containers, anonymous database volumes and private networks were removed after successful completion. This proves the named source revision; a final candidate with additional lifecycle tests will be verified separately before sign-off.

## Hosted CI distinction

`.github/workflows/test.yml` uses the same `scripts/verify-release.sh` payload on Ubuntu 24.04 with both compiler versions, Python 3.12, and the same PostgreSQL image digest. The payload downloads and verifies dependencies, checks contract drift, vets/builds/installs, executes the full suite with the race detector, requires PostgreSQL and Python, rejects skipped tests, exercises stdio plus the documented direct/MCP/Python examples, and scans reachable vulnerabilities with pinned govulncheck v1.9.0 in the Go 1.27.3 lane.

Local execution does not run GitHub's checkout/setup/upload actions or validate hosted runner orchestration. There is no Git remote and no GitHub Actions run has been dispatched. The minimum Go 1.25.0 compiler verifies compatibility; its old standard library is not a deployment recommendation and the vulnerability scan runs with Go 1.27.3.
