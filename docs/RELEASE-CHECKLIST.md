# Release checklist

Applies to the standalone exploration graph portable-tool release (M1 + M2). The optional M3 viewer is outside this checklist. Every item is initially **NOT VERIFIED**. Record commands, environment/version, result, and artifact path. Do not check an item based on a draft schema, mocked database, or unexecuted command.

## Contract and clean environment

- [ ] **NOT VERIFIED** — Contract version and compatibility decisions are approved, documented, and reflected in schemas, migrations, result/error contracts, fixtures, and release notes. Resolve open questions in `REQUIREMENTS.md` before depending on them.
- [ ] **NOT VERIFIED** — A clean supported environment can fetch/build/install the module and start the in-process example and stdio MCP adapter using documented instructions; no ARTEX/Norma runtime, local developer files, or unstated service is needed.
- [ ] **NOT VERIFIED** — Supported Go, PostgreSQL, MCP protocol, and SDK versions are declared and pinned/ranged as appropriate; version checks and compatibility expectations are documented.
- [ ] **NOT VERIFIED** — CI validates JSON schemas with UUID, URI, and date-time format assertions; validates examples; detects contract drift; and runs the documented release commands.

## M1 core and real PostgreSQL verification

- [ ] **NOT VERIFIED** — Shared semantic suite passes against both memory and PostgreSQL adapters.
- [ ] **NOT VERIFIED** — PostgreSQL integration suite ran against an actual isolated supported PostgreSQL database. CI fails rather than skips if database configuration is missing. Record server version, test command, and full pass/fail summary.
- [ ] **NOT VERIFIED** — Restart test demonstrates durable graph, revisions, references, events, and idempotency results.
- [ ] **NOT VERIFIED** — Concurrent same-work claims have one winner; concurrent dependency additions cannot create a cycle; claim racing a dependency edit cannot make running work ineligible. Record repeated/race test commands and results.
- [ ] **NOT VERIFIED** — Old execution IDs cannot write after stop/retry/reclaim; trusted producer/execution binding and host authorization are tested.
- [ ] **NOT VERIFIED** — Injected failure between related writes rolls back all graph data/event/result; retry after lost response returns the exact original IDs and graph revision, including after graph closure.
- [ ] **NOT VERIFIED** — Freshness tests cover stale intermediate evidence, transitive acceptance invalidation, unrelated goals, goal criteria changes, explicit direct support selection, and preserved historical acceptance.
- [ ] **NOT VERIFIED** — Projection/cursor tests cover serialized UTF-8 byte boundaries, required anchors, omissions/retrieval routes, `budget_too_small`, stale cursors, pagination, and EventsAfter recovery across writes.
- [ ] **NOT VERIFIED** — Snapshot tests cover repeatable export, identity/history-preserving round trip, revision/state/relationship validation, unsupported versions, invalid references/cycles, metadata collision, running-work rejection, atomic failure, and destination event/revision semantics.
- [ ] **NOT VERIFIED** — Stable error codes, safe details, retryability, limits, and “no partial writes” behavior pass boundary and contract tests.
- [ ] **NOT VERIFIED** — Declared-hardware benchmarks include 100, 1,000, and 10,000 nodes and report overview/traversal/invalidation latency, serialized bytes, query count, and memory. Investigate unbounded whole-history reads before sign-off.

## M2 tool and protocol conformance

- [ ] **NOT VERIFIED** — All eight tool names, strict schemas, unknown-field rejection, bounds, inputs, result fields, and stable error envelopes match TOOLS/tool-contracts contracts.
- [ ] **NOT VERIFIED** — Planner/worker/solo grants and opt-in acceptance are tested in both adapters; direct forbidden-tool calls, spoofed actor/graph/producer/execution values, invented IDs, and unauthorized idempotent retries are rejected.
- [ ] **NOT VERIFIED** — In-process deterministic scripted fixture passes, including negative evidence, fresh-session resume, evidence retrieval, and acceptance under explicit host policy.
- [ ] **NOT VERIFIED** — MCP client tests cover discovery, valid/invalid calls, structured tool errors, lifecycle shutdown, disconnect, lost response, and ambiguous-write recovery. Protocol/SDK versions and test client are recorded.
- [ ] **NOT VERIFIED** — In-process and MCP scripted state snapshots compare equal after normalizing generated IDs/timestamps; nodes, relations, revisions, lifecycle, and acceptance closure agree.
- [ ] **NOT VERIFIED** — Shared, worker, planner, and finish/retry prompt assets are versioned and checked for required guidance; test evidence confirms prompts are not treated as enforcement.
- [ ] **NOT VERIFIED** — A second consumer outside the direct Go API completes documented setup and a scripted end-to-end operation.

## Dependencies, licensing, and package hygiene

- [ ] **NOT VERIFIED** — Dependency inventory is reviewed; vulnerability and license checks run for direct and transitive dependencies and release artifacts. Record tool/version, database/driver licenses, findings, and disposition.
- [ ] **NOT VERIFIED** — Intended project license and distribution model are documented. Any ARTEX-derived code is separately reviewed for provenance and license compatibility; behavioral inspiration alone is not represented as copied implementation.
- [ ] **NOT VERIFIED** — Release contains only intended source, schemas, migrations, fixtures/examples, documentation, and packaged binaries if applicable. No credentials, local runtimes, temporary database logs/data, generated executables, or debug artifacts are present.
- [ ] **NOT VERIFIED** — Clean checkout status is reviewed; ignored local artifacts and secrets are checked separately; release archive contents are enumerated and inspected.

## Compatibility and sign-off

- [ ] **NOT VERIFIED** — Public Go API, JSON snapshot/tool contracts, SQL migration/versioning, error codes, and MCP behavior have explicit compatibility guarantees and deprecation/versioning policy. Breaking changes require an intentional contract version and migration notes.
- [ ] **NOT VERIFIED** — Import/export compatibility is tested across the declared supported snapshot versions; unknown versions fail clearly without repair or partial import.
- [ ] **NOT VERIFIED** — Release notes state verified functionality and remaining limits; do not claim transparent exactly-once behavior for hosts without durable invocation identity, automatic resource authorization/resolution, external-effect fencing, ARTEX migration, or M3 viewer delivery.
- [ ] **NOT VERIFIED** — Reviewer records gate evidence for M1 and M2, open risks, compatibility decision, and release artifact checksums/location.

## Evidence record

| Field | Release record |
|---|---|
| Contract/release version | MISSING |
| Commit/revision | MISSING |
| Clean setup command and environment | MISSING |
| PostgreSQL version and test command/results | MISSING |
| Race/concurrency command/results | MISSING |
| Protocol/SDK versions and conformance results | MISSING |
| Dependency/license scan and disposition | MISSING |
| Compatibility/release notes | MISSING |
| Artifact inventory/checksum | MISSING |
| Reviewer and date | MISSING |

