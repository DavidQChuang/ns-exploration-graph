# Architecture decisions

## 001: Shared semantic aggregate transactions (approved by integrator)

The local Go module is `ns-exploration-graph`. The graph package owns semantic commands. Stores expose a detached graph aggregate inside one serializable graph mutation callback. PostgreSQL locks the graph row, loads the aggregate, invokes the shared service, and persists normalized table differences and an event in one transaction. Callback failure rolls back all writes. In-memory storage is a reference adapter, not an alternative semantic implementation.

This deliberately prioritizes correctness for modest graphs over write throughput. Aggregate mutation cost grows with graph history; 100/1,000/10,000-node benchmarks document practical bounds. Read snapshots exclude event envelopes; bounded EventsAfter is separate. All store calls require context and namespace. Namespace checks are partition checks, while authorization belongs to host adapters.

Public snapshots mirror the versioned JSON contract. Typed payload structs encode known fields; revision payloads remain raw JSON for immutable storage and strict kind-specific validation. Result data uses a JSON object so storage can preserve exact committed responses on retry.

## 002: Initial revisions, acceptance results and freshness

CreateGraph writes graph revision 1 and its event. Done work is terminal. Dependency edits are permitted only while the dependent remains open. Read summaries label evidence `fresh` or `stale` by recursively checking exact supporting revisions; other kinds are `not_applicable`.

Acceptance results explicitly include acceptance ID, resulting goal revision, reviewed evidence revision references, complete support edge closure and reviewed resource IDs. Host references use `node_id`; model tool references use `id` and adapters translate them. First and replayed results use detached JSON-compatible values (numbers in generic result maps use float64).

## 003: Revision evidence and restore validation

New active observation revisions must supply inline evidence because existing resource references are pinned to older revisions and are not inherited. A reference-only observation can be retracted with an explicit reason; its retracted revision need not manufacture new evidence. The original evidence references remain historical. Restore applies the evidence requirement to active observation revisions and validates that lifecycle transitions are legal.

Snapshot import validates the canonical embedded JSON Schema with UUID, URI, and date-time format assertions, then all cross-record relationships before accessing current revisions. It preserves imported node histories, writes destination event/revision 1, and rejects collisions with the reserved `metadata.import_source` field, including attempts to restore an already-restored snapshot without explicit metadata preparation by the host.

## 004: Scoped ordinary reads and request-scoped idempotency lookup (G3, core_perf)

Two optional, additive store extensions are defined in `graph/types.go`; the `Store` interface and every service method signature and result are unchanged. Stores that do not implement them keep working through `Read`/`Update`, which are supersets.

- `ScopedReader.ReadScoped(ctx, namespace, graphID, ReadScope)` returns a consistent, detached state with every node's current revision only and every edge. `ReadScope.DetailNodeID` adds that node's full history, resource references and acceptances. ListNodes, GetNode/GetRevision, Neighbors/Ancestors/Descendants, ListEligibleWork, ExplainGoal and ProjectContext use it. Snapshot/ExportSnapshot still use the full repeatable-read `Read`. `Node.Current()` now tolerates states that carry only the current revision.
- `RequestUpdater.UpdateRequest(..., requestID, fn)` behaves like `Update`, except that `State.Events` contains only the committed event with that request ID. Idempotency replay/conflict checks are unchanged. PostgreSQL resolves the event through the existing `(graph_id, request_id)` unique index, so the full event history is no longer loaded per mutation.

ProjectContext measures each candidate incrementally: its own encoded bytes, a comma, and the re-encoded omitted counts and truncation flag. Selection order and greedy whole-node omission are unchanged. After selection, the service encodes the complete envelope; if the prediction differs, it recomputes the projection by encoding every candidate (the previous algorithm), so the byte budget is never relaxed. An internal randomized test compares both paths, and a conformance test compares scoped and complete-aggregate reads on memory and PostgreSQL.

## 005: Process-level acceptance grant on graph-mcp

`graph-mcp -acceptance human|validator|agent` is the host's grant of `graph_accept_goal` for that process. It is off by default. The stdio binary has no per-request policy. A host that needs one embeds the library and supplies `AcceptancePolicy`.

## 006: Storage errors do not carry driver text

SPEC requires every error message to be safe and to exclude SQL and credentials. `postgres.storageError` returns `storage_unavailable` with the fixed message "Graph service is unavailable." Coded graph errors pass through unchanged. The toolkit applies the same replacement, so a storage error built with driver text cannot reach a model either.
