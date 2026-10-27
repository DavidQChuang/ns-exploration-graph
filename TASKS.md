# Implementation task ledger

Status: active. Scope: complete portable-tool release M1 + M2; optional viewer M3 deferred by the accepted plan.

- [x] Bootstrap dedicated local Git repository and import specification.
- [x] G0: Freeze typed contracts, results, dependencies, fixtures, and decisions. Decisions 001–009, result schemas, and the generator drift check are recorded. Public changes after this point need a new decision.
- [x] G1: PostgreSQL vertical slice, durable retry, rollback, competing claims, restart. Root reran real DB tests successfully; evidence in docs/POSTGRES-VERIFICATION.md.
- [x] G2: All semantic commands, freshness, acceptance invalidation, execution fencing, conformance. Root reran the shared suite on memory and PostgreSQL on 2026-10-27.
- [x] G3: Bounded projection, traversal/cursors, events, snapshot export/restore, benchmarks. Overview is bounded; traversal and GetNode stay O(nodes+edges), measured and documented. Commit 7fa9d4f.
- [x] G4: Eight tools, strict validation, role/context enforcement, scripted harness and prompt assets. Passed in the root suite. No model-backed smoke test (optional in the plan).
- [ ] G5: MCP adapter, protocol parity, clean setup, documentation, CI, release audit.
- [x] Independent correctness/security review; resolve all material findings. Reviewer 35e7e671 reported two medium and three low findings; review_fixes 1941b273 resolved them; root reran the suite on 2026-10-27.
- [ ] Final requirements-to-evidence audit and clean committed delivery.

Current focus: close the tested gaps the audit left open (unrelated goal, criteria invalidation, claim/dependency race, cyclic snapshot, compatibility note, draft release notes). Do not mark G5 or the release ready. GitHub Actions, the race detector, and a clean install stay open. Earlier: complete G2/G3 adversarial validation and restore, plus toolkit/MCP tests. Shared conformance passes on memory and real PostgreSQL, with explicit fault injection and concurrency tests. G0 result-schema completion and G4/G5 adapter/release checks remain open.
