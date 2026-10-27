# Orchestration checkpoint

Repository: C:\Users\dc\Downloads\crowncards_breach_writeup_10-25-26\ns-exploration-graph

Root role: coordinator, integrator, release auditor, and Git committer. Implementation is delegated.

## Active assignments

Orchestrator handoff on 2026-10-27: Cursor agent (Claude) replaced the stopped Codex session. Earlier Codex agents are not running.

- `core_perf` (agent bd7b6b3f) — claude-opus-5-5-medium; owns graph/**, bounded-read performance, G3 projection/cursor/restore tests, docs/DECISIONS.md (append) and docs/POSTGRES-VERIFICATION.md.
- `toolkit_mcp` (agent 5f884d15) — claude-sonnet-5-5-high; owns toolkit/, adapters/mcp/, cmd/, examples/, result schemas in spec/, README.md, docs/INTEGRATION.md, .github/workflows/test.yml.
- Completed (previous session) `requirements_docs` — delivered docs/REQUIREMENTS.md, docs/RELEASE-CHECKLIST.md, LICENSE, NOTICE. Verification status remains NOT VERIFIED until runtime evidence is recorded.
- Planned after both lanes finish: independent reviewer (correctness/security) and release-audit lane to fill docs/RELEASE-CHECKLIST.md evidence.

All agents share the checkout. Root stages named paths only and records commits at coherent boundaries. No agent may overwrite another lane's work or silently change the public contract.

## Recovery

Read this file, TASKS.md, ACTION_LOG.md, docs/DECISIONS.md, actual Git status/log, and live agent state. Do not restart already active assignments. Run commands from WSL with the Windows toolchain (see ACTION_LOG handoff entry); PostgreSQL on port 55439 must be inspected before any restart. The old Codex heartbeat `resume-ns-exploration-graph-implementation` and its usage-reset logic are not carried over. Do not mark incomplete work complete.

Completion requires evidence for every M1/M2 gate; no blanket production-ready assertion without the audit.
