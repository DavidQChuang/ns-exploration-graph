# Release notes — source candidate 0.2.0

Candidate metadata for the proposed source-only 0.2.0 release, dated 2026-10-27. Tool contract 0.2 has been approved and is marked `approved` in `spec/tool-contracts.json`. This does not certify the final candidate: required lifecycle and URI corrections, the two-stage fixture, clean-matrix reruns, final archive inspection and production sign-off remain pending. Nothing has been published, and this document is not a production-readiness declaration.

The intended distribution is project source only. It includes no Go or PostgreSQL toolchain/runtime, built executable, or dependency binary. No future SQL upgrade path is promised; migration 001 is the only migration currently present. Snapshot schema version 1 is the only supported version. See `docs/COMPATIBILITY.md` for the pre-1.0 versioning policy.

## Candidate scope

- Standalone Go exploration-graph library with shared semantics for in-memory and PostgreSQL stores.
- Eight schema-validated tools, available in process or through the stdio MCP adapter.
- Versioned snapshots, prompt assets and deterministic direct/MCP/Python examples.
- Explicit trusted-host boundaries for authorization, evidence resolution, work assignment, acceptance policy and durable invocation identity.

The library does not run models, schedule workers, resolve or authorize external evidence, authenticate remote callers, provide a viewer, or promise exactly-once writes for generic MCP clients. The stock stdio host assigns a fresh invocation identity to each call; after an ambiguous write, clients must reconcile by reading the graph.

## Baseline verification evidence

The clean verification payload passed for committed baseline `01a1bf1cf69b` on 2026-10-27 with Go 1.25.0 and Go 1.27.3. Each independent source-archive run exited successfully and recorded 142 test/subtest passes, zero failures and zero skipped tests across seven test-bearing packages. The runs used empty build/module caches, native Linux amd64 with CGO enabled, GCC 13.3.0, Python 3.12.3 and isolated PostgreSQL 17.11. Each included dependency verification, contract generation drift check, `go vet ./...`, `go build ./...`, tool installation, `go test -race -json -count=1 ./...`, explicit stdio/Python checks and direct/MCP/Python example parity. The Go 1.27.3 lane also ran govulncheck v1.9.0: no vulnerability affecting called code, no findings in imported packages and one finding in a required but uncalled module.

The two-stage fixture lane is underway. Its intended workflow resumes a new session from graph state, proposes fix-verification work and records a passing result. This candidate still needs that fixture and equivalence checks in the direct, MCP and Python examples. The remaining lifecycle/error and URI validation gates also need correction and runtime coverage. Once those changes are integrated, rerun both clean compiler lanes; baseline results do not verify later source changes.

## Remaining release gates

- Complete runtime coverage for the remaining lifecycle transitions and all stable error codes; correct host URI validation to match snapshot validation.
- Complete the two-stage fixture and compare normalized direct, MCP and Python outcomes.
- Rerun the clean Go 1.25.0 and 1.27.3 matrices against final candidate source.
- Inspect the actual source archive, its tracked-file inventory, notices and checksum; record the archive location and integrator sign-off.

GitHub Actions has not run: there is no remote and no hosted workflow execution. Local verification exercised the checked-in payload, not GitHub's checkout/setup/upload orchestration. The source dependency/license inventory in `docs/DEPENDENCIES.md` is an engineering record, not legal review or a legal conclusion. The final archive has not been inspected. No package, binary or other release has been published.

For detailed results and gates, see `docs/CLEAN-ENVIRONMENT.md`, `docs/RELEASE-CHECKLIST.md`, `docs/REQUIREMENTS.md` and `docs/POSTGRES-VERIFICATION.md`.
