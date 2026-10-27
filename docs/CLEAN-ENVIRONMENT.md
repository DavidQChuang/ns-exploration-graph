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

## Candidate verification: ba4769a (2026-10-27)

The frozen candidate `ba4769a5c0692eba96b7f41d99e928f130030316` was exported independently for both compiler lanes. Both source archives have SHA-256 `23bb80403475bd3fdbee41432fb5e8b72570118985a0e1a579d89deb692e459e`. Commands:

```sh
bash scripts/verify-clean-environment.sh ba4769a5c0692eba96b7f41d99e928f130030316 1.25.0
bash scripts/verify-clean-environment.sh ba4769a5c0692eba96b7f41d99e928f130030316 1.27.3
```

The prepared image IDs, immutable Ubuntu/PostgreSQL image digests, compiler archive hashes and runtime versions match the initial verification above. Each lane used a new source copy, new runner, absent `/isolated/buildcache` and `/isolated/modcache`, and a newly initialized PostgreSQL 17.11 database. Both used native Linux amd64, `CGO_ENABLED=1`, `GOTOOLCHAIN=local`, `CI=1`, and required PostgreSQL/Python. No developer cache or database state was copied.

- **Go 1.27.3: exit 0.** Dependency verification, contract drift, vet, build/install and full race suite passed: **164 test/subtest passes, 0 failed tests, 0 skipped tests, 7 test-bearing packages passed**. The two helper packages still have no standalone test files. Explicit stdio tests and both Python drill modes passed. The documented direct, in-process MCP and Python stdio reports agreed: graph revision 14, two completed work items, four reviewed evidence nodes, two reviewed resources, a met goal and graph-only resume. Govulncheck v1.9.0 reported 0 affecting vulnerabilities, 0 findings in imported packages and 1 finding in a required module whose vulnerable code is not called.
- **Go 1.25.0: exit 1.** Dependency verification, generator drift, vet and build/install passed, but the full race suite reported **160 test/subtest passes, 4 failed test events, 0 skipped tests, 5 test-bearing packages passed and 2 failed packages**. The underlying failure was `HostResourceURIsMatchPortableContract` in both memory and PostgreSQL conformance: `RecordObservation` accepted `http://[not-ipv6]/log`, and export plus snapshot validation returned no error (`graph/conformance/suite.go:549`). The parent conformance tests also failed, accounting for four failed test events. No data race was reported. Because the shared script stops on this failure, its later explicit stdio/example commands did not run in this lane; no vulnerability scan is configured for the minimum-compiler lane. This candidate does **not** satisfy the declared minimum compiler's complete suite and cannot receive environment sign-off from these results.

Retained evidence directories (relative to the repository; ignored):

- `.local/clean-environment/20261027T171919Z-ba4769a5c069-go1.25.0-299138/` — failed minimum-compiler run.
- `.local/clean-environment/20261027T171920Z-ba4769a5c069-go1.27.3-299155/` — successful patched-compiler run.

Both retain `source.tar`, `source.sha256`, `environment.txt`, `provision.log`, `verification.log`, `postgres.log` and `evidence/race-tests.jsonl`. The successful lane additionally retains `evidence/stdio-tests.log`, all three example reports, Python transcript and `govulncheck.log`. Containers, anonymous database volumes and private networks were removed after both commands finished. Hosted Actions remains unexecuted; final minimum-compiler compatibility needs a correction and another recorded run.

## Focused URI correction check (2026-10-27)

Before committing the correction, diagnostic source copies overlaid the seven ready implementation/test files (`spec/contracts.go`, `spec/contracts_test.go`, new `spec/uri_test.go`, `graph/snapshot.go`, `graph/conformance/suite.go`, `toolkit/contracts.go`, `toolkit/boundaries_test.go`) on an archived source baseline. Both native compiler lanes exited 0 under the race detector with fresh PostgreSQL and caches. Each passed the 27 named cases in `TestURIIPLiteralFormatStableAcrossCompilers`, `TestURIFormatsMatchHostAndSnapshot` on memory and PostgreSQL, and the exact memory/PostgreSQL `HostResourceURIsMatchPortableContract` subtest. The initial broad unit-test selector found no standalone graph URI/resource test; the separately selected conformance subtests did execute and pass.

Commands inside each diagnostic runner:

```sh
go test -race -count=1 -v -run 'URI|Resource' ./spec ./toolkit ./graph
go test -race -count=1 -v -run '^TestMemoryConformance$/^HostResourceURIsMatchPortableContract$' ./graph
go test -race -count=1 -v -run '^TestPostgresConformance$/^HostResourceURIsMatchPortableContract$' ./graph/store/postgres
```

Evidence: `.local/uri-go1.25.0-20261027T172839Z-332616/` and `.local/uri-go1.27.3-20261027T172840Z-332706/`, containing source copies, baseline revision, `overlay.sha256`, `verification.log`, PostgreSQL log and exit result. These targeted worktree-overlay checks establish the correction's focused behavior; they do not replace the full matrix on the next committed candidate. The shared assertion makes IP-literal/authority handling consistent across compilers while retaining existing opaque/custom-scheme parsing behavior; it is not a claim of a complete strict RFC parser.

The diagnostic baseline was `bbabbe6b10c217dfc46f95f6e2a1afe3b09fc0bb`; each log contains **35 test/subtest passes, 0 failures and 0 skips** across the focused commands above.

## Completed corrected-source matrix: 62c083f

The corrected implementation `62c083f422be2f25e538724179c9fa142bd17e91` completed both native clean-source verification lanes. The commands were launched on 2026-10-27; their durable results were inspected after the usage reset without rerunning either matrix. Exact commands:

```sh
bash scripts/verify-clean-environment.sh 62c083f422be2f25e538724179c9fa142bd17e91 1.25.0
bash scripts/verify-clean-environment.sh 62c083f422be2f25e538724179c9fa142bd17e91 1.27.3
```

The independently recomputed SHA-256 of each retained `source.tar` is `59abb2201ac460539ae793050627c7691052f29a9eb18a2ec0e545d650df24f9`. Tool image IDs remain `sha256:67b4151ef689e091606acc3a7fbbd63ef171aa02a5336d2eadb5317a9c473084` (Go 1.25.0) and `sha256:eb1f98d818d6bb6c02c19c7327675f84bc5383ea22b42e6e43e5fff86fe3a88f` (Go 1.27.3). Each used fresh source, absent module/build caches, a separate new PostgreSQL database, the pinned image/compiler inputs above, native Linux amd64, Python 3.12.3, GCC 13.3.0, PostgreSQL 17.11 (`Debian 17.11-1.pgdg13+2`), `CGO_ENABLED=1`, `GOTOOLCHAIN=local`, `CI=1`, and required PostgreSQL/Python.

Both lanes passed dependency download/verification, generator drift, vet, build/install and the complete race suite. Independently parsed `evidence/race-tests.jsonl` records **195 test/subtest passes, 0 failed tests, 0 skipped tests and 7 passed test-bearing packages in each lane**. The two helper packages have no standalone test files; their package-level skip events are not skipped tests. No data race was reported. This includes the corrected URI cases and the full memory/PostgreSQL conformance suites, closing the minimum-compiler failure recorded for `ba4769a` above.

Both lanes also completed the explicit stdio tests and Python consumer with and without the lost-response drill. Their separately executed `direct.json`, `mcp.json` and `python.json` reports are equal after JSON decoding: revision 14, 14 events, two completed work items, four reviewed evidence nodes, two reviewed resources, a met goal and graph-only resume. The minimum lane's final log line is `Documented direct, in-process MCP, and Python stdio reports agree`. The patched lane subsequently completed govulncheck v1.9.0: 0 affecting vulnerabilities, 0 findings in imported packages, and 1 finding in a required module whose vulnerable code is not called. Its retained log ends with that successful scan report. The minimum lane does not configure a vulnerability scan.

Final retained evidence directories (relative to the repository; ignored):

- `.local/clean-environment/20261027T173045Z-62c083f422be-go1.25.0-344721/` — complete minimum-compiler lane; original process handle `54818`.
- `.local/clean-environment/20261027T173044Z-62c083f422be-go1.27.3-344709/` — complete patched-compiler lane; original process handle `84590`.

Each contains source/archive hashes, environment and provisioning records, the complete verification/PostgreSQL logs and copied `evidence/` results; the patched lane additionally retains the vulnerability report. Follow-up inspection found no matching runner/database containers, private Docker networks or active verification process. The original process handles had expired and the helper did not persist its outer terminal `Exit:` line, so a directly recovered shell exit code is unavailable. Completion and successful checks are established by the complete race results, all smoke reports, final configured step output and completed cleanup; no missing terminal exit status is presented as an observed value. No new tests, provisioning or matrix restart were used for this confirmation.

The clean-source build/install, required database/Python execution, native race detector and declared minimum compiler gates are supported by this corrected-source matrix. Hosted GitHub Actions orchestration remains unexecuted. Final artifact inventory/checksum and integrator release approval are separate records.
