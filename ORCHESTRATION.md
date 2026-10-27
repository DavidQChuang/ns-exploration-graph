# Orchestration checkpoint

Repository: C:\Users\dc\Downloads\crowncards_breach_writeup_10-25-26\ns-exploration-graph

Root role: coordinator, integrator, release auditor, and Git committer. Implementation is delegated.

## Assignment plan

- Architecture/core lead: strong reasoning model; owns Go domain contracts, semantic service, memory store, and core tests.
- Infrastructure/storage lane: workhorse model; owns local toolchain provisioning, PostgreSQL adapter/migrations, DB tests, and CI.
- Documentation/conformance lane: lighter model initially; owns requirement inventory and operational/release documentation. Later replace with integration lane when interfaces stabilize.

All agents share the checkout. Root stages named paths only and records commits at coherent boundaries. No agent may overwrite another lane's work or silently change the public contract.

## Recovery

Read this file, TASKS.md, ACTION_LOG.md, docs/DECISIONS.md, actual Git status/log, and live agent state. Do not restart already active assignments. Check usage before expensive stages. At >=90% five-hour usage (or a limiting weekly allowance), finish/checkpoint the current bounded unit, stop new assignments, and defer until reset. A thread heartbeat checks for safe resumption hourly at minute 7. It does not spend reset credits or purchase capacity. If a goal has been explicitly paused for the usage wait, user-authorized recovery still must follow app goal-state capabilities; do not mark incomplete work complete.

Completion requires evidence for every M1/M2 gate; no blanket production-ready assertion without the audit.
