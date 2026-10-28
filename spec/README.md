# Contract index

Normative contract v0.2 is approved by [Decision 012](../docs/DECISIONS.md#012-contract-freeze-and-candidate-version). The implementation is present; tested source and exact delivered archive identities are recorded in [clean-environment verification](../docs/CLEAN-ENVIRONMENT.md) and [release artifacts](../docs/RELEASE-ARTIFACTS.md).

- [SPEC.md](SPEC.md): core semantics, commands, revisions, evidence freshness, execution fencing, persistence, reads and snapshot restore.
- [TOOLS.md](TOOLS.md): eight agent tools, trusted host bindings, roles, prompts, limits and retry behavior.
- [tool-contracts.json](tool-contracts.json): approved model input schemas, role presets and contract metadata.
- [snapshot.schema.json](snapshot.schema.json): structural snapshot schema; version 1 is the only supported version. Snapshot numbering is independent of tool contract 0.2.
- [Initial PostgreSQL migration](../graph/store/postgres/migrations/001_initial.sql): canonical seven-table DDL embedded by the PostgreSQL store.
- [example.snapshot.json](example.snapshot.json): generated example validated by the contract checks.
- [validate_schema.py](validate_schema.py): Python/jsonschema structural checks; run `python spec/validate_schema.py` from the repository root. It regenerates the example snapshot.

Schemas cover structural constraints; SPEC and TOOLS govern semantic constraints that schemas cannot express. Go contract/format tests and `python toolkit/generate_contracts.py --check` check runtime contract parity. The full release verification payload is [scripts/verify-release.sh](../scripts/verify-release.sh); recorded real PostgreSQL, concurrency, authorization and transport evidence is in [the release checklist](../docs/RELEASE-CHECKLIST.md).

For usage, start with [the eight guides](../docs/README.md). For compatibility decisions and unique verification records, use [the maintainer index](../docs/maintainers/README.md).
