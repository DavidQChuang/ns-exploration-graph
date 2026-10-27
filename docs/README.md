# ns-exploration-graph documentation

These guides explain how to give agents durable exploration memory and embed it in a host harness. Start with the short workflow in the [project README](../README.md), then choose a path below.

The implemented release is 0.2.0: a Go library, memory/PostgreSQL stores, eight tools and a stdio MCP adapter. The host supplies model execution, scheduling, authentication and evidence resolution. Future integrations and UI work are described separately in the roadmap.

## Start here

| I want to… | Read |
| --- | --- |
| Connect an agent or design its planner/worker sessions | [Agent integrations](01-agents.md) |
| Embed the graph in a Go program | [Go library](02-go-library.md) |
| Start the stdio server or inspect tool inputs and outputs | [MCP guide](03-mcp.md) |
| Understand how work becomes evidence and accepted goals | [Exploration workflow](04-workflow.md) |
| See the components, node types and trust boundaries | [Architecture](05-architecture.md) |
| Choose a store, read events or export/restore a graph | [Storage and portability](06-storage.md) |
| Configure limits, recover from errors or run verification | [Operations](07-operations.md) |
| Check implemented capabilities and possible next steps | [Status and roadmap](08-roadmap.md) |

## Recommended paths

**Agent integration:** [Agents](01-agents.md) → [MCP](03-mcp.md) → [Workflow](04-workflow.md). This path explains what an agent sees, which operations its host grants and how it records durable results.

**Build a harness:** [Go library](02-go-library.md) → [Workflow](04-workflow.md) → [Architecture](05-architecture.md) → [Storage](06-storage.md). Use this path when your program owns assignments, execution identities and acceptance policy.

**Operate an integration:** [Operations](07-operations.md) → [Storage](06-storage.md). Read the error/retry guidance before adding automatic recovery or snapshot restore.

For contributor setup, normative schemas, design decisions, release audits and orchestration history, use the [maintainer index](maintainers/README.md). Machine-readable contracts stay in [spec/](../spec/); these guides explain their use rather than replacing them.
