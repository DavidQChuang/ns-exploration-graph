# Compatibility

The proposed source release is 0.2.0. Tool contract 0.2 is approved, but final candidate verification, archive inspection and production sign-off remain pending. Nothing has been published. This page records compatibility policy; it is not a claim of production readiness or a guarantee for a published version.

- **Tool contract:** version 0.2 is `approved` (`spec/tool-contracts.json`). The schema and tool surface are fixed for this candidate.
- **Snapshot schema:** version 1 is the only supported version. Import of any other version fails with `invalid_argument`, with no partial import (no graph and no events are left behind). There is no repair or migration of snapshots.
- **SQL migration:** `graph/store/postgres/migrations/001_initial.sql` is the only migration currently present. No future upgrade path is promised or specified.
- **Error codes:** the stable set listed in `spec/SPEC.md`. Only `storage_unavailable` is automatically retryable.
- **MCP SDK:** `github.com/modelcontextprotocol/go-sdk` v1.8.0 (`go.mod`).
- **Pre-1.0 compatibility:** patch releases preserve the approved 0.2 tool contract. A breaking change to the tool contract, snapshot schema, SQL schema or stable errors requires an explicit versioning decision recorded in `docs/DECISIONS.md` and release notes; a minor-version contract change requires a version bump. Whether a proposed breaking change fits a patch or minor release must be decided and documented before that release. No deprecation window is promised.
- **Distribution:** the proposed release contains project source only. It does not package Go/PostgreSQL toolchains or runtimes, built executables, or dependency binaries. Dependency license inventories are engineering records, not legal review or a legal conclusion; inspect actual contents and applicable notices for any future archive.

The clean baseline verification and its scope are recorded in `docs/CLEAN-ENVIRONMENT.md`; final candidate matrix runs remain pending. Versions of Go and PostgreSQL exercised for that baseline are in `README.md` ("Versions") and `docs/POSTGRES-VERIFICATION.md`.
