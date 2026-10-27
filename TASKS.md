# Implementation task ledger

Status: active. Scope: complete portable-tool release M1 + M2; optional viewer M3 deferred by the accepted plan.

- [x] Bootstrap dedicated local Git repository and import specification.
- [ ] G0: Freeze typed contracts, results, dependencies, fixtures, and decisions. Result schemas, format assertions, and generator drift checks are implemented and agent-verified; root rerun and the acceptance-flag decision record are still open.
- [x] G1: PostgreSQL vertical slice, durable retry, rollback, competing claims, restart. Root reran real DB tests successfully; evidence in docs/POSTGRES-VERIFICATION.md.
- [ ] G2: All semantic commands, freshness, acceptance invalidation, execution fencing, conformance.
- [ ] G3: Bounded projection, traversal/cursors, events, snapshot export/restore, benchmarks.
- [ ] G4: Eight tools, strict validation, role/context enforcement, scripted harness and prompt assets.
- [ ] G5: MCP adapter, protocol parity, clean setup, documentation, CI, release audit.
- [ ] Independent correctness/security review; resolve all material findings.
- [ ] Final requirements-to-evidence audit and clean committed delivery.

Current focus: core_perf is still on G3 bounded reads, restore tests, and benchmarks. toolkit_mcp is committed (e4d5568, 237d1a2) but not yet root-verified. Next: integrate core_perf, rerun the suite, record the acceptance-flag decision, then independent review and the release-checklist audit. Earlier: complete G2/G3 adversarial validation and restore, plus toolkit/MCP tests. Shared conformance passes on memory and real PostgreSQL, with explicit fault injection and concurrency tests. G0 result-schema completion and G4/G5 adapter/release checks remain open.
