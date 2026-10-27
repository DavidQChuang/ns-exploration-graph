# MCP guide

[Documentation](README.md) · [Agents](01-agents.md) · [Go library](02-go-library.md) · [MCP](03-mcp.md) · [Workflow](04-workflow.md) · [Architecture](05-architecture.md) · [Storage](06-storage.md) · [Operations](07-operations.md) · [Roadmap](08-roadmap.md)

`graph-mcp` exposes the eight graph tools over local stdio. One process serves one pre-existing graph, actor and role. Your harness starts the process, runs the model and owns work scheduling, evidence retrieval and completion.

The repository tests an official Go SDK client and a Python standard-library stdio client. The setup below describes a generic subprocess harness. It is not a tested configuration for Codex, Claude or Cursor.

## Prepare the database and graph

Use Go 1.25.0 or newer to build from the repository root:

```sh
go build -o bin/graph-mcp ./cmd/graph-mcp
```

On Windows, build `bin/graph-mcp.exe` instead. Prepare a PostgreSQL store with `postgres.Open` and `store.Migrate`, then create a graph with the host API as shown in the [Go guide](02-go-library.md#switch-to-postgresql). The MCP binary does not migrate or create graphs.

Configure your harness to execute the absolute binary path with an argument vector equivalent to:

```text
-namespace YOUR_NAMESPACE -graph EXISTING_GRAPH_UUID -actor YOUR_ACTOR -role planner
```

These are launch placeholders; replace them with the persisted host binding. Pass `EXPLORATION_DATABASE_URL` in the subprocess environment. Keep stdin/stdout connected for MCP messages. When the client closes stdin, the server exits with status 0. Invalid configuration exits nonzero; database configuration errors do not echo the connection string.

A planner session starts with six tools. The server instructions include the role prompt, a starting overview and configured byte limits. Reuse that overview when sufficient.

## Set launch flags from trusted host state

- `-namespace`, `-graph`, `-actor`: required binding; the graph must already exist.
- `-role`: `planner` by default, or `worker` / `solo`.
- `-work` and `-execution`: both valid UUIDs together, taken from a successful host claim. Observation writes require this binding; worker conclusion writes require it too.
- `-acceptance`: optional `human`, `validator` or `agent`; grants `graph_accept_goal`. Off by default.

The acceptance flag is a process-level grant with an allow policy. Embed the adapter if you need per-request acceptance review. Neither the binary nor the embedded service authenticates arbitrary remote clients.

Planner tools are overview/get/list, propose work, record conclusion and add support. Worker tools are overview/get/list, record observation and record conclusion. Solo combines these presets. Acceptance is separate from every preset. Discovery lists granted tools; authorization also checks every invocation, including direct calls to hidden tools and retries.

## Read arguments and result envelopes

The sections below show schema-valid **argument shapes**, not a runnable sequential workflow. UUIDs are illustrative existing records; substitute exact IDs and revisions returned by your own graph. Inventing an ID does not create its record. The complete [scripted example](../examples/scripted/main.go) and [Python consumer](../examples/python/README.md) retain returned IDs throughout.

All tools return an envelope. Here is a complete illustrative `graph_propose_work` success shape:

```json
{
  "ok": true,
  "graph_revision": 2,
  "data": {
    "work_item_id": "11111111-1111-4111-8111-111111111111",
    "node_revision": 1,
    "lineage_edge_ids": ["22222222-2222-4222-8222-222222222222"]
  }
}
```

A failure has this shape (messages and optional details vary):

```json
{
  "ok": false,
  "error": {
    "code": "revision_conflict",
    "message": "Node revision changed; read it again.",
    "retryable": false,
    "details": {"current_revision": 3}
  }
}
```

Check `ok`. MCP returns the envelope in `structuredContent` and as JSON text in `content`, with `isError: true` for tool failures. A successful protocol exchange can therefore contain a failed graph operation. Successful writes include `graph_revision`; reads identify their snapshot revision, which may already have changed by the next call.

Unknown input fields are rejected. Namespace, graph, actor, role, acceptance method, producer work and execution are host bindings, never model arguments. Model revision references use `id` and `revision`; Go host `graph.Ref` serializes its ID as `node_id`.

## Get a bounded overview: graph_overview

```json
{"byte_budget": 16384}
```

Also accepts `{}`. Returns `data.origin`, `goals`, `work`, `evidence`, `hints`, `edges`, `totals`, `omitted`, `truncated` and `retrieval`. Use omission counts and retrieval routes to decide what to read next. The default budget is 16 KiB; the schema allows 1024–65536 bytes, subject to the adapter's result cap. A budget too small for required context returns `budget_too_small`.

## Read one revision: graph_get_node

```json
{"node_id": "11111111-1111-4111-8111-111111111111", "revision": 1}
```

Omit `revision` for the current revision. Returns `data.id`, `kind`, `current_revision`, `node`, `freshness`, `edges`, `resource_refs`, `total_related`, `omitted_related` and optional `next_related_cursor`. The `node` value is one revision, not the complete history.

Pass the returned cursor as `related_cursor` with the same node/revision selection to retrieve further edges and resource references. Cursors bind to the observed graph revision. After `stale_cursor`, restart the read; an oversized node alone returns `limit_exceeded` and needs a host read/export path.

## Find current nodes: graph_list_nodes

```json
{"kind": "work_item", "state": "open", "limit": 20}
```

All fields are optional. Returns `data.nodes`, `total`, `omitted` and optional `next_cursor`. Entries include `id`, `kind`, `revision`, `state`, `title` and `freshness`. Continue with the returned `cursor` and the same filters. The default limit is 20, maximum 100. Use a state compatible with the selected kind. Restart after `stale_cursor` if the graph changed.

## Propose an open task: graph_propose_work

```json
{"title": "Reproduce rejection", "instruction": "Run the parser on twelve and retain the output.", "priority": 5}
```

Returns `work_item_id`, `node_revision` and `lineage_edge_ids` inside `data`. Optional `parents` is an array of exact `{"id":"…","revision":1}` references. Without parents, lineage defaults to the origin. Proposing work creates an open item; only the host can claim and execute it. Priority is an integer from 0 to 100.

## Preserve evidence: graph_record_observation

```json
{"title": "Rejection reproduced", "statement": "The parser rejected twelve.", "evidence": "Input twelve; invalid integer; exit 1.", "epistemic_status": "observed", "polarity": "negative"}
```

Requires a host-bound running execution. Returns `observation_id`, `node_revision`, `yields_edge_id` and `resource_reference_ids`. Inline `evidence` must be nonempty. Optional fields are `detail`, `polarity` and `resource_refs`; epistemic status is `observed`, `inferred` or `reported`.

Each resource reference has `resource_type` and an absolute `uri`, with optional `external_revision` and lowercase 64-character `sha256`. Resources identify external evidence; the host resolves, authorizes and verifies it. A missing binding returns `permission_denied`; a fenced assignment returns `stale_execution`.

## Synthesize supported evidence: graph_record_conclusion

```json
{"title": "Input is malformed", "statement": "The fixture uses a word where an integer is required.", "epistemic_status": "inferred", "supports": [{"id": "11111111-1111-4111-8111-111111111111", "revision": 1}]}
```

Requires at least one supporting evidence revision. Returns `conclusion_id`, `node_revision`, `support_edge_ids`, `resource_reference_ids` and, when produced by assigned work, `yields_edge_id`. Optional `detail` and `resource_refs` follow the observation shapes. A worker needs a bound execution; a planner may synthesize without one. The service checks support types, freshness and cycles.

## Link evidence to a conclusion or goal: graph_add_support

```json
{"source": {"id": "11111111-1111-4111-8111-111111111111", "revision": 1}, "target": {"id": "33333333-3333-4333-8333-333333333333", "revision": 1}}
```

Returns `data.edge_id`. The source must be evidence; the target must be a permitted conclusion or goal. Both references are exact revisions. Linking does not verify the evidence or accept the goal. Invalid types, stale evidence and cycles fail without a write.

## Record reviewed acceptance: graph_accept_goal

```json
{"goal": {"id": "33333333-3333-4333-8333-333333333333", "revision": 1}, "support_edge_ids": ["22222222-2222-4222-8222-222222222222"], "rationale": "The selected evidence satisfies the reviewed criteria."}
```

Requires the explicit acceptance grant. Select current direct support edges for the exact goal revision after reviewing their evidence; do not substitute lineage or yields edges. Returns `acceptance_id`, `resulting_goal_revision`, `reviewed_evidence`, `support_edge_ids` and `reviewed_resource_ids`. Reviewed evidence uses `id`/`revision` references. Acceptance records a reviewed closure; the host decides whether to close the graph or finish the task.

## Reconcile a lost write response

An embedding host can supply `adapters/mcp.Options.ResolveInvocation`. Persist an invocation identity and its original binding/arguments before sending a mutation; the resolver returns that identity for the request. Retrying identical arguments with that identity returns the original success envelope, including its original graph revision. Different arguments return `idempotency_conflict`; a new identity is a new operation. Authorization still runs on retries.

The stock stdio binary has no resolver and generates a new random identity per mutation. A lost response leaves the outcome ambiguous. Stop writing, reconnect under the appropriate binding, and read with `graph_list_nodes` / `graph_get_node` to establish whether the write committed. Issue a new mutation only after reconciling; a blind retry can duplicate it. Do not derive durable identity from a JSON-RPC request ID or model-controlled `_meta`.

Only `storage_unavailable` is retryable at the toolkit boundary, and only with the same durable identity. Without a resolver, MCP clears that retry flag and adds `details.outcome: "ambiguous"`. On `revision_conflict` or `stale_evidence`, reread and reconsider. On `stale_execution`, the worker stops and returns control. The full fixed error set and schemas are in [spec/TOOLS.md](../spec/TOOLS.md) and [tool-contracts.json](../spec/tool-contracts.json).

Default tool input is capped at 256 KiB and structured output at 64 KiB; at most 100 parents, supports or resources are accepted per command. Over-limit input fails before a partial write. The MCP frame also includes a text copy and is larger than the structured envelope.

Continue with the [host workflow](04-workflow.md), [Go embedding](02-go-library.md), or the [Python subprocess scenario](../examples/python/README.md).
