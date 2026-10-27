# Architecture decisions

## 001: Shared semantic aggregate transactions (approved by integrator)

The local Go module is `ns-exploration-graph`. The graph package owns semantic commands. Stores expose a detached graph aggregate inside one serializable graph mutation callback. PostgreSQL locks the graph row, loads the aggregate, invokes the shared service, and persists normalized table differences and an event in one transaction. Callback failure rolls back all writes. In-memory storage is a reference adapter, not an alternative semantic implementation.

This deliberately prioritizes correctness for modest graphs over write throughput. Aggregate mutation cost grows with graph history; 100/1,000/10,000-node benchmarks document practical bounds. Read snapshots exclude event envelopes; bounded EventsAfter is separate. All store calls require context and namespace. Namespace checks are partition checks, while authorization belongs to host adapters.

Public snapshots mirror the versioned JSON contract. Typed payload structs encode known fields; revision payloads remain raw JSON for immutable storage and strict kind-specific validation. Result data uses a JSON object so storage can preserve exact committed responses on retry.
