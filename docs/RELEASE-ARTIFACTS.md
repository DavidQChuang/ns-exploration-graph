# Verified local source release 0.2.0

Integrator delivery approval: 2026-10-27. Scope: the complete M1 core library and M2 portable-tool release. Optional M3 viewer and remote publication are outside this delivery.

Local tag `v0.2.0` identifies source commit `7521239b9f109d2e39cf3aadaa5f8d5ae5b908d2`. Its runtime source, schemas, prompt assets, dependencies, examples and CI payload are byte-identical to clean-tested commit `62c083f422be2f25e538724179c9fa142bd17e91`; only documentation differs. Each native clean Go1.25.0/Go1.27.3 lane recorded195passing test/subtests, zero failures/skipped tests, required PostgreSQL17.11/Python3.12 and race detection. See [CLEAN-ENVIRONMENT.md](CLEAN-ENVIRONMENT.md).

## Preflight inspection

Before final packaging, root inspected commit `9da1e0b9e33aba4a2aef9fb79b9a85f67d76433d` and compared all 88 tracked files byte-for-byte with Git. Its preflight source archive SHA-256 was `6698a1c3442a51c4555b881d2b607e3f63bb6a97ba796c34f3dd7cc912dfc9f0`; this identifies preflight input, not a final distributable. Required source, schemas, migration, examples, license and documentation were present, with no prohibited local state, credentials or file patterns. Final tagged archive inspection is recorded below.

## Delivered artifacts

Files are under `.local/releases/0.2.0/` in this repository:

- `ns-exploration-graph-0.2.0-source.zip` — 279067bytes; SHA256 `8a937c81125c88ea1ec2c8fbeba1fdcea586dd7871dd6b5001fc0a25ac2f67c8`.
- `ns-exploration-graph-0.2.0-source.tar.gz` — 224125bytes; SHA256 `82a63603642da3b9bcb54e73727bb4f15120c8998454edcc9ebfb50689a33658`.
- `release-manifest.json` — exact source/tested commit identities, verification scope, artifact hashes and per-file inventory.
- `SHA256SUMS` — detached checksums for both archives and the manifest.

Both archives contain the same89regular source files. Root compared every archived byte with the tagged Git blob, verified the required code/schema/migration/example/license/documentation files, and checked for excluded local runtimes, database data/logs, generated binaries and high-confidence credential patterns. No findings occurred. The manifest and checksums intentionally remain outside the archives: an archive cannot contain its own final checksum. The archived checklist captures the pre-packaging audit; this post-archive record and the detached manifest close delivery inspection/sign-off without changing the tagged source.

The archives, raw test evidence, downloaded runtimes and caches are ignored local artifacts, not Git-tracked binaries. GitHub Actions is configured and shares the verified local CI payload, but hosted orchestration has not run. Nothing was published to a remote repository or package registry. Engineering approval covers the documented host/security and modest-graph scaling boundaries; it is not a legal review or an unlimited-throughput claim.
