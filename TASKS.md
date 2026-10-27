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

Current focus: G0 contracts and local toolchain provisioning. No runtime behavior has been verified yet.
