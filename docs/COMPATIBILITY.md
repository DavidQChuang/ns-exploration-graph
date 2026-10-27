# Compatibility

The local source release candidate is 0.2.0. Tool contract 0.2 is approved, and runtime verification passed for candidate commit `62c083f` on Go 1.25.0 and 1.27.3. Final source-archive inspection and root delivery sign-off remain pending; nothing has been published. This page records compatibility policy, not a general production-readiness claim or a guarantee for a published version.

- **Tool contract:** version 0.2 is `approved` (`spec/tool-contracts.json`). The schema and tool surface are fixed for this candidate.
- **Snapshot schema:** version 1 is the only supported version. Import of any other version fails with `invalid_argument`, with no partial import (no graph and no events are left behind). There is no repair or migration of snapshots.
- **SQL migration:** `graph/store/postgres/migrations/001_initial.sql` is the only migration currently present. No future upgrade path is promised or specified.
- **Error codes:** the stable set listed in `spec/SPEC.md`. Only `storage_unavailable` is automatically retryable.
- **MCP SDK:** `github.com/modelcontextprotocol/go-sdk` v1.8.0 (`go.mod`).
- **Pre-1.0 compatibility:** patch releases preserve the approved 0.2 public contracts. Intentional breaking changes require a recorded decision, a new minor contract/release version, migration or compatibility notes, and corresponding fixtures (Decision 012). No deprecation window is promised.
- **Distribution:** this local release is project source only. It does not package Go/PostgreSQL toolchains or runtimes, built executables, or dependency binaries. Dependency license inventories are engineering records, not legal review or a legal conclusion; inspect actual contents and applicable notices in the final archive.

The candidate matrix results and scope are recorded in `docs/CLEAN-ENVIRONMENT.md`; the final archive and delivery sign-off remain pending. Exercised Go and PostgreSQL versions are listed in `README.md` ("Versions") and `docs/POSTGRES-VERIFICATION.md`.
