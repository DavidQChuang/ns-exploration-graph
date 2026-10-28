# Release notes — source release 0.2.0

Approved local source-only release 0.2.0, dated 2026-10-27. Tool contract 0.2 is approved by Decision 012; snapshot schema version 1 is the only supported version. Corrected source `62c083f422be2f25e538724179c9fa142bd17e91` passed the native clean runtime matrix on both declared compilers. The source delivery was approved after a preflight inventory at `9da1e0b9e33aba4a2aef9fb79b9a85f67d76433d`; root will regenerate and recheck the final archive and detached checksums from the final documentation commit. Its local release tag will be `v0.2.0`. Nothing has been published, and GitHub Actions has not run.

## Included

- Standalone Go exploration-graph library with shared semantics for in-memory and PostgreSQL stores.
- Eight schema-validated tools available in process or through the stdio MCP adapter.
- Versioned snapshots, prompt assets and deterministic direct, in-process MCP and Python stdio examples.
- Trusted-host boundaries for authorization, evidence resolution, work assignment, acceptance policy and durable invocation identity.

The distribution is project source only. It includes no Go or PostgreSQL toolchain/runtime, built executable, or dependency binary. Migration `001_initial.sql` is the only current SQL migration; no future SQL upgrade path is promised. The source includes the project license and notice. The dependency license inventory is an engineering record, not legal review or a legal conclusion.

## Runtime verification

The corrected candidate source passed both clean environment commands on 2026-10-27:

```sh
bash scripts/verify-clean-environment.sh 62c083f422be2f25e538724179c9fa142bd17e91 1.25.0
bash scripts/verify-clean-environment.sh 62c083f422be2f25e538724179c9fa142bd17e91 1.27.3
```

Both lanes used fresh source copies and test environments, native Linux amd64 with `CGO_ENABLED=1`, required PostgreSQL 17.11 and Python 3.12.3, and completed the full race-enabled release payload. Each recorded **195 test/subtest passes, 0 failures and 0 skipped tests across 7 test-bearing packages**. The direct, in-process MCP and Python stdio reports agreed on 14 events, 8 nodes, 2 work items, 4 evidence nodes and 2 resources. In the Go 1.27.3 lane, govulncheck v1.9.0 found 0 vulnerabilities affecting code, 0 findings in imported packages and 1 finding in a required module whose vulnerable code is not called. Full environment evidence, including earlier candidate failure and correction, is recorded in `docs/CLEAN-ENVIRONMENT.md`.

## Source preflight and final artifacts

At preflight commit `9da1e0b9e33aba4a2aef9fb79b9a85f67d76433d`, all 88 tracked files were compared byte-for-byte with Git. The inventory included required source, schemas, migration, examples, license and documentation and contained no prohibited local state, credentials or prohibited file patterns. Preflight source archive SHA-256: `6698a1c3442a51c4555b881d2b607e3f63bb6a97ba796c34f3dd7cc912dfc9f0`. This hash identifies the preflight archive only, not a final distributable.

After this final documentation commit, root will tag the source locally as `v0.2.0`, regenerate and reassert the source inventory, then create ZIP and TAR.GZ archives and a detached manifest/checksum file under `.local/releases/0.2.0/`. The detached checksums file is kept outside the archives, so it does not contain a self-referential archive checksum. Final artifact hashes and manifest contents will be recorded by root when generated; none are asserted here.

## Scope and limits

The graph stores evidence references but does not fetch, authorize or interpret their targets. Hosts own authentication, scheduling, worker leases and external effects. The stock stdio host assigns a new invocation identity per call; after an ambiguous write, clients must read the graph before deciding whether to retry. Generic MCP clients do not receive transparent exactly-once writes.

Ordinary reads transfer current nodes and graph edges in O(nodes + edges); mutations load graph history. Snapshot operations are whole-graph, and `ListEligibleWork` is unpaged. These are documented scale limits, not throughput promises. Model-backed smoke testing is optional and was not part of the release gate. The optional M3 viewer is outside this release.

Local verification exercised the checked-in payload, not GitHub's hosted checkout/setup/upload orchestration. Source archives were generated and inspected locally; no package, binary or other release has been published remotely. For integrator approval and exact delivered archive identities, see [RELEASE-ARTIFACTS.md](RELEASE-ARTIFACTS.md).
