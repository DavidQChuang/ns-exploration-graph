# Implementation task ledger

Status: complete. Delivered local source release0.2.0 for the complete portable-tool scope M1+M2; optional viewer M3 remains outside this release.

- [x] Bootstrap dedicated local Git repository and import specification.
- [x] G0: Freeze typed contracts, results, dependencies, fixtures, and decisions. Decisions 001–009, result schemas, and the generator drift check are recorded. Public changes after this point need a new decision.
- [x] G1: PostgreSQL vertical slice, durable retry, rollback, competing claims, restart. Root reran real DB tests successfully; evidence in docs/POSTGRES-VERIFICATION.md.
- [x] G2: All semantic commands, freshness, acceptance invalidation, execution fencing, conformance. Root reran the shared suite on memory and PostgreSQL on 2026-10-27.
- [x] G3: Bounded projection, traversal/cursors, events, snapshot export/restore, benchmarks. Overview is bounded; traversal and GetNode stay O(nodes+edges), measured and documented. Commit 7fa9d4f.
- [x] G4: Eight tools, strict validation, role/context enforcement, scripted harness and prompt assets. Passed in the root suite. No model-backed smoke test (optional in the plan).
- [x] G5: MCP adapter/parity, clean setup, documentation, CI payload and release audit. Corrected62c083f native clean/race matrices pass on Go1.25.0/Go1.27.3:195passes each,zero failures/skipped tests. Approved0.2.0 source archives, inventory/checksums and root sign-off are recorded in docs/RELEASE-ARTIFACTS.md. Hosted GitHub orchestration was not executed; no remote publication was required or performed.
- [x] Independent correctness/security review; resolve all material findings. Reviewer 35e7e671 reported two medium and three low findings; review_fixes 1941b273 resolved them; root reran the suite on 2026-10-27.
- [x] Final requirements-to-evidence audit and clean committed delivery. Local tagv0.2.0/source7521239 identifies the immutable89-file source delivery; repository main retains the post-archive attestation.

Current state: complete; no implementation or verification jobs remain. ZIP/TAR.GZ, detached per-file manifest and SHA256SUMS are under .local/releases/0.2.0/. Runtime/contract/prompt/CI content is unchanged from tested62c083f; source-tag differences are documentation only. The durable goal is complete and its recovery heartbeat was deleted. Further work requires a new user request.

## Post-release documentation refresh

- [x] Inspect the requested zvec-grep reference and present an outline before writing.
- [x] Add the documentation index and eight task-oriented numbered guides with shared navigation.
- [x] Shorten the project README; route historical/specification/audit records through a maintainer index without moving them.
- [x] Preserve legacy INTEGRATION.md links and documentation assertions.
- [x] Check204local links/anchors, navigation consistency and balanced fences; execute the complete memory quickstart inside the checkout and from a separate consumer module.
- [x] Compile the PostgreSQL replacement, validate eight tool inputs plus two result examples, run existing documentation/example assertions and verify schema-generator equality.
- [x] Commit the documentation-only refresh on main. Immutable v0.2.0 tag/archives and runtime source are unchanged.
