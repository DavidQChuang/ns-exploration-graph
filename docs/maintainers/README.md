# Maintainer document index

[Documentation](../README.md) · [Project](../../README.md)

This index routes maintainers to the existing records in place. Normative specifications define behavior; release records retain approval and verification evidence; coordination logs preserve the work history.

## Consumer and operator guides

- [Repository overview and quick start](../../README.md) — project scope, quick start and guide navigation.
- [Operations guide](../07-operations.md) — database setup boundaries, stdio invocation, environment variables, limits, troubleshooting, and recovery.
- [Agent guide](../01-agents.md) and [Go library guide](../02-go-library.md) — host setup, trusted identity, lifecycle and evidence-resolution responsibilities.
- [Python consumer](../../examples/python/README.md) — second-consumer scenario and invocation details.
- [Scripts guide](../../scripts/README.md) — local PostgreSQL helpers and release/clean-environment verification procedures.

## Normative contracts

- [Semantic specification](../../spec/SPEC.md) — graph data, command rules, errors, execution fencing, snapshots, and reads.
- [Tool contract](../../spec/TOOLS.md) — tool inputs/results, role behavior, limits, retries, and MCP boundary.
- [JSON schemas and contract assets](../../spec/) — snapshot/tool schemas and checked-in generated contract files.
- [Compatibility policy](../COMPATIBILITY.md) — approved versions and pre-1.0 compatibility expectations.
- [Architecture decisions](../DECISIONS.md) — accepted design choices and their rationale.

## Release, verification, and provenance records

- [Release artifacts](../RELEASE-ARTIFACTS.md) — local archive inventory, hashes, and inspection results.
- [Release checklist](../RELEASE-CHECKLIST.md) — pre-delivery gates and checklist history.
- [Release notes](../RELEASE-NOTES.md) — current release behavior and changes.
- [Clean-environment verification](../CLEAN-ENVIRONMENT.md) — exact compiler/runtime matrix, results, limitations, and CI distinction.
- [PostgreSQL verification](../POSTGRES-VERIFICATION.md) — database correctness, measurements, and remaining performance bounds.
- [Dependency record](../DEPENDENCIES.md) — dependency inventory and verification notes.
- [Provenance](../PROVENANCE.md) — source and verification provenance.

## Coordination history

- [Action log](../../ACTION_LOG.md) — chronological work and evidence log.
- [Task tracker](../../TASKS.md) — current task coordination record.
- [Orchestration record](../../ORCHESTRATION.md) — integration and assignment history.

Keep current guides, normative contracts and unique verification/provenance evidence. Remove obsolete redirects, completed planning documents and duplicate summaries when their essential facts already have a canonical home; update live references at the same time. Git history and immutable release archives retain previous versions. Add new canonical records here and update the owning record when its status changes.
