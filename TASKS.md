# Implementation task ledger

Status: active. Scope: complete portable-tool release M1 + M2; optional viewer M3 deferred by the accepted plan.

- [x] Bootstrap dedicated local Git repository and import specification.
- [ ] G0: Freeze typed contracts, results, dependencies, fixtures, and decisions.
- [x] G1: PostgreSQL vertical slice, durable retry, rollback, competing claims, restart. Root reran real DB tests successfully; evidence in docs/POSTGRES-VERIFICATION.md.
- [ ] G2: All semantic commands, freshness, acceptance invalidation, execution fencing, conformance.
- [ ] G3: Bounded projection, traversal/cursors, events, snapshot export/restore, benchmarks.
- [ ] G4: Eight tools, strict validation, role/context enforcement, scripted harness and prompt assets.
- [ ] G5: MCP adapter, protocol parity, clean setup, documentation, CI, release audit.
- [ ] Independent correctness/security review; resolve all material findings.
- [ ] Final requirements-to-evidence audit and clean committed delivery.

Current focus (2026-10-27 handoff): G3 bounded-read performance and restore verification (core_perf); result schemas, G4/G5 conformance, stdio + Python second consumer, docs and CI (toolkit_mcp). Then independent review and release-checklist audit. Earlier: complete G2/G3 adversarial validation and restore, plus toolkit/MCP tests. Shared conformance passes on memory and real PostgreSQL, with explicit fault injection and concurrency tests. G0 result-schema completion and G4/G5 adapter/release checks remain open.
