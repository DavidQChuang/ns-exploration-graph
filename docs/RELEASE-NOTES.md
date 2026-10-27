# Release notes (draft, unreleased)

Draft for the current unreleased tree. No version is assigned, nothing is published, and this is not a release declaration. See `docs/COMPATIBILITY.md` for what is and is not promised.

## What the local suite covers

The shared conformance suite (`graph/conformance`) runs against both the memory store (`TestMemoryConformance`) and PostgreSQL 17.11 (`TestPostgresConformance`). It covers, among other things:

- command replay, idempotency conflicts, fencing of stale executions, and cursor staleness;
- one winner for concurrent claims, and no cycle from opposite concurrent dependency additions;
- a claim racing a dependency edit on the same work: running work never has an undone prerequisite, and an existing dependency means the claim failed and the work is still open (25 attempts per store per run);
- dependency edits on running work return `invalid_transition`;
- acceptance freshness: revising evidence invalidates only goals whose accepted closure includes it; revising a met goal's criteria is a single revision to `needs_review` with a stale acceptance;
- snapshot export/restore round trip, rejection of unknown versions, running work, inconsistent acceptance closures and relation cycles (`supports`, `depends_on`), each leaving no graph and no events;
- bounded projection, pagination, event feed resume, and scoped reads matching complete-aggregate reads.

`graph/store/postgres` adds PostgreSQL-only tests: injected rollback at five checkpoints, restart with lost response, simultaneous claims, concurrent dependencies and concurrent creation. Fault injection exists for PostgreSQL only.

The memory store serializes updates under one mutex, so the claim/dependency race test there exercises ordering through the store lock, not a database. Only the PostgreSQL run has been observed to produce both outcomes (claim first and dependency first).

Last run of the two core packages on this tree: `go test -count=1 ./graph ./graph/store/postgres` against PostgreSQL 17.11 on `127.0.0.1:55439` with `GRAPH_REQUIRE_POSTGRES=1`: both packages `ok`; verbose run: 59 `--- PASS`, 0 `--- FAIL`, 0 `--- SKIP`. Results for the other packages are in `docs/RELEASE-CHECKLIST.md`.

## Limits that remain

- No GitHub Actions run. `.github/workflows/test.yml` exists but there is no remote, so it has never executed.
- No race detector run (`CGO_ENABLED=0`, no C compiler). The concurrency tests above are not a substitute.
- No clean-machine install. All runs used the developer's `.local/` Go and PostgreSQL. The module minimum Go 1.25.0 has not been built.
- Traversal, `GetNode` and mutations are O(nodes + edges); `ListEligibleWork` is unbounded. No production throughput claim is made (`docs/POSTGRES-VERIFICATION.md`, "Current scaling boundary"; `docs/DECISIONS.md` 004).
- Stock `graph-mcp` has no durable invocation identity. It gives each call a fresh identity and reports ambiguous writes after a lost response; clients must read the graph to reconcile. There is no exactly-once claim over MCP.
- No model-backed smoke test.
- No M3 viewer.
- Tool contract 0.2 is still `proposed`; no approval is recorded.
- Also not provided: automatic resource authorization or resolution, external-effect fencing, ARTEX migration, a SQL upgrade path beyond `001_initial.sql`, a dependency license scan, or release artifacts and checksums.

## Where to look next

- `docs/RELEASE-CHECKLIST.md`: per-item verification status and evidence record.
- `docs/POSTGRES-VERIFICATION.md`: PostgreSQL runtime, test commands and scaling boundary.
- `docs/COMPATIBILITY.md`: compatibility and versioning statements.
