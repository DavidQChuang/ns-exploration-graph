# Implementation task ledger

Status: active. Scope: complete portable-tool release M1 + M2; optional viewer M3 deferred by the accepted plan.

- [x] Bootstrap dedicated local Git repository and import specification.
- [x] G0: Freeze typed contracts, results, dependencies, fixtures, and decisions. Decisions 001–009, result schemas, and the generator drift check are recorded. Public changes after this point need a new decision.
- [x] G1: PostgreSQL vertical slice, durable retry, rollback, competing claims, restart. Root reran real DB tests successfully; evidence in docs/POSTGRES-VERIFICATION.md.
- [x] G2: All semantic commands, freshness, acceptance invalidation, execution fencing, conformance. Root reran the shared suite on memory and PostgreSQL on 2026-10-27.
- [x] G3: Bounded projection, traversal/cursors, events, snapshot export/restore, benchmarks. Overview is bounded; traversal and GetNode stay O(nodes+edges), measured and documented. Commit 7fa9d4f.
- [x] G4: Eight tools, strict validation, role/context enforcement, scripted harness and prompt assets. Passed in the root suite. No model-backed smoke test (optional in the plan).
- [ ] G5: MCP adapter, protocol parity, clean setup, documentation, CI, release audit. Native clean-source/race/CI-payload verification passed at 01a1bf1 on Go1.25.0 and1.27.3. Final audited fixes, literal two-stage resume fixture, final-candidate rerun, contract approval and archive/sign-off remain open. Hosted GitHub Actions has not executed; no remote publication is authorized.
- [x] Independent correctness/security review; resolve all material findings. Reviewer 35e7e671 reported two medium and three low findings; review_fixes 1941b273 resolved them; root reran the suite on 2026-10-27.
- [ ] Final requirements-to-evidence audit and clean committed delivery.

Current focus: exact-source native clean/race verification of ba4769a and final artifact/sign-off. Host lifecycle coverage, URI parity, typed UUID snapshot portability, the literal two-stage fixture and approved contract0.2 metadata are implemented and pass targeted/full local checks. Final auditor found no remaining behavior gap and gave conditional GO pending the exact-source matrix and archive inspection. Source release0.2.0 is not complete until those checks finish.
