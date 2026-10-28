# Roadmap and release status

[Documentation](README.md) · [Agents](01-agents.md) · [Go library](02-go-library.md) · [MCP](03-mcp.md) · [Workflow](04-workflow.md) · [Architecture](05-architecture.md) · [Storage](06-storage.md) · [Operations](07-operations.md) · [Roadmap](08-roadmap.md)

This page summarizes what the local source release contains and separates delivered behavior from ideas that have no approved schedule.

## 1. Current release

Version 0.2.0 is an approved local source release. Tool contract 0.2 is approved; snapshot schema version 1 is the only supported snapshot version. The release includes the core Go library, memory and PostgreSQL stores, eight schema-validated tools, in-process and stdio MCP adapters, prompt assets, a deterministic Go scenario, and a Python standard-library consumer. It is source delivery, not a hosted product or package publication.

Runtime verification passed for source revision `62c083f422be2f25e538724179c9fa142bd17e91` on Go 1.25.0 and 1.27.3. Each clean lane recorded 195 test/subtest passes, no failures or skips, with required PostgreSQL 17.11 and Python 3.12. Direct, in-process MCP, and Python stdio reports agreed. The local tag/artifact sign-off and exact archive checksums are in [RELEASE-ARTIFACTS.md](RELEASE-ARTIFACTS.md). Hosted GitHub Actions has not run.

## 2. Delivered capabilities

- Typed, revisioned graph nodes and relations, with lifecycle rules and immutable history.
- One semantic service used by in-memory and transactional PostgreSQL stores.
- Bounded projections and retrieval, event reads, consistent snapshots, and validated restore into a fresh graph identity.
- Host-bound actors, roles, producer work and execution identity; stale workers are fenced from graph writes.
- Eight narrow tools with schema validation, role presets, explicit acceptance grants, and stable error envelopes.
- Local stdio MCP bound to one pre-existing graph, plus in-process use and documented retry reconciliation.
- External resource references whose fetching, authorization, integrity checking, and retention remain with the host.

See [README.md](../README.md), [SPEC.md](../spec/SPEC.md), and [TOOLS.md](../spec/TOOLS.md) for the normative interfaces and behaviors.

## 3. Measured limits

The design targets modest investigation graphs, not unlimited graph sizes or high-throughput remote serving. PostgreSQL ordinary reads remain O(N+E) in rows transferred; mutations load the graph's node history, edges, resources, and acceptances. Snapshot reads are whole-graph. `ListEligibleWork` has no limit or cursor, and traversal summaries are not checked against the core result byte cap. The memory store clones aggregates and is for semantic coverage, not performance claims.

The 10,000-node benchmark observations and their measurement conditions are recorded in [POSTGRES-VERIFICATION.md](POSTGRES-VERIFICATION.md). They are measured limitations, not a service-level objective. The local release does not provide a UI, worker scheduler, model runtime, remote authentication, or hosted multi-tenant service.

## 4. Ideas requiring a separate decision

These are proposals only. They are not committed release plans, promises, owners, or timelines:

- A read-only viewer (optional M3).
- A SQLite store for simpler local persistence.
- Cross-graph links or queries.
- Model-generated hot/cold summaries or graph digests that preserve exact source membership and revisions.
- Remote serving and authentication/session isolation.
- Bounded PostgreSQL reads, cursor design for eligible-work results, or a query strategy that avoids loading all graph rows.

Each proposal needs an explicit contract, security and data-integrity review, migration/compatibility decisions where applicable, and evidence before it becomes release scope. No implementation schedule is approved.

## 5. Compatibility and release records

Contract compatibility policy is in [COMPATIBILITY.md](COMPATIBILITY.md). Release approval, artifact provenance, clean-environment evidence, and operational requirements remain in their existing records. Consult [the maintainer index](maintainers/README.md) for the correct record and its authority. The maintainer index identifies the retained canonical records.
