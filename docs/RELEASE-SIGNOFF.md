# Local source release sign-off

Date: 2026-10-27

## Approval and scope

The integrator accepts the scoped M1/M2 implementation as **GO** and approves local source delivery 0.2.0. Normative tool contract 0.2 and snapshot support set `{1}` are approved by [Decision 012](DECISIONS.md#012-contract-freeze-and-candidate-version). This is approval for a local source release; it does not authorize remote publication and does not claim hosted CI execution.

## Runtime evidence

The tested source is commit `62c083f422be2f25e538724179c9fa142bd17e91`. The clean environment commands for Go 1.25.0 and 1.27.3 each completed with **195 test/subtest passes, 0 failures, 0 skipped tests and 7 test-bearing packages passed**, using native CGO, a fresh PostgreSQL 17.11 instance and Python 3.12.3. Direct, in-process MCP and Python stdio reports agreed on 14 events, 8 nodes, 2 work items, 4 evidence nodes and 2 resources. The Go 1.27.3 vulnerability scan reported 0 vulnerabilities affecting code, 0 findings in imported packages and 1 finding in a required but uncalled module. Detailed commands and retained logs are listed in [CLEAN-ENVIRONMENT.md](CLEAN-ENVIRONMENT.md).

## Source inspection and final artifact identity

Preflight commit: `9da1e0b9e33aba4a2aef9fb79b9a85f67d76433d`. Root compared all 88 tracked files byte-for-byte against Git and confirmed the required source, schemas, migration, examples, license and documentation were present, with no prohibited local state, credentials or file patterns. The SHA-256 of that preflight source archive was `6698a1c3442a51c4555b881d2b607e3f63bb6a97ba796c34f3dd7cc912dfc9f0`. This is the preflight archive checksum, not a checksum for the final distributable.

The final documentation commit will be identified by local Git tag `v0.2.0`. Root will regenerate ZIP and TAR.GZ source archives from that commit, repeat the Git byte-equivalence and inventory checks, and produce a detached manifest and `SHA256SUMS` in `.local/releases/0.2.0/`. The manifest/checksum file remains outside the archives to avoid a self-referential checksum. Final artifact filenames, hashes and manifest contents will be recorded by root after generation; no final archive checksum is asserted in this sign-off.

Post-archive completion: tagv0.2.0 identifies7521239b9f109d2e39cf3aadaa5f8d5ae5b908d2. Both89-file source archives were generated, inspected against Git and checksummed; delivery is complete. Exact final hashes, sizes and scope are in [RELEASE-ARTIFACTS.md](RELEASE-ARTIFACTS.md) and the detached manifest/checksum files. This post-packaging attestation does not modify the immutable tagged archive.

The distribution is source-only: it contains project source and license/notice files, without Go or PostgreSQL runtimes/toolchains, built executables or dependency binaries. The included third-party dependency inventory is an engineering record, not legal review or a legal conclusion.

## Known scope limits

Ordinary reads transfer current nodes and graph edges in O(nodes + edges); mutations load graph history; snapshots are whole-graph; and `ListEligibleWork` is unpaged. These limits do not support a general throughput claim. Generic MCP clients do not get transparent exactly-once writes; clients reconcile ambiguous writes by reading graph state. Model-backed smoke testing is optional and was not part of this approval. M3 viewer work is excluded. GitHub Actions has not run, and nothing has been published.
