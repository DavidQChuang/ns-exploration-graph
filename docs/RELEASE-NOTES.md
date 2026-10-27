# Release notes — source release candidate 0.2.0

Local source-only release candidate 0.2.0, dated 2026-10-27. Tool contract 0.2 is approved by Decision 012; snapshot schema version 1 is the only supported version. Candidate commit `62c083f` passed the clean runtime matrix on both declared compilers. Final source-archive inspection and root delivery sign-off remain packaging steps. Nothing has been published, and this note does not claim hosted CI execution or a published release.

## Included

- Standalone Go exploration-graph library with shared semantics for in-memory and PostgreSQL stores.
- Eight schema-validated tools available in process or through the stdio MCP adapter.
- Versioned snapshots, prompt assets and deterministic direct, in-process MCP and Python stdio examples.
- Trusted-host boundaries for authorization, evidence resolution, work assignment, acceptance policy and durable invocation identity.

The source distribution contains no Go or PostgreSQL toolchain/runtime, built executable, or dependency binary. Migration `001_initial.sql` is the only current SQL migration; no future SQL upgrade path is promised. Contract compatibility policy is in `docs/COMPATIBILITY.md`.

## Runtime verification

The corrected candidate source `62c083f` passed both clean environment commands on 2026-10-27:

```sh
bash scripts/verify-clean-environment.sh 62c083f 1.25.0
bash scripts/verify-clean-environment.sh 62c083f 1.27.3
```

Both lanes used fresh source copies and test environments, native Linux amd64 with `CGO_ENABLED=1`, required PostgreSQL 17.11 and Python 3.12.3, and completed the full race-enabled release payload. Each recorded **195 test/subtest passes, 0 failures and 0 skipped tests across 7 test-bearing packages**. The direct, in-process MCP and Python stdio reports agreed on 14 events, 8 nodes, 2 work items, 4 evidence nodes and 2 resources. In the Go 1.27.3 lane, govulncheck v1.9.0 found 0 vulnerabilities affecting code, 0 findings in imported packages and 1 finding in a required module whose vulnerable code is not called. The exact environment evidence is recorded in `docs/CLEAN-ENVIRONMENT.md`.

## Scope and limits

The graph stores evidence references but does not fetch, authorize or interpret their targets. Hosts own authentication, scheduling, worker leases and external effects. The stock stdio host assigns a new invocation identity per call; after an ambiguous write, clients must read the graph before deciding whether to retry. Generic MCP clients do not receive transparent exactly-once writes.

Ordinary reads transfer current nodes and graph edges in O(nodes + edges); mutations load graph history. Snapshot operations are whole-graph, and `ListEligibleWork` is unpaged. These are documented scale limits, not throughput promises. Model-backed smoke testing is optional and was not part of the release gate. The optional M3 viewer is outside this release.

GitHub Actions has not run. Local runs exercised the checked-in verification payload, not GitHub's hosted checkout/setup/upload orchestration. The final source archive has not yet been inspected and no archive checksum or location is claimed. No package, binary or other release has been published. Dependency license records in `docs/DEPENDENCIES.md` are an engineering inventory, not legal review or a legal conclusion.

For implementation and verification details, see `docs/CLEAN-ENVIRONMENT.md`, `docs/POSTGRES-VERIFICATION.md`, `docs/COMPATIBILITY.md` and `docs/RELEASE-CHECKLIST.md`.
