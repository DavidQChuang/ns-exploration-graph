# Compatibility

Nothing has been released. This page records what the tree currently promises; it is not a stability guarantee for any published version.

- **Tool contract:** version 0.2 is still `proposed` (`spec/tool-contracts.json`). No approval is recorded.
- **Snapshot schema:** version 1 only. Import of any other version fails with `invalid_argument`, with no partial import (no graph and no events are left behind). There is no repair or migration of snapshots.
- **SQL migration:** `graph/store/postgres/migrations/001_initial.sql` is the only migration. There is no upgrade path from it yet.
- **Error codes:** the stable set listed in `spec/SPEC.md`. Only `storage_unavailable` is automatically retryable.
- **MCP SDK:** `github.com/modelcontextprotocol/go-sdk` v1.8.0 (`go.mod`).
- **Breaking changes:** a change to the tool contract, snapshot schema, SQL schema or error codes needs a recorded decision in `docs/DECISIONS.md` and a contract version bump.
- **Deprecation:** there is no deprecation window yet, because nothing has been released.

Versions of Go and PostgreSQL that were actually exercised are in `README.md` ("Versions") and `docs/POSTGRES-VERIFICATION.md`.
