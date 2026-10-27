# Host integration guide

The task-oriented guides now provide the maintained host integration instructions:

- [Agent integrations](01-agents.md): session roles, trusted bindings and initial context.
- [Go library](02-go-library.md): complete examples, graph creation, commands and toolkit embedding.
- [MCP](03-mcp.md): stdio configuration, tools, authorization and invocation identity.
- [Exploration workflow](04-workflow.md): assignment, evidence, acceptance and revision handling.
- [Operations](07-operations.md): limits, retries, recovery and verification.

Prompts are guidance, not enforcement. The host authorizes calls and binds identities; the toolkit and command service enforce their respective rules regardless of what an agent is told or reads.

The normative contracts remain [SPEC.md](../spec/SPEC.md) and [TOOLS.md](../spec/TOOLS.md). Use the [documentation index](README.md) for reading paths and the [maintainer index](maintainers/README.md) for compatibility decisions and release evidence. This page preserves existing links to `docs/INTEGRATION.md`.
