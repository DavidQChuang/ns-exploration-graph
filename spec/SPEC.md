# Standalone exploration graph: implementation specification

Status: proposed v0.2, October 27, 2026. This specifies a new implementation inspired by the inspected ARTEX behavior; no library or transport has been implemented. MUST, MUST NOT, and SHOULD express requirements. SQL and JSON files are draft implementation contracts, not proof that the requirements hold.

Read this document for core semantics, [TOOLS.md](TOOLS.md) for agent integration, and [DEVELOPMENT-PLAN.md](DEVELOPMENT-PLAN.md) for delivery order and ownership. `snapshot.schema.json` defines snapshot shape; `tool-contracts.json` defines model-facing inputs. This document governs semantic constraints that schemas cannot express. A conflict between these documents and the schemas is a release blocker, not an invitation for an adapter to choose its own behavior.

## Locked decisions for the first implementation

- Write a new Go module with PostgreSQL persistence. Use the inspected ARTEX code as behavioral reference; do not assume permission to relicense extracted source.
- Implement one shared command service. Both the in-memory test adapter and PostgreSQL adapter run the same semantic rules.
- Use UUID identities, immutable node revisions, explicit evidence references, and semantic commands. No model-facing arbitrary node/edge mutation.
- Serialize graph mutations with a graph-row lock. Require node revision checks for changes to existing nodes; graph-wide optimistic revision checks remain optional.
- Deliver an in-process tool adapter and deterministic scripted harness first. Then deliver an MCP adapter against the same service. The release called the portable tool requires both; the core-library milestone does not.
- Keep scheduling, model invocation, authentication, external-effect execution, and artifact resolution in the host. Store an execution identity to reject stale worker writes without implementing a scheduler.
- Defer full ARTEX migration, cross-graph inheritance, SQLite, automatic summarization, and an editable web UI. A read-only viewer is a separate follow-on milestone.

## Objective and boundary

Provide durable, typed relationships between objectives, work, observations, conclusions, and external evidence. Any harness can write to and query this graph through ordinary library calls. The core requires neither an LLM nor an agent SDK, HTTP server, scheduler, or graph visualization.

Initial core: Go library and PostgreSQL adapter, preserving the source project's implementation language while publishing a language-neutral JSON contract. Ship an in-memory adapter for semantic tests. The portable-tool release additionally includes the integration kit and MCP adapter specified in TOOLS.md. An HTTP application server and web UI remain separate integrations.

The graph records structured claims and provenance; it cannot establish that a model's claim is true. The host supplies authorization, acceptance policy, and any independent verification.

## Extraction map

The current ARTEX implementation distributes the relevant behavior across:

- `db/exploration.go`: graph reads/writes mixed with activity logs, token accounting, finding joins, cancellation cleanup, and worker claims.
- `db/schema.sql`: graph tables embedded in the full application schema; anchors reference `assets` directly.
- `agent/tools.go`: semantic rules, lineage construction, goal acceptance, and the compact graph overview, wrapped in Norma tools.
- `db/exploration_sources.go`: cross-graph read views implemented through ARTEX task relations.
- `db/digest.go`, `agent/coldgraph.go`, `agent/compaction.go`, and `agent/tools_digest.go`: storage, graph algorithms, LLM summarization, and presentation of older graph regions.
- Finding recording and evidence storage: separate host services with vulnerability and traffic-specific data.

The extraction must move semantic validation out of tool handlers into library commands. A direct copy of `ExplorationStore` would carry application dependencies into the new package. The existing cold-graph algorithms have a useful separation from I/O, but still reference ARTEX graph types/constants and need adaptation.

Recommended package boundaries:

```text
graph/                  IDs, types, commands, validation, relation rules
graph/store/            transactional storage interface
graph/store/postgres/   SQL, migrations, isolated database tests
graph/store/memory/     semantic reference adapter for tests
graph/projection/       bounded context views and lineage queries
schema/                 JSON contracts and PostgreSQL migration
adapters/artex/         source-model translation and host callbacks
adapters/norma/         optional CoreTool wrappers
toolkit/                model-facing contracts, role presets, prompt fragments
adapters/mcp/           transport/session binding, no graph business rules
examples/scripted/     deterministic harness with no model dependency
```

Public interfaces accept `context.Context`; errors have stable codes. No core package imports ARTEX, Norma, a provider SDK, or HTTP/UI packages. The adapter can depend on the core; the core must never depend on the adapter.

## Semantic model

An exploration is a graph with a namespace, optional host-owned external reference, description, lifecycle state, and monotonically increasing revision.

Node kinds:

- `origin`: original request or supplied background; explicitly not independently verified evidence.
- `goal`: a desired outcome with acceptance criteria.
- `work_item`: a proposed or performed unit of work.
- `observation`: a recorded result, positive or negative, carrying an epistemic label.
- `conclusion`: a claim synthesized from observations or evidence.
- `hint`: a suggestion or human steering input.

Core relation direction is defined individually:

- `spawns`: origin → goal.
- `derived_from`: origin/observation/conclusion → work_item. The arrow means the source motivated the destination.
- `yields`: work_item → observation/conclusion.
- `supports`: observation/conclusion → conclusion/goal.
- `depends_on`: dependent work_item → prerequisite work_item.

`supports` alone never changes goal state. Accepting a goal is a separate, attributable command. `depends_on` alone controls work eligibility; evidence lineage must not implicitly become an execution dependency.

Each node has immutable revisions and a current-revision pointer. Kind is immutable. Revisions hold title, state, typed payload, actor, and timestamp. Claims identify whether they are `observed`, `inferred`, or `reported`; this is separate from whether the host has accepted or rejected them.

Allowed states:

- origin: `recorded`.
- goal: `open`, `met`, `abandoned`, `needs_review`.
- work_item: `open`, `running`, `paused`, `done`, `blocked`, `exhausted`, `stopped`.
- observation/conclusion: `active`, `retracted`.
- hint: `active`, `consumed`.

Host policy controls who may invoke commands; the library controls valid mutations regardless of which adapter invokes them. Core work lifecycle permits open→running/stopped; running→paused/done/blocked/exhausted/stopped; paused→open/stopped; blocked/exhausted/stopped→open. A done work item remains done; further work gets a new work item. A retry resumes the same logical work item, while its execution attempt identity remains host-owned.

Goal lifecycle: creation→open; open/needs_review→met through AcceptGoal; open/met/needs_review→abandoned through AbandonGoal; met/abandoned/needs_review→open through ReopenGoal. Evidence invalidation or revision of a met goal causes met→needs_review. Abandoning or reopening a met goal first invalidates its current acceptance. Retracted evidence and consumed hints cannot be reactivated; create a new node or hint instead. Origin is immutable. Exactly one origin MUST exist per graph.

ReviseNode requires the expected current revision, preserves kind, and cannot directly change state. It may edit open/needs_review goals, met goal criteria (atomically moving to needs_review), open work items, active evidence, or active hints. Other states reject revision. Every successful node change appends exactly the next revision; revision sequences start at 1 and have no gaps. Current revision is always the maximum stored revision.

The support relation MUST be acyclic over node identities, even when links pin different revisions. Dependency cycles are checked independently over work-item identities. There is no requirement that the union of all relation types be a DAG. No self-links are permitted. A conclusion MUST start with at least one supporting observation/conclusion; an observation MUST have nonempty inline evidence or at least one resource reference. `reported` is an attribution label, not verification.

Required payloads are specified structurally in `snapshot.schema.json`. Domain-specific metadata lives under `extensions`; application-specific node kinds are deferred until an extension registration/versioning mechanism exists.

## External resources and evidence

Replace integer asset anchors with opaque resource references attached to node revisions. Each reference has a type, URI, and optional external revision and SHA-256 digest. Examples include a repository commit, test log, document section, ticket, or ARTEX asset ID encoded as a URI.

The core stores these identifiers and never dereferences a URI. A host resolver checks access and retrieves content. This removes foreign keys to ARTEX assets, findings, traffic captures, and filesystem paths without claiming that external references are automatically valid or authorized.

An evidence reference is not a node that must hold a copied artifact body. Record artifact provenance and integrity metadata, and leave blob storage and retention to the host.

## Storage contract

`001_initial.sql` defines seven tables in a dedicated `exploration_graph` PostgreSQL schema:

1. `graphs`: namespace, lifecycle, schema version, and current graph revision.
2. `nodes`: stable identity, kind, and current node revision.
3. `node_revisions`: immutable historical snapshots.
4. `edges`: typed links, with revision pinning for provenance.
5. `resource_refs`: external references attached to an exact node revision.
6. `goal_acceptances`: explicit acceptance decisions and the supporting edge IDs reviewed.
7. `graph_events`: one durable mutation envelope per committed graph revision, including the idempotency key and returned result.

Use application-generated UUIDs. Graph-local composite keys prevent accidental cross-graph links. Namespace is an ownership partition; it is not an authorization implementation. The host must authorize a namespace/graph operation before invoking the library and supply an attributable actor ID.

All provenance edges pin both endpoint revisions. Dependency edges pin neither and refer to current work-item state. Old provenance remains historical after a node changes; it is not silently rewritten to support the new version.

SQL provides key/reference integrity and basic shape checks. The command layer additionally validates endpoint kinds, state transitions, acyclicity of `depends_on`, evidence freshness, and JSON payload schemas. JSON Schema validation alone cannot perform those graph-wide checks.

## Command API

Each mutation takes `graph_id`, `request_id`, `actor_id`, command payload, and optional `expected_graph_revision`. Graph creation instead takes a caller-generated graph ID and namespace. The return contains committed graph revision and created/changed object IDs.

Minimum commands:

- `CreateGraph`: atomically create graph, origin node, and first event.
- `AddGoal`: create a goal and its origin link.
- `ProposeWork`: validate parent evidence, create the work item, and create provenance links in one transaction. With no parents, use the origin.
- `RecordObservation`: create a typed observation, link its producing work revision, and attach evidence references atomically.
- `RecordConclusion`: create a conclusion, its producer link if present, supporting links, and resource references atomically.
- `AddSupport`: link an active observation/conclusion revision to a conclusion or goal revision. Reject cycles in the support relation. Historical support edges are immutable; retract or revise the claim when evidence changes.
- `AddHint` and `ConsumeHint`: record and acknowledge steering information.
- `ReviseNode`: create a new immutable revision using the expected current node revision; restrict changes by kind and state. State transitions use their dedicated commands rather than arbitrary payload editing.
- `AddDependency` and `RemoveDependency`: operate on current work-item identities; reject cycles. The dependent work item must be open. Paused or retryable work must first return to open; dependencies of done work cannot change.
- `TransitionWork`: compare-and-set a valid state transition. Entering running requires every current prerequisite to be done. This transition is serialized with dependency edits.
- `AcceptGoal`: validate the goal revision and current, non-retracted supporting evidence; record who accepted it and why, then set the goal to met atomically. Acceptance stores the goal revision evaluated before the state transition and the new resulting revision.
- `AbandonGoal`, `ReopenGoal`, and `RetractEvidence`: explicit, audited operations. Reopening or evidence invalidation marks relevant acceptances stale.
- `AttachResource`: create an additional immutable reference to an existing node revision after host authorization.
- `SetGraphState`: active→closed, closed→active/archived. Closing requires no running work. Archived graphs are read-only. Other content mutations require active state; an identical committed request can still return its original result after closing.

Do not expose unrestricted node/edge CRUD as the primary public API. Internal storage primitives may be generic, but agent-facing tools call semantic commands so required lineage and invariants cannot be omitted.

RecordObservation requires a current running producer. RecordConclusion requires current, active supporting revisions; a producer is optional for planner synthesis and required for worker calls. ProposeWork accepts current, active evidence parents or the origin. For both commands, a conclusion parent must also have a recursively fresh support closure as defined under acceptance freshness. Stale evidence never becomes fresh merely because an adapter passes its ID without a revision. New work investigating stale claims can derive from the origin and reference those claims in its instruction; it must not present them as current support.

AddSupport requires the current revision of both endpoints. Sources must be active evidence; targets must be active conclusions or open/needs_review goals. Supporting links to a met goal cannot be appended without reopening it. An acceptance takes explicit direct support-edge IDs, not every available fact. Its reviewed closure and resource IDs are computed by the service under the mutation lock.

Duplicate identity-based links and same-state transitions return `already_exists` or `invalid_transition`; they do not create a new graph event. A retry of the original successful request returns its original result. ProposeWork does not perform fuzzy semantic deduplication; avoiding equivalent work is a planning responsibility.

## Transactions, concurrency, and event delivery

Start with one database row lock per graph mutation. This favors a simple correctness model for modest exploration graphs over maximum write throughput.

Within the transaction:

1. Lock the graph row and verify namespace. For creation, insert the row under the unique graph ID and handle a concurrent creation through the same idempotency contract.
2. Look up the request ID before checking the expected revision. If already committed with the same request hash, return the stored result; if reused with different input, return `idempotency_conflict`.
3. Verify lifecycle, expected graph/node revisions, and semantic preconditions.
4. Write the complete mutation, including related edges, evidence references, stale acceptances, and affected goal states.
5. Increase graph revision by one and insert its event/result envelope.
6. Commit before delivering any notification.

The hash covers a canonical encoding of command kind, actor, expected revisions, bound producer/execution identity when present, and payload. Timestamps and generated result IDs are excluded. IDs generated for a committed request are returned unchanged on retry. The adapter must retain the original bound context when retrying a committed invocation; rebinding its key to a new execution is an idempotency_conflict.

Events provide a reliable changes feed, not a promise that the complete database can be rebuilt by replaying them. Consumers request events after a graph revision, deduplicate by `(graph_id, revision)`, and rebuild from a consistent snapshot if history has expired. In v1 events are retained for the life of the graph. A future retention mechanism must expose a minimum retained revision and an explicit `resync_required` result.

External callbacks never run inside the database transaction. A failed notification does not roll back committed state; the consumer resumes from the event log. The library performs no shell commands or external effects, so its idempotency contract covers graph mutations only.

The host scheduler still owns attempts, worker leases, fencing, cancellation, and crash recovery. `TransitionWork` is an atomic graph-state operation, not a complete distributed job lease. Do not copy ARTEX's blanket startup reset of running intents into this library.

Entering running MUST store a host-generated UUID `execution_id` in the work payload. The service rejects an execution_id previously assigned to a running revision anywhere in that graph. Every worker write and worker completion carries that identity through trusted execution context. Under the graph lock, the service verifies that the work is running and its current execution_id matches; otherwise return `stale_execution`. The model cannot select the producer or execution identity. A stop/retry/new claim therefore prevents the old worker from appending graph output. This only fences graph writes; the host must separately fence external effects. When a worker crashes, the host explicitly transitions the old attempt to stopped or paused before retrying. Terminal/paused revisions retain the last execution_id for attribution; a new running transition replaces it with a new ID.

The host authenticates and authorizes every call, including idempotent retries, before service invocation. Authorization is not inferred from UUID secrecy or tool visibility. No network callbacks occur under the graph lock. Acceptance policy is evaluated by the host for the exact revision/evidence request; a concurrent mutation causes conflict or stale-evidence failure and requires reevaluation.

Stable error codes: `invalid_argument`, `not_found`, `permission_denied`, `revision_conflict`, `invalid_transition`, `invalid_relation`, `cycle_detected`, `stale_evidence`, `stale_execution`, `idempotency_conflict`, `already_exists`, `graph_read_only`, `budget_too_small`, `stale_cursor`, `limit_exceeded`, and `storage_unavailable`. Errors contain a safe message and structured details, never SQL or credentials. Only storage_unavailable is automatically retryable, with the same request identity; conflicts require rereading and a new decision/request.

CreateGraph uses INSERT ... ON CONFLICT DO NOTHING followed by a lock/read of the graph row in the transaction. An existing graph with the same committed creation request/hash returns its original result; any other creation targeting that identity fails with already_exists. Failed commands write no graph event and reserve no request key. Cancellation after a commit may leave the response undelivered; retry resolves the outcome from the stored result. Nested model-facing batches are excluded from the initial tool contract to avoid partial-success ambiguity.

## Evidence changes and acceptance freshness

Historical links remain readable. New revisions or retractions of evidence invalidate dependent acceptances using an explicit traversal of the accepted support paths, including intermediate conclusions. The affected goals move from met to needs_review in the same mutation transaction. A specification change to an accepted goal also requires review.

Rechecking may be performed by a human, deterministic validator, or agent under host policy. The core records the actor and method and checks the structure; it does not treat the name `supports` or a model-generated rationale as proof of truth.

An old acceptance's supports and evaluated goal revision remain queryable after invalidation. A new acceptance creates another record rather than overwriting history.

Acceptance captures the complete reviewed support-edge closure, including intermediate conclusions, and the exact reviewed resource-reference IDs; its direct edges target the evaluated goal revision. Its own transition to met does not invalidate that decision. Adding support or resource references later does not retroactively enlarge the reviewed set. Changes to any reviewed evidence revision invalidate the decision conservatively, even when the edit only changes wording.

To accept a goal, validate each selected direct edge targets its current open/needs_review revision. Recursively follow all incoming supports targeting each cited conclusion's exact revision. All evidence nodes in that closure MUST be active and their cited revisions MUST be current. A conclusion that cites an obsolete observation is stale even if its own revision has not changed. Reject such a closure with stale_evidence; the caller must revise/rebuild the conclusion and add fresh support links. Revisions never inherit support links implicitly. AttachResource only adds references; old references cannot be edited or deleted through the library.

An acceptance stores the deduplicated closure edge IDs and all resource references attached to the reviewed evidence revisions at decision time. It must include at least one direct goal support and no unrelated edges/references. To invalidate, find current acceptances whose captured closure cites the changed evidence revision, mark each stale with a reason, and append needs_review revisions for their met goals in the same transaction. Goal evaluated/resulting revisions must be consecutive with identical criteria and open/needs_review→met states. Import validates these relationships. A current acceptance exists if and only if the current goal state is met; exactly one such acceptance is allowed per goal. Stale acceptances remain historical and are not valid proof for new work.

## Read API and context projection

Expose `GetNode`, `GetRevision`, `ListNodes`, `Neighbors`, `Ancestors`, `Descendants`, `ExplainGoal`, `ListEligibleWork`, `Snapshot`, and `EventsAfter`. Every traversal names which relations it follows and provides depth/result limits. A dependency walk and a provenance walk have different semantics.

Provide `ProjectContext(graph_id, options)` as a separate deterministic module. The default includes current goals, eligible/running work, recent observations and conclusions, their supporting links, and hints. It returns totals, truncation flags, and retrieval cursors so omitted history is never mistaken for nonexistent history. A byte budget is required; hosts can additionally inject a token-count function. If required anchors cannot fit, return `budget_too_small` rather than dropping the objective silently.

All reads return the graph revision observed. An overview is read under a consistent database snapshot. Mandatory anchors are the origin summary, all non-abandoned goal IDs/states/criteria, and the worker's assigned item when present. Remaining budget is allocated to active hints, running work, eligible work by priority descending then creation time/ID ascending, and evidence by newest creation time/ID. Always include exact node revisions, total counts, omitted counts, and retrieval routes. Size means UTF-8 bytes of the complete serialized tool result; nodes are omitted whole rather than silently truncating their statements. If mandatory content exceeds budget, return budget_too_small with required_bytes.

List/traversal cursors bind graph ID, observed graph revision, query/filter hash, and stable sort position. A mutation between pages returns stale_cursor; the caller restarts the query. This first version promises no historical paginated view. EventsAfter is separately ordered by immutable graph revision and remains resumable across writes. Snapshot export runs in one repeatable-read transaction. HTTP/SSE subscription delivery is not part of the core contract.

The first version needs useful bounded projection and retrieval, not LLM summarization. In a later optional module, port hot/cold analysis and digest membership. Summarization should use a caller-supplied function; digests retain original member revisions and never replace evidence. A model call must occur outside the database lock and its result must be checked against source revisions before committing.

## Portable JSON snapshot

`snapshot.schema.json` defines a versioned snapshot with graphs, stable nodes containing their revision histories, edges, resource references, and acceptance records. Events are delivered separately and are not required for snapshot portability.

Export at a single database snapshot and include the graph revision. Import is an explicit restore operation into an empty graph identity; it is not ordinary mutation and does not silently merge. The importer must validate payloads, exactly one origin, unique identities, every foreign reference, current revision pointers, relation endpoint types, acceptance references, allowed states, and dependency/support cycles. Enable UUID, URI, and date-time format assertions in the JSON validator. Namespace authorization is checked by the host. Persist the snapshot atomically and write one import event; preserve node/evidence IDs, retain the original source revision as import metadata, and initialize the destination graph revision under the destination's own history.

Restore MUST reject running work in v1: no imported execution is allowed to acquire write authority accidentally. Stop/pause it in the source first. The caller supplies the authorized destination namespace and unused graph ID; all node/edge/reference/acceptance IDs and node revisions remain unchanged. Destination graph revision starts at 1 for the import event. Preserve source graph ID/revision under `metadata.import_source`; report any metadata name collision instead of overwriting it. Unknown schema_version values fail; no best-effort repair or silent dropping of records. Snapshots do not transfer request-id history, so retries from the source instance cannot be replayed against the restored instance.

## ARTEX adapter and migration

Map intent→work_item, fact→observation, finding→conclusion, and the origin-state fact→origin. Map `proves` to candidate support, but do not fabricate reviewed revision history or silently create a current acceptance from a legacy met flag. Import those goals as needs_review with legacy decision metadata until explicitly reaccepted. Preserve legacy confidence in a namespaced extension; map recognized values to epistemic_status and missing/unknown values to reported. Carry vulnerability metadata in a namespaced extension. Findings without supporting observations need an explicit reported observation with the legacy evidence reference, or a migration error; they cannot bypass the conclusion invariant.

Convert integer IDs through a durable old-ID/new-UUID mapping. Convert anchors to external references such as `artex://assets/123`. Activity, provider usage, transcripts, evidence bodies, and task lifecycle remain in ARTEX; keep external references to them where useful.

Cross-task inheritance is deferred in v1. The adapter can supply explicitly authorized external context; it must not silently copy mutable nodes or create foreign-graph edges. Digest nodes should be migrated as optional summary metadata or regenerated later while preserving original members. Do not discard members because they were folded in the UI.

First run the new library against synthetic fixtures. Then migrate a copied ARTEX dataset, compare lineage and overview projections, and adapt the existing tool handlers to call the library. Prefer a controlled import/cutover to indefinite dual writes, which would introduce another consistency problem. Any production cutover needs a backup, reconciliation report, and rollback procedure.

## Acceptance tests and definition of done

Required semantic and PostgreSQL integration tests:

- Invalid endpoint kinds, invalid states, and cross-graph links are rejected.
- Two concurrent claims of the same work item have exactly one winner.
- Concurrent dependency additions cannot create a cycle; claim and dependency edit cannot race eligibility.
- A repeated request returns its original result; reused keys with different payloads fail.
- Injected failures between node, edge, and event writes roll back the whole mutation.
- Negative observations remain queryable and are included appropriately in context projection.
- Editing/retracting supporting evidence invalidates direct and transitive goal acceptance.
- Retrying a graph write does not duplicate nodes, relationships, or events.
- Snapshot export/import preserves identities, history, references, and semantics.
- Pagination and bounded projection report omissions and retain retrieval paths.
- Namespace checks and host authorization hooks are exercised at adapters.
- Database tests run against an isolated database and fail, rather than skip, when required CI database configuration is missing.
- Old execution identities cannot write after stop/retry/reclaim, even if a caller reuses a node ID.
- Acceptance rejects a current conclusion backed by a stale observation revision.
- A lost response followed by retry returns the same IDs and committed revision, including after graph closure.
- Planner/worker/solo presets enforce the same authorization through in-process and MCP adapters.
- A fresh scripted session can resume from graph context alone and complete the fixture; external evidence is resolved only by the host.

Core done means a standalone module can construct an investigation, record evidence, propose derived work, accept and invalidate a goal, export/import it, and produce a bounded context view without ARTEX or Norma installed. Portable-tool done additionally requires the tool schemas, prompt fragments, role enforcement, in-process and MCP adapters, and equivalent scripted end-to-end fixtures. SQL and JSON contracts are versioned; migrations and behavioral fixtures ship with the module.

## Delivery scope and effort

The delivery gates and scope are defined in DEVELOPMENT-PLAN.md. A small ARTEX translation fixture is useful for conformance but is not a live migration. A planner, execution sandbox, full ARTEX UI, multi-tenant auth service, cross-graph inheritance, and model-generated digests are outside the portable-tool release.

Planning range for one experienced engineer: roughly 2–4 weeks for the core milestone, then 1–2 weeks for integration kit, MCP adapter, and conformance hardening. Estimate again after the first PostgreSQL vertical slice; these are scope-based planning ranges, not measured throughput or a completion promise. A live ARTEX migration and web UI are additional milestones.

ARTEX's checked-in license is AGPL-3.0. Decide the intended license and distribution model before directly extracting source; this specification does not establish that copied source can be relicensed.

## Source references

Inspected ARTEX commit: `7f8c7cbcae7324e823d8952371ae2013521cc711`.

- [Graph store and coupled models](https://github.com/BaaRai/ARTEX/blob/7f8c7cbcae7324e823d8952371ae2013521cc711/db/exploration.go)
- [Application schema](https://github.com/BaaRai/ARTEX/blob/7f8c7cbcae7324e823d8952371ae2013521cc711/db/schema.sql)
- [Tool-layer semantics and graph overview](https://github.com/BaaRai/ARTEX/blob/7f8c7cbcae7324e823d8952371ae2013521cc711/agent/tools.go)
- [Cross-task graph views](https://github.com/BaaRai/ARTEX/blob/7f8c7cbcae7324e823d8952371ae2013521cc711/db/exploration_sources.go)
- [Cold-graph algorithms](https://github.com/BaaRai/ARTEX/blob/7f8c7cbcae7324e823d8952371ae2013521cc711/agent/coldgraph.go)
- [Graph flow tests](https://github.com/BaaRai/ARTEX/blob/7f8c7cbcae7324e823d8952371ae2013521cc711/db/exploration_test.go)
- [License](https://github.com/BaaRai/ARTEX/blob/7f8c7cbcae7324e823d8952371ae2013521cc711/LICENSE)
