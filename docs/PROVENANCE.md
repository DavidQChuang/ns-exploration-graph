# Source provenance review

Review scope: tracked source and documentation at `HEAD 93e054b`, with the repository's pinned behavioral reference identified in `NOTICE` and `spec/SPEC.md` as ARTEX commit `7f8c7cbcae7324e823d8952371ae2013521cc711`. This records a bounded engineering review, not a legal opinion, originality certification, or guarantee that no protectable expression was copied.

## Evidence reviewed

- `README.md`, `NOTICE`, `spec/SPEC.md`, `spec/TOOLS.md`, and `AGENTS.md` describe behavioral study as design input and explicitly prohibit copying ARTEX implementation code.
- Tracked Go, SQL, schema, and fixture files were inspected by path and representative source. The implementation is organized around the project's own `graph` command service, `State`/`Service` abstractions, normalized PostgreSQL store, JSON contracts, and conformance tests. Repository search found no ARTEX package imports, ARTEX-specific implementation paths, or imported Norma runtime/service; README explicitly says no ARTEX/Norma code or service is used.
- Search for `ARTEX` in tracked files found contextual references, project statements, and links to the pinned reference in the specification/notice; it did not reveal vendored ARTEX files. Common domain terms such as graph, node, edge, goal, evidence, and work are not treated as evidence of copying by themselves.
- The local `../artex-architecture-source` Git checkout was verified at the pinned ARTEX commit `7f8c7cbcae7324e823d8952371ae2013521cc711`. A bounded exact-line comparison covered all 363 upstream and all 41 project tracked `.go`/`.sql` files at project commit `93e054b`. It normalized whitespace, discarded blank/comment lines, and compared exact non-comment lines at least 70 characters long; it found two matches, both generic `context.WithTimeout(context.Background(), duration)` setup lines in `cmd/graph-mcp/stdio_test.go` matching test/server files upstream. It found no exact matching five-line runs. This simple scan is evidence only; it does not test semantic similarity, modified/collapsed code, documentation, or independently authored behavior. The 79-file count refers to the tracked tree at the time of this review, before the new documents and script are added to Git's index; the inventory script reports only tracked files and must be rerun after these deliverables are tracked.

## Assessment and limits

The internal evidence and bounded source comparison are consistent with a separately structured implementation. No ARTEX code artifact/import or substantial exact source match was identified; the two exact-line matches are generic timeout setup in tests. This does not prove independent authorship or settle whether behavior, architecture, tests, terminology, or any particular material raises a legal issue, and it is not legal review. The comparison method and limits should remain part of the release record; obtain appropriate legal review before external distribution if needed.

