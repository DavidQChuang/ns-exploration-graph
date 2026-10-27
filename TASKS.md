# Implementation task ledger

Status: active. Scope: complete portable-tool release M1 + M2; optional viewer M3 deferred by the accepted plan.

- [x] Bootstrap dedicated local Git repository and import specification.
- [ ] G0: Freeze typed contracts, results, dependencies, fixtures, and decisions.
- [ ] G1: PostgreSQL vertical slice, durable retry, rollback, competing claims, restart.
- [ ] G2: All semantic commands, freshness, acceptance invalidation, execution fencing, conformance.
- [ ] G3: Bounded projection, traversal/cursors, events, snapshot export/restore, benchmarks.
- [ ] G4: Eight tools, strict validation, role/context enforcement, scripted harness and prompt assets.
- [ ] G5: MCP adapter, protocol parity, clean setup, documentation, CI, release audit.
- [ ] Independent correctness/security review; resolve all material findings.
- [ ] Final requirements-to-evidence audit and clean committed delivery.

Current focus: G1/G2 behavioral tests, PostgreSQL runtime verification, and toolkit integration. Core command/read/projection packages compile with Go 1.27.3; no semantic test gate has passed yet. Toolchain and isolated PostgreSQL files are provisioned locally; adapter verification remains pending.
