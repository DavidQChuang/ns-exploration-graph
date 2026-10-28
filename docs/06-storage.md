# Storage and portability

[Documentation](README.md) · [Agents](01-agents.md) · [Go library](02-go-library.md) · [MCP](03-mcp.md) · [Workflow](04-workflow.md) · [Architecture](05-architecture.md) · [Storage](06-storage.md) · [Operations](07-operations.md) · [Roadmap](08-roadmap.md)

Both stores persist the same semantic model through `graph.Service`. Choose memory
for tests and process-local examples; choose PostgreSQL when graph state must
survive process exit. Neither store supplies caller authentication.

## Open the appropriate store

Memory needs no setup:

```go
store := memory.New()
svc := graph.NewService(store)
```

For PostgreSQL, the host opens a connection and initializes the schema explicitly:

```go
store, err := postgres.Open(ctx, dsn)
if err != nil {
    return err
}
defer store.Close()
if err := store.Migrate(ctx); err != nil {
    return err
}
svc := graph.NewService(store)
```

These snippets assume imports from `ns-exploration-graph/graph` and the selected
`graph/store/memory` or `graph/store/postgres` package. `postgres.Open` pings the
database; `Migrate` is a separate operation. The `graph-mcp` binary does neither
graph creation nor migration on your behalf.

`Migrate(ctx)` embeds
[`001_initial.sql`](../graph/store/postgres/migrations/001_initial.sql). It takes
a transaction advisory lock, checks whether `exploration_graph.graphs` exists,
and executes the DDL only when that table is absent. Repeated initialization is
safe for an intact schema. This is not a migration ledger, schema repair tool, or
upgrade mechanism; an existing graphs table causes it to skip DDL without checking
the other objects. The raw SQL file itself uses ordinary `CREATE` statements.

## Know what the seven tables hold

All tables live in the `exploration_graph` schema:

- `graphs`: graph identity, namespace, state, revision, metadata, and timestamps.
- `nodes`: immutable kind and current-revision pointer for each graph-local node.
- `node_revisions`: append-only state, title, typed payload, actor, and timestamp.
- `edges`: typed endpoints, revision pins, actor, and timestamp. Dependencies have
  no revision pins.
- `resource_refs`: URI, resource type, optional external revision/SHA-256, and the
  exact node revision to which the reference belongs.
- `goal_acceptances`: evaluated/resulting goal revisions, reviewed support closure
  and resource IDs, actor/method/rationale, and current/stale status.
- `graph_events`: ordered command events, request ID/hash, command data, and the
  committed result used for retry replay.

Foreign keys and indexes protect graph-local references and uniqueness. The shared
service additionally enforces endpoint kinds, lifecycle transitions, freshness,
cycles, and acceptance closure. Writing tables directly bypasses those rules.
Canonical definitions are [SPEC](../spec/SPEC.md) and
[the schema DDL](../graph/store/postgres/migrations/001_initial.sql).

## Understand concurrent writes and reads

PostgreSQL mutations run in one transaction under `SELECT … FOR UPDATE` on the
graph row. Creation inserts if absent, then takes that same lock. The namespace
must match before the callback runs. Semantic checks and all resulting changes,
including the event, commit together; failure rolls them all back.

The lock serializes writes to one graph, including competing claims and dependency
edits. Expected node revisions detect outdated input; an optional
`ExpectedGraphRevision` adds a graph-wide check. Every new successful command
advances the graph revision once, even if it revises multiple nodes.

For idempotency, `UpdateRequest` loads the event matching `(graph_id, request_id)`
through the unique index. Same request and input return the original stored result;
changed input returns `idempotency_conflict`. This is request replay, not semantic
deduplication. A new identity represents a new operation.

Ordinary service reads use `ReadScoped` in a repeatable-read transaction. They load
all current node revisions and all edges. A detail request adds that node's full
history, resource references, and goal acceptances. `Snapshot` uses the full
content read instead. These reads omit event envelopes; use `EventsAfter` for them.

The memory adapter returns detached copies with equivalent semantics, but a
store-wide mutex serializes its updates. Its data disappears with the process.
Both stores use the same service rules; memory is the semantic reference adapter.

## Keep authority in the host

Namespace equality is a partition check, not proof that a caller may use that
namespace. Authenticate and authorize before supplying trusted bindings or calling
the Go API. Graph IDs are database-global identities; cross-graph edges are not
supported. This release does not provide per-node ACLs or database row-level policy.

A running work revision contains a host-issued execution ID. New writes require
the matching running assignment; stopping, completing, or replacing it fences the
old execution. There is no expiry, heartbeat, distributed lease, or automatic
reclaim. The host must separately control external effects and worker recovery.

## Store external evidence as references

Record useful inline evidence and resource references through semantic commands.
A reference can look like this Go value from the scripted fixture:

```go
graph.ResourceInput{
    ResourceType: "test_log",
    URI:          "fixture://parser/rejection-log",
}
```

The graph stores the reference, not the referenced artifact body. The host resolves
the URI, checks access, and verifies optional digests or external revisions. URI
validation does not perform a network request.

Host writes, toolkit validation, and snapshots share the recorded absolute-URI
guard, including explicit IPv6/IPvFuture authority checks. This is not a claim of
complete strict RFC lexical validation; existing opaque/custom-scheme behavior
is retained. See [decisions 011 and 014](DECISIONS.md#011-host-resource-references-use-the-portable-uri-validator).

## Export and validate a portable snapshot

The actual host APIs are:

```go
func (s *Service) Snapshot(ctx context.Context, ns, id string) (Snapshot, error)
func (s *Service) ExportSnapshot(ctx context.Context, ns, id string) ([]byte, error)
func ValidateSnapshot(encoded []byte) (Snapshot, error)
func (s *Service) ImportSnapshot(ctx context.Context, c CommandContext, encoded []byte) (Result, error)
```

`Snapshot` reads consistent full content at one graph revision. `ExportSnapshot`
sorts record arrays by ID and returns JSON. The default import/export content cap
is 64 MiB via `svc.Limits.SnapshotBytes`; export checks after loading the graph.

Only snapshot schema version 1 is supported: the support set is `{1}`. Validation
checks the canonical [JSON Schema](../spec/snapshot.schema.json), asserted UUID,
URI, and date-time formats, then cross-record integrity. It requires exactly one
origin, contiguous revision histories, valid references and lifecycle transitions,
legal relations, no support/dependency cycles, and consistent acceptance closures.

Typed UUID fields are canonicalized to lowercase before semantic validation and
storage, including historical execution IDs and acceptance reference arrays.
Case-alias duplicate identities are rejected. Opaque metadata, extensions, actor
strings, and URIs are preserved; ordinary host calls should reuse exact returned
IDs rather than relying on case aliases.

## Restore into an unused identity

Given an authorized source binding and destination namespace, this is the core flow:

```go
encoded, err := svc.ExportSnapshot(ctx, sourceNamespace, sourceGraphID)
if err != nil {
    return err
}
result, err := svc.ImportSnapshot(ctx, graph.CommandContext{
    Namespace: destinationNamespace,
    GraphID:   graph.NewID(),
    ActorID:   hostActor,
    RequestID: restoreRequestID,
}, encoded)
```

Persist `restoreRequestID`, the destination ID, and the bytes before sending a
restore if you need to retry it. Reuse the same context and bytes for that retry.

Restore is atomic and requires an unused graph identity; it does not merge graphs.
It preserves node/edge/reference/acceptance IDs and node histories, replaces graph
identity and namespace, and creates destination revision 1 with one import event.
Source graph ID/revision are saved under `metadata.import_source`. A collision
with that reserved key fails, including reimport of an already-restored snapshot
without deliberate host preparation. Source graph lifecycle state is retained.

Running work is rejected: stop or pause it in the source before export for restore.
Export itself can contain running work, so successful export is not a guarantee
that the bytes are restorable. Historical execution IDs remain provenance.
Snapshots exclude request/event history; source retries cannot replay at the
destination. Unknown versions and invalid content fail without partial restore.

## Resume event readers and paginated reads

Use `svc.EventsAfter(ctx, ns, id, afterRevision, limit)` for ordered durable events.
Default limit is 100, maximum 1,000. Persist `NextRevision` only after processing
that page; `GraphRevision` reports the consistent read's head. No notification or
subscription service is shipped. A host can poll and coalesce planner wakeups.

List, traversal, and related-record cursors bind to graph revision and query.
Mutation between pages returns `stale_cursor`; restart the read. These opaque
cursors are different from the event feed's numeric revision checkpoint.

For database setup, testing, and release evidence, continue with
[Operations](07-operations.md) and [PostgreSQL verification](POSTGRES-VERIFICATION.md).
