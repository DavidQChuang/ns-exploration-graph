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

## 007: CreateGraph across namespaces returns permission_denied

A CreateGraph for a graph ID that already exists in another namespace returns `permission_denied`. The store checks the namespace partition before the service can return `already_exists`, following SPEC's "verify namespace" step. `already_exists` remains the result for a different creation of that identity in the same namespace. Conformance expects this; the code is unchanged.

## 008: The 64 KiB result cap measures the toolkit envelope

The enforced 64 KiB cap applies to the toolkit envelope, the structured tool result. The MCP adapter also sends that envelope as text content, so the protocol frame is larger than the cap. Hosts that need a smaller on-the-wire result lower `ResultBytes`. The budget is unchanged.

## 009: Snapshot import does not use wall-clock order

`CreatedAt` is written by whichever process committed the record, so separate hosts or a clock step can reorder it. Import therefore does not compare timestamps. Node revision numbers and each acceptance's stored support-edge and reviewed-resource IDs define history and the acceptance closure; `CreatedAt` remains informational. Acceptance validation walks only the stored support edges: direct edges must target the evaluated goal revision, every reached conclusion needs a stored support, and every stored edge must be reached. Supports added later are not part of the closure. Each reviewed resource must be pinned to a cited evidence revision. Resources attached after acceptance need not be listed, but a cited reference-only observation revision must have a listed resource, because it always had one at decision time.

## 010: Identity length counts Unicode code points

Host request and durable invocation identities MUST be valid UTF-8 and contain at most 200 Unicode code points. This resolves the published "200 characters" limit consistently with JSON Schema string lengths. Memory and PostgreSQL enforce the same rule; invalid identities do not reserve an idempotency key. All command, projection, snapshot, and result size budgets continue to measure serialized UTF-8 bytes rather than character counts. The conformance suite tests exactly 200 non-ASCII characters, 201 characters, and malformed UTF-8.

## 011: Host resource references use the portable URI validator

Host commands validate resource URIs with the same compiled URI fragment and format assertions as the portable snapshot schema. Independent `net/url.Parse` checks admitted malformed IPv6 authorities such as `http://::1/log`, producing committed data that failed snapshot validation. The shared conformance suite now rejects those writes atomically and proves valid escaped opaque URI export/restore. This decision establishes host/schema parity; it does not claim that the dependency's URI validator rejects every nonconforming RFC edge case (for example, it accepts opaque URI spaces).

## 012: Contract freeze and candidate version

The integrator approves normative tool contract 0.2 and snapshot schema support set `{1}` for the source-release candidate 0.2.0. Approval freezes public semantics and metadata; it does not assert that final-candidate verification or artifact sign-off has completed. Changes to released 0.2.x patch versions must preserve those public contracts. Intentional breaking changes require a recorded decision, a new minor contract/release version, migration or compatibility notes, and corresponding fixtures. No remote publication is authorized by this local release preparation.

## 013: Canonical UUID representation at the snapshot boundary

UUID lexical case does not change identity. After structural validation, snapshot validation/restore canonicalizes only contract-defined UUID identities and references to lowercase before semantic checks and persistence, including historical work execution IDs and acceptance closure/resource-reference arrays. This prevents PostgreSQL UUID columns and JSONB references from diverging after a schema-valid uppercase import. Case-alias duplicate identities are rejected before any mutation. Opaque text, resource URIs, actor identities, and extensions are preserved. This is snapshot-boundary normalization; ordinary host APIs continue to expect the exact identifiers returned by the service rather than promising lookup aliases for arbitrary UUID spellings.
