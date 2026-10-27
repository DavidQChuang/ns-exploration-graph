# Action log

## 2026-10-27 — Bootstrap

- User authorized end-to-end implementation, tiered subagents, regular commits, logs, and usage-aware reset recovery.
- Created a local Git repository named ns-exploration-graph and copied the v0.2 design package into spec/.
- Activated the durable implementation goal. Initial usage check: 28% of five-hour allowance used, 20% weekly used.
- Go, PostgreSQL, and Docker are not on PATH. Provisioning and actual database execution are required gates.
- Repository publication is not requested; no remote has been created.
- Initial commit: 504bbef, specification and orchestration scaffold.
- Started tiered agents: core_lead (astra/high), storage_infra (sol/high), requirements_docs (luna/high).
- Created reset-recovery heartbeat `resume-ns-exploration-graph-implementation`, hourly at minute 7.
- Approved shared semantic service over detached aggregate transactions with normalized PostgreSQL persistence; ordinary reads must avoid unnecessary full event-payload history, and scaling is subject to measurements.
- Selected Apache-2.0 for independently implemented project code; standard license text and accurate attribution assigned to documentation lane.
- Documentation lane completed requirement IDs, release checklist, standard Apache license, and design-source notice; whitespace verification passed, no runtime claims.
- Infrastructure provisioned checksum-verified Go 1.27.3 under ignored .local/; PostgreSQL 17.11 portable runtime provisioning underway.
- Started toolkit_mcp (sol/high) for strict tool contracts, agent role bindings, MCP adapter, scripted examples, and integration documentation.
- Resolved creation/event numbering (starts at 1), acceptance result fields (typed reviewed node revisions plus closure edges/resources), read freshness labels, and dependency eligibility (done is terminal). Core lead records details in docs/DECISIONS.md; MCP protocol pinning remains assigned to integration lane.
- All three implementation agents terminated with account usage-limit errors. A subsequent authoritative usage check reported capacity available (2% five-hour used); resumed the same agents from existing state. No reset credit or purchase was requested by the orchestrator. Core/storage/toolkit files remain in progress and unverified.
- Root independently ran `go test ./graph ./graph/store/memory` with the portable toolchain: packages compile, currently no test files. Recording a foundation commit, not a passed semantic gate; core agent is now building the reusable behavioral suite.
- Root reran `go test -count=1 -v ./graph ./graph/store/memory`: real MemoryConformance suite passed (persistent slice/replay/fencing, concurrent claims/dependencies, failed writes/cursors). PostgreSQL agent reports real rollback/concurrency tests passing and is integrating the same reusable suite; root DB rerun is still pending.
- Core behavioral checkpoint committed as 3232a11.
- Root independently ran real PostgreSQL suite with GRAPH_REQUIRE_POSTGRES=1: PASS restart/lost response, five transactional rollback injection points, simultaneous claims, concurrent dependency cycle rejection, concurrent creation, and shared conformance. G1 marked complete. Portable PostgreSQL 17.11 runs on loopback port 55439; reproducible commands are in docs/POSTGRES-VERIFICATION.md.
- Usage recheck after G1: 51% five-hour used, 8% weekly used. Remaining agents continue bounded checkpoints; no reset credits consumed by orchestration.
- PostgreSQL checkpoint committed as b5d2f53.
- Root reran direct toolkit tests: PASS strict input/output validation, permissions and trusted worker binding; MCP server/CLI compile but protocol tests not yet present. Committing adapter foundation without claiming G4/G5 complete. Official MCP Go SDK pinned to v1.8.0 and raises module minimum to Go 1.25.
- Approved semantic clarification: a retracted observation revision need not duplicate prior resource evidence; active observation revisions still require evidence, and original references remain historical.

## 2026-10-27 — Handoff to new orchestrator

- Orchestration moved from the stopped Codex session to a Cursor agent (Claude). Prior Codex agents (core_lead/storage_infra/toolkit_mcp) are no longer running; reset heartbeat is not carried over. Usage-limit logic does not apply to this orchestrator. Toolchain runs from WSL by invoking the Windows `.local/go/bin/go.exe` with GOCACHE/GOMODCACHE via WSLENV; PostgreSQL 17.11 (PID 46012, port 55439) was still running and was reused.
- Inspected actual state: HEAD 79b71b4 plus uncommitted snapshot/adversarial/MCP-test/example work. Root ran `go build`, `go vet`, and `go test -count=1 ./...` with GRAPH_REQUIRE_POSTGRES=1: all packages PASS, including real PostgreSQL, after the pgx v5.9.2 / x/text v0.41.0 upgrade.
- Root reran govulncheck v1.9.0: no reachable vulnerabilities (0 affected; 1 in a required module not called). This closes the pending post-upgrade rescan.
- Committed existing work in three coherent commits: fc675c2 (dependency upgrade, benchmarks), a54b4d3 (snapshot export/restore, adversarial tests, result contracts), e6d3a6d (MCP protocol tests, scripted harness example). Still not claimed: G0 result schemas, G3 performance sign-off, G4/G5 release gates.
- Started tiered agents on disjoint paths: core_perf (claude-opus-5-5-medium; graph/**, bounded reads, G3 performance and restore tests) and toolkit_mcp (claude-sonnet-5-5-high; toolkit/adapters/cmd/examples, result schemas, stdio + Python second consumer, README/INTEGRATION, CI). Root remains coordinator and Git committer.

## 2026-10-27 — toolkit_mcp handoff

- toolkit_mcp (agent 5f884d15) finished G0 result-schema checks, G4 role/error/prompt tests, and G5 stdio plus Python consumer. Root reviewed the diff and committed only that lane: e4d5568 (schemas, roles, errors, prompts) and 237d1a2 (stdio MCP, Python consumer, README, docs/INTEGRATION.md, CI). graph/** and docs/DECISIONS.md were left unstaged because core_perf is still editing them.
- Root accepts the new `-acceptance human|validator|agent` flag: launching graph-mcp with it is the host grant of graph_accept_goal for that process, off by default, with no per-request policy. Record this in docs/DECISIONS.md after core_perf stops writing that file.
- Agent-reported verification (not yet rerun by root, because core_perf is using the shared Go cache): go vet ./... and go test -count=1 ./... passed with PostgreSQL required, including the in-progress graph edits; go test -count=3 on toolkit, cmd, examples, adapters and spec passed; python toolkit/generate_contracts.py --check matched; govulncheck v1.9.0 found no reachable vulnerabilities; the Python consumer matched the Go report under Linux Python 3.10.12 against the Windows binaries. Deliberate mutations of format assertions and the storage_unavailable sanitizer failed the new tests. Race detector was not run (no CGO). GitHub Actions has not run.
- Open from this lane, not fixed here: postgres.storageError still puts raw driver text into graph.Error (the toolkit now hides it from models; host callers and logs still see it). spec JSON files are CRLF in Git; a *.json eol=lf attribute is deferred until both lanes are idle. No model-backed smoke test. validate_schema.py stays out of CI because it needs the jsonschema package and rewrites the example snapshot.
- Release-checklist boxes stay unchecked until root reruns the suite on a quiet tree and records evidence.
