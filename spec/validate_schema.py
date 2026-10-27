"""Validate the proposed JSON contract; does not test SQL or graph semantics.

Requires jsonschema. Run: python validate_schema.py
Writes example.snapshot.json beside this script.
"""
import copy
import json
from pathlib import Path
from jsonschema import Draft202012Validator, FormatChecker

ROOT = Path(__file__).resolve().parent
schema = json.loads((ROOT / "snapshot.schema.json").read_text())
Draft202012Validator.check_schema(schema)
validator = Draft202012Validator(schema, format_checker=FormatChecker())
when = "2026-10-26T12:00:00Z"


def uid(number):
    return f"00000000-0000-4000-8000-{number:012d}"


def node(number, kind, states, payload):
    return dict(id=uid(number), kind=kind, current_revision=len(states),
                created_at=when, revisions=[
                    dict(revision=i, state=state, title=kind, payload=payload,
                         actor_id="example:harness", created_at=when)
                    for i, state in enumerate(states, 1)])


def edge(number, source, source_revision, target, target_revision, relation):
    return dict(id=uid(number), source_id=uid(source), source_revision=source_revision,
                target_id=uid(target), target_revision=target_revision,
                relation=relation, actor_id="example:harness", created_at=when)


sample = dict(
    schema_version=1,
    graph=dict(id=uid(1000), namespace="example", description="Verify a parser fix",
               state="active", revision=8, metadata={}, created_at=when, updated_at=when),
    nodes=[
        node(1, "origin", ["recorded"], {"request": "Verify the parser fix."}),
        node(2, "goal", ["open", "met"], {"acceptance_criteria": ["Regression test passes."]}),
        node(3, "work_item", ["open", "running", "done"],
             {"instruction": "Run the parser regression test.", "priority": 50}),
        node(4, "observation", ["active"],
             {"statement": "The regression test passed.", "epistemic_status": "observed", "polarity": "positive"}),
    ],
    edges=[edge(101, 1, 1, 2, 1, "spawns"), edge(102, 1, 1, 3, 1, "derived_from"),
           edge(103, 3, 2, 4, 1, "yields"), edge(104, 4, 1, 2, 1, "supports")],
    resource_refs=[dict(id=uid(201), node_id=uid(4), node_revision=1,
                        resource_type="test_log", uri="artifact://example/parser-test/1",
                        actor_id="example:harness", created_at=when)],
    acceptances=[dict(id=uid(301), goal_id=uid(2), evaluated_revision=1, resulting_revision=2,
                      support_edge_ids=[uid(104)], reviewed_resource_ids=[uid(201)],
                      actor_id="example:validator", method="validator",
                      rationale="The referenced regression test satisfies the criterion.",
                      state="current", created_at=when)],
)
sample["nodes"][2]["revisions"][1]["payload"] = {
    **sample["nodes"][2]["revisions"][1]["payload"], "execution_id": uid(401)
}
sample["nodes"][2]["revisions"][2]["payload"] = {
    **sample["nodes"][2]["revisions"][2]["payload"], "execution_id": uid(401)
}
validator.validate(sample)
(ROOT / "example.snapshot.json").write_text(json.dumps(sample, indent=2) + "\n", encoding="utf-8")

cases = {
    "unknown kind": lambda d: d["nodes"][0].update(kind="unknown"),
    "invalid state for origin": lambda d: d["nodes"][0]["revisions"][0].update(state="running"),
    "missing epistemic label": lambda d: d["nodes"][3]["revisions"][0]["payload"].pop("epistemic_status"),
    "revision-pinned dependency": lambda d: d["edges"][0].update(relation="depends_on"),
    "unpinned provenance": lambda d: d["edges"][0].pop("source_revision"),
    "stale acceptance without reason": lambda d: d["acceptances"][0].update(state="stale"),
    "invalid UUID": lambda d: d["graph"].update(id="invalid"),
    "invalid date": lambda d: d["graph"].update(created_at="yesterday"),
    "invalid URI": lambda d: d["resource_refs"][0].update(uri="not a URI"),
    "unknown envelope field": lambda d: d.update(unexpected=True),
    "empty nodes": lambda d: d.update(nodes=[]),
    "running without execution identity": lambda d: d["nodes"][2]["revisions"][1]["payload"].pop("execution_id"),
}
for name, mutate in cases.items():
    invalid = copy.deepcopy(sample)
    mutate(invalid)
    if validator.is_valid(invalid):
        raise AssertionError(f"Unexpectedly accepted: {name}")
print(f"Schema valid; example accepted; {len(cases)} invalid variants rejected.")
print("Graph-wide semantic checks and PostgreSQL execution are not covered.")

manifest = json.loads((ROOT / "tool-contracts.json").read_text())
reference = {"id": uid(4), "revision": 1}
tool_examples = {
    "graph_overview": {"byte_budget": 16384},
    "graph_get_node": {"node_id": uid(4)},
    "graph_list_nodes": {"kind": "observation", "limit": 20},
    "graph_propose_work": {"title": "Check fix", "instruction": "Run regression test", "priority": 50, "parents": [reference]},
    "graph_record_observation": {"title": "Test result", "statement": "Regression passed", "evidence": "test output: PASS", "epistemic_status": "observed"},
    "graph_record_conclusion": {"title": "Fix verified", "statement": "The criterion is satisfied", "epistemic_status": "inferred", "supports": [reference]},
    "graph_add_support": {"source": reference, "target": {"id": uid(2), "revision": 1}},
    "graph_accept_goal": {"goal": {"id": uid(2), "revision": 1}, "support_edge_ids": [uid(104)], "rationale": "Regression passed"},
}
tool_checks = 0
for tool in manifest["tools"]:
    Draft202012Validator.check_schema(tool["inputSchema"])
    check = Draft202012Validator(tool["inputSchema"], format_checker=FormatChecker())
    example = tool_examples[tool["name"]]
    check.validate(example)
    for forbidden in ("graph_id", "actor_id", "execution_id", "work_item_id"):
        if check.is_valid({**example, forbidden: uid(999)}):
            raise AssertionError(f"Tool accepts model-controlled host context: {tool['name']}/{forbidden}")
        tool_checks += 1
    for required in tool["inputSchema"]["required"]:
        invalid = copy.deepcopy(example)
        del invalid[required]
        if check.is_valid(invalid):
            raise AssertionError(f"Missing required field accepted: {tool['name']}/{required}")
        tool_checks += 1
tools_by_name = {t["name"]: t for t in manifest["tools"]}
assert len(tools_by_name) == len(manifest["tools"]) == 8
for role, names in manifest["role_presets"].items():
    assert len(names) == len(set(names))
    assert set(names) <= set(tools_by_name)
    assert "graph_accept_goal" not in names
assert "graph_propose_work" not in manifest["role_presets"]["worker"]
assert manifest["capability_gated_tools"]["accept_goal"] == ["graph_accept_goal"]
print(f"8 tool input schemas/examples valid; {tool_checks} invalid tool inputs rejected; role manifests checked.")
print("These checks validate contracts, not runtime authorization or transport behavior.")
