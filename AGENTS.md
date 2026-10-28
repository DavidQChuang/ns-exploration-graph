# Implementation collaboration rules

This repository implements the portable-tool release described in spec/SPEC.md and spec/TOOLS.md. Read those contracts before changing behavior; docs/README.md indexes the current guides and docs/maintainers/README.md indexes compatibility and release evidence. User instructions take precedence.

- Root coordinates, integrates, audits, and commits. Delegate implementation to tiered agents. Do not spawn further agents unless the root assigns capacity.
- Work only within your assigned paths. All agents share this checkout; do not reset, clean, stash, switch branches, or commit other agents' work. Root owns Git commits.
- Publish public interface decisions to root before dependent agents implement against them. Resolve contract changes explicitly in docs/DECISIONS.md.
- Never weaken requirements or tests to manufacture completion. PostgreSQL tests must actually run for release readiness; document any environment limitation accurately.
- Do not copy ARTEX implementation code. Implement the contracts independently. Do not publish to a remote/package registry without a separate user instruction.
- Keep credentials, local runtimes, logs from temporary databases, and generated executables out of Git.
- Each handoff reports owned files, actual verification commands/results, known gaps, and next steps. Root maintains TASKS.md, ACTION_LOG.md, and ORCHESTRATION.md.
