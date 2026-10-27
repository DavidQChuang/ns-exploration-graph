# Exploration graph tool design package

Normative contract v0.2, approved by [DECISION 012](../docs/DECISIONS.md#012-contract-freeze-and-candidate-version) · October 27, 2026. Corrected source `62c083f422be2f25e538724179c9fa142bd17e91` passed the Go 1.25.0 and 1.27.3 runtime matrix; local source delivery was approved after preflight inspection at `9da1e0b9e33aba4a2aef9fb79b9a85f67d76433d`. See [CLEAN-ENVIRONMENT.md](../docs/CLEAN-ENVIRONMENT.md) and [RELEASE-SIGNOFF.md](../docs/RELEASE-SIGNOFF.md).

Start with [the implementation specification](SPEC.md), then [the agent integration contract](TOOLS.md), then [the orchestration game plan](DEVELOPMENT-PLAN.md).

- [SPEC.md](SPEC.md): boundaries, graph semantics, command invariants, revisions, execution fencing, persistence, and restore.
- [TOOLS.md](TOOLS.md): eight agent-facing tools, trusted host bindings, role presets, prompt fragments, retry behavior, and the host loop.
- [DEVELOPMENT-PLAN.md](DEVELOPMENT-PLAN.md): phased work packets, ownership, dependencies, merge/review rhythm, and acceptance gates.
- [tool-contracts.json](tool-contracts.json): draft machine-readable model input schemas and capability presets.
- [snapshot.schema.json](snapshot.schema.json): draft structural snapshot schema, schema_version 1. Document/tool contract version 0.2 is independent of this unreleased snapshot format number.
- [001_initial.sql](001_initial.sql): proposed initial PostgreSQL schema; not executed against PostgreSQL.
- [example.snapshot.json](example.snapshot.json): illustrative parser investigation with work, evidence, and goal acceptance.
- [validate_schema.py](validate_schema.py): runnable contract checks; requires Python and the jsonschema package. Run `python validate_schema.py` from this directory. It regenerates the example snapshot.

## What has been verified

The snapshot schema validates its example and rejects 12 malformed variants. All eight tool input schemas validate examples, reject 49 malformed inputs, and pass role-manifest consistency checks. These are structural checks only. The SQL migration, semantic service, runtime authorization, concurrency guarantees, MCP transport, and UI have not been implemented or tested. Strict tool-result schemas are an explicit G0 deliverable.

This package describes the pre-implementation design baseline. Current implementation and agent status live in ../TASKS.md, ../ACTION_LOG.md, and ../ORCHESTRATION.md. Schema checks do not establish runtime completion.
