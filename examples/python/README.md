# Python second consumer

`parser_scenario.py` is an MCP client written with the Python 3.8+ standard library only (no pip packages). It speaks newline-delimited JSON-RPC over stdio to the compiled `graph-mcp` binary and completes the parser-failure scenario from `examples/scripted` without touching the Go API.

Two compiled binaries are involved, both built from this repository:

- `graph-mcp`: the MCP server under test (model-facing tools only).
- `scripted` with the `host` subcommand: the host control CLI for the operations models cannot perform (create graph and goal, claim work, finish work, print the normalized report).

## Run

```sh
go build -o bin/graph-mcp ./cmd/graph-mcp
go build -o bin/scripted ./examples/scripted
export EXPLORATION_DATABASE_URL='postgres://postgres@127.0.0.1:5432/exploration_graph_test?sslmode=disable'
python3 examples/python/parser_scenario.py --graph-mcp bin/graph-mcp --host bin/scripted
```

Use a database you can write to; the scenario migrates it and creates a fresh graph in a unique `python-...` namespace. Pass `--no-drill` to skip the lost-response drill. Stdout is the final state as JSON; the transcript is on stderr. The exit status is non-zero on any failed assertion.

## What it does

1. Host creates the graph and goal.
2. Planner session: checks discovery (six tools, schemas present, no acceptance or observation tool), sends a spoofed `graph_id` (rejected, `invalid_argument`), calls a hidden worker tool directly (`permission_denied`), and proposes work.
3. Host claims the work under a new execution ID.
4. Worker session bound to that work and execution: spoofed `execution_id` is rejected, then it records the negative observation with a resource reference and a conclusion.
5. Host finishes the work; the worker's next write is fenced (`stale_execution`).
6. A fresh planner session started with `-acceptance validator` sees only the graph: it reads the conclusion and its supporting observation, resolves the resource URI with its own local resolver (evidence retrieval is the consumer's job), proposes verification work derived from the retrieved diagnosis, and asks the host to claim it. A new bound worker parses the corrected fixture input `12`, records a positive observation and a conclusion supported by both failure diagnosis and passing evidence, and the host finishes the work. The planner retrieves the passing log, links the verification conclusion to the goal and accepts it. This is a deterministic fixture input correction, not a claim that parser code changed.
7. Host prints the normalized report.

## Ambiguous write drill

This client has no durable invocation identity, and `graph-mcp` does not either. In the drill the client sends the observation write, discards the server's answer to simulate a lost response, and raises `AmbiguousWriteError`. It does not retry. It closes the session, opens a new one with the same binding, reads `graph_list_nodes(kind="observation")`, finds the committed observation and continues with its ID. Had the read found nothing, reissuing the write would have been safe. A blind retry would have created a duplicate (`cmd/graph-mcp/stdio_test.go` demonstrates this).

## Verification

`examples/scripted/stdio_test.go` builds both binaries, runs this script with and without the drill against PostgreSQL, and requires its persisted end state to equal the in-process normalized state (nodes, per-node lifecycles, relations, command history, acceptance closure, graph revision). Both also match the fixed, reviewed 14-event golden report in `examples/scripted/main_test.go`; acceptance reviews four evidence nodes and both failure/passing resource references.
