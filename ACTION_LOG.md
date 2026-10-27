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
