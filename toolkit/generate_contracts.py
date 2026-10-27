"""Generate output schemas and the embedded manifest from versioned contracts.

Run from any working directory: python toolkit/generate_contracts.py
Use --check to verify, without writing, that spec/tool-contracts.json and
toolkit/contracts.json are what this generator produces, ignoring CRLF/LF checkout differences (CI/drift gate).
Uses only the Python standard library. Go tests assert byte parity afterward.
"""
import copy
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
manifest = json.loads((ROOT / "spec/tool-contracts.json").read_text())
# Contract approval is release metadata, applied by the source generator so the
# checked-in spec and embedded manifest cannot disagree about its status.
manifest["status"] = "approved"
snap = json.loads((ROOT / "spec/snapshot.schema.json").read_text())
defs = {k: copy.deepcopy(v) for k, v in snap["$defs"].items()
        if k in ("id", "text", "time", "version", "revision", "edge", "resource",
                 "originPayload", "goalPayload", "workPayload", "evidencePayload", "hintPayload")}
def ref(name): return {"$ref": "#/$defs/" + name}
def obj(properties, required=None):
    return {"type": "object", "additionalProperties": False, "properties": properties,
            "required": list(properties) if required is None else required}
def arr(items): return {"type": "array", "items": items}
def integer(): return {"type": "integer", "minimum": 0}
ID, VER, TXT = ref("id"), ref("version"), ref("text")
defs["ref"] = obj({"id": ID, "revision": VER})
kinds = ["origin", "goal", "work_item", "observation", "conclusion", "hint"]
states = {"origin": ["recorded"], "goal": ["open", "met", "abandoned", "needs_review"],
          "work_item": ["open", "running", "paused", "done", "blocked", "exhausted", "stopped"],
          "observation": ["active", "retracted"], "conclusion": ["active", "retracted"],
          "hint": ["active", "consumed"]}
payloads = dict(zip(kinds, ["originPayload", "goalPayload", "workPayload", "evidencePayload", "evidencePayload", "hintPayload"]))
fresh = {"enum": ["fresh", "stale", "not_applicable"]}
summary = obj({"id": ID, "kind": {"enum": kinds}, "revision": VER, "state": TXT, "title": TXT, "freshness": fresh})
summary["allOf"] = [{"if": {"properties": {"kind": {"const": k}}}, "then": {"properties": {"state": {"enum": states[k]}}}} for k in kinds]
defs["summary"] = summary
context = copy.deepcopy(summary)
context["properties"]["payload"] = {"type": "object"}
context["required"].append("payload")
for cond, k in zip(context["allOf"], kinds): cond["then"]["properties"]["payload"] = ref(payloads[k])
defs["context_node"] = context
node = obj({"id": ID, "kind": {"enum": kinds}, "current_revision": VER, "node": ref("revision"),
            "freshness": fresh, "edges": arr(ref("edge")), "resource_refs": arr(ref("resource")),
            "total_related": integer(), "omitted_related": integer(), "next_related_cursor": TXT},
           ["id", "kind", "current_revision", "node", "freshness", "edges", "resource_refs", "total_related", "omitted_related"])
node["allOf"] = [{"if": {"properties": {"kind": {"const": k}}},
                  "then": {"properties": {"node": {"properties": {"state": {"enum": states[k]}, "payload": ref(payloads[k])}}}}} for k in kinds]
counts = obj({k: integer() for k in kinds + ["edges"]})
outputs = {
 "graph_overview": obj({"origin": ref("context_node"), "goals": arr(ref("context_node")), "work": arr(ref("context_node")),
                        "evidence": arr(ref("context_node")), "hints": arr(ref("context_node")), "edges": arr(ref("edge")),
                        "totals": counts, "omitted": counts, "truncated": {"type": "boolean"},
                        "retrieval": obj({"get_node": {"const": "graph_get_node"}, "list_nodes": {"const": "graph_list_nodes"}, "snapshot": {"const": "host_snapshot"}})}),
 "graph_get_node": node,
 "graph_list_nodes": obj({"nodes": arr(ref("summary")), "total": integer(), "omitted": integer(), "next_cursor": TXT}, ["nodes", "total", "omitted"]),
 "graph_propose_work": obj({"work_item_id": ID, "node_revision": VER, "lineage_edge_ids": arr(ID)}),
 "graph_record_observation": obj({"observation_id": ID, "node_revision": VER, "yields_edge_id": ID, "resource_reference_ids": arr(ID)}),
 "graph_record_conclusion": obj({"conclusion_id": ID, "node_revision": VER, "support_edge_ids": arr(ID), "resource_reference_ids": arr(ID), "yields_edge_id": ID}, ["conclusion_id", "node_revision", "support_edge_ids", "resource_reference_ids"]),
 "graph_add_support": obj({"edge_id": ID}),
 "graph_accept_goal": obj({"acceptance_id": ID, "resulting_goal_revision": VER, "reviewed_evidence": arr(ref("ref")), "support_edge_ids": arr(ID), "reviewed_resource_ids": arr(ID)})}
error_codes = ["invalid_argument", "not_found", "permission_denied", "revision_conflict", "invalid_transition", "invalid_relation", "cycle_detected", "stale_evidence", "stale_execution", "idempotency_conflict", "already_exists", "graph_read_only", "budget_too_small", "stale_cursor", "limit_exceeded", "storage_unavailable"]
failure = obj({"ok": {"const": False}, "error": obj({"code": {"enum": error_codes}, "message": TXT, "retryable": {"type": "boolean"}, "details": {"type": "object"}}, ["code", "message", "retryable"])})
for tool in manifest["tools"]:
    tool["outputSchema"] = {"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
                            "oneOf": [obj({"ok": {"const": True}, "graph_revision": VER, "data": outputs[tool["name"]]}), failure], "$defs": defs}
manifest["description"] = "Transport-neutral strict model input and result envelope schemas. Trusted context is host-owned."
encoded = json.dumps(manifest, indent=2) + "\n"
targets = [ROOT / "spec/tool-contracts.json", ROOT / "toolkit/contracts.json"]
if "--check" in sys.argv[1:]:
    drift = [str(t.relative_to(ROOT)) for t in targets if t.read_bytes().decode("utf-8").replace("\r\n", "\n") != encoded]
    if drift:
        sys.exit("contract drift: run python toolkit/generate_contracts.py; differs: " + ", ".join(drift))
    print("contracts match generator output: " + ", ".join(str(t.relative_to(ROOT)) for t in targets))
else:
    for t in targets:
        newline = "\r\n" if b"\r\n" in t.read_bytes() else "\n"
        t.write_bytes(encoded.replace("\n", newline).encode("utf-8"))
