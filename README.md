# ns-exploration-graph

Durable exploration memory for agent harnesses. **ns** stands for **North Star**.

Keep goals, proposed work, observations, conclusions and the evidence behind decisions in a shared, revisioned graph. Agents use eight schema-validated tools; a host can embed the Go library or expose those tools through a stdio MCP server. Memory and PostgreSQL stores share the same command service.

The host runs models, schedules workers, authenticates callers and resolves evidence. The graph records provenance, enforces work and revision rules, and tracks which evidence a goal acceptance reviewed.

## Try the workflow

Use a patched Go toolchain. The module declares `go 1.25.0`; release verification exercised Go 1.25.0 for compatibility and Go 1.27.3 for the patched compiler lane.

From the repository root, run the deterministic in-memory example:

```sh
go run ./examples/scripted -transport direct
```

It records a parser-input failure, resumes from graph state, verifies corrected input and accepts a goal. The result has 14 events, two completed work items and a met goal. This example needs neither PostgreSQL nor a model.

Run the same workflow through an in-process MCP client/server:

```sh
go run ./examples/scripted -transport mcp
```

For a Go host, continue with the [Go library guide](docs/02-go-library.md). For an agent client, start with [Agent integrations](docs/01-agents.md) and the [MCP guide](docs/03-mcp.md). The stdio binary uses PostgreSQL and an existing graph; its setup is separate from this memory example.

## Documentation

The [documentation index](docs/README.md) provides task-based reading paths.

- [Agent integrations](docs/01-agents.md): connect a harness and bind planner, worker or solo sessions.
- [Go library](docs/02-go-library.md): build a host with runnable examples and PostgreSQL setup.
- [MCP](docs/03-mcp.md): configure the stdio process and use its eight tools.
- [Exploration workflow](docs/04-workflow.md): follow work, evidence, acceptance and revision handling.
- [Architecture](docs/05-architecture.md): components, graph semantics and trust boundaries.
- [Storage](docs/06-storage.md): persistence, events, concurrency and portable snapshots.
- [Operations](docs/07-operations.md): limits, verification, recovery and troubleshooting.
- [Roadmap](docs/08-roadmap.md): current capabilities, known limits and possible next steps.

Specifications, architecture decisions, release evidence and contributor resources are linked from the [maintainer index](docs/maintainers/README.md).

## Release and compatibility

Local source release **0.2.0** is tagged `v0.2.0`. Tool contract **0.2** is approved; snapshot schema **1** is the only supported format. The module path is `ns-exploration-graph`; it is distributed as local source and has not been published to a registry or remote repository.

- Go module minimum: `go 1.25.0`; tested with Go 1.25.0 and 1.27.3.
- PostgreSQL: tested 17.11; the verification workflow pins its container image by digest.
- MCP SDK: `github.com/modelcontextprotocol/go-sdk` v1.8.0.
- MCP protocol: SDK client negotiation uses `2026-07-28`; the raw compatibility client uses `2025-11-25`.
- JSON Schema: 2020-12, with shared runtime format assertions and versioned input/output contracts.

Both clean compiler lanes recorded 195 passing test/subtests, zero failures and zero skipped tests, with the race detector, PostgreSQL and Python required. The stdio Python consumer and direct/MCP examples agreed. The [release artifact record](docs/RELEASE-ARTIFACTS.md) identifies the tested runtime, immutable source tag and detached checksums; current documentation updates on `main` do not move that tag. Hosted GitHub Actions has not run.

Read [Operations](docs/07-operations.md) before deploying or verifying a host. Graph writes are idempotent only when the host preserves invocation identities; a stock MCP client must reconcile ambiguous writes. Graph size affects read and mutation costs. Authorization, external effects and evidence retrieval remain host responsibilities.

## License

Apache-2.0. See [LICENSE](LICENSE), [NOTICE](NOTICE) and the [dependency inventory](docs/DEPENDENCIES.md).
