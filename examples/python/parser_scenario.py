#!/usr/bin/env python3
"""Second consumer: the parser-failure scenario over MCP stdio, with no Go API.

This script speaks MCP JSON-RPC (newline-delimited, stdio) to the compiled
`graph-mcp` binary using only the Python 3 standard library. Host-only steps that
the eight model tools deliberately do not expose (create graph, claim work, finish
work) use the compiled `scripted host ...` control CLI. Both binaries are built from
this repository; see examples/python/README.md.

Required: EXPLORATION_DATABASE_URL in the environment (a PostgreSQL URL).

  python parser_scenario.py --graph-mcp ./graph-mcp --host ./scripted

Stdout is one JSON document: the normalized state report built from persisted graph
state by `scripted host report`. The transcript goes to stderr.
"""
import argparse
import json
import os
import queue
import subprocess
import sys
import threading
import uuid

PROTOCOL_VERSION = "2025-11-25"  # server also supports 2026-07-28; see docs/INTEGRATION.md
MUTATING = {"graph_propose_work", "graph_record_observation", "graph_record_conclusion",
            "graph_add_support", "graph_accept_goal"}
LOG = "fixture integer parser: malformed input rejected"
RESOURCE_URI = "fixture://parser/rejection-log"
RATIONALE = "Host reviewed the stored parser rejection log."


class ProtocolError(Exception):
    """The transport or JSON-RPC layer failed (not a tool-level error)."""


class AmbiguousWriteError(ProtocolError):
    """A mutating call may or may not have committed.

    This client has no durable invocation identity: the server cannot recognize a
    repeat of the same logical write. Do not retry; read the graph and reconcile.
    """


class Session:
    """Minimal MCP stdio client: one graph-mcp subprocess, strictly sequential calls."""

    def __init__(self, argv, timeout=30.0):
        self.timeout = timeout
        self.proc = subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=subprocess.PIPE, text=True, encoding="utf-8", bufsize=1)
        self.lines = queue.Queue()
        self.next_id = 0
        threading.Thread(target=self._pump, daemon=True).start()
        self.stderr = []
        threading.Thread(target=lambda: self.stderr.append(self.proc.stderr.read()), daemon=True).start()
        result = self.request("initialize", {"protocolVersion": PROTOCOL_VERSION, "capabilities": {},
                                             "clientInfo": {"name": "python-stdlib-consumer", "version": "1"}})
        self.server = result
        self.notify("notifications/initialized")

    def _pump(self):
        for line in self.proc.stdout:
            self.lines.put(line)
        self.lines.put(None)  # EOF: server exited or closed stdout

    def _send(self, message):
        try:
            self.proc.stdin.write(json.dumps(message, separators=(",", ":")) + "\n")
            self.proc.stdin.flush()
        except (BrokenPipeError, OSError, ValueError) as e:
            raise ProtocolError("connection lost while sending: %s" % e)

    def notify(self, method, params=None):
        message = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            message["params"] = params
        self._send(message)

    def _read(self):
        try:
            line = self.lines.get(timeout=self.timeout)
        except queue.Empty:
            raise ProtocolError("timed out waiting for the server")
        if line is None:
            raise ProtocolError("server closed the connection%s" % (": " + "".join(self.stderr).strip() if self.stderr else ""))
        return json.loads(line)

    def request(self, method, params=None, drop_response=False):
        self.next_id += 1
        rid = self.next_id
        message = {"jsonrpc": "2.0", "id": rid, "method": method}
        if params is not None:
            message["params"] = params
        self._send(message)
        while True:
            reply = self._read()
            if "method" in reply:  # server-initiated request or notification
                if "id" in reply:
                    self._send({"jsonrpc": "2.0", "id": reply["id"], "result": {}})
                continue
            if reply.get("id") != rid:
                continue
            if drop_response:  # drill: pretend the network lost this answer
                raise AmbiguousWriteError("response to %s was lost" % method)
            if "error" in reply:
                raise ProtocolError("JSON-RPC error %s" % json.dumps(reply["error"]))
            return reply["result"]

    def list_tools(self):
        return {t["name"]: t for t in self.request("tools/list", {})["tools"]}

    def call(self, name, arguments, drop_response=False):
        """Call a tool and return its structured envelope {ok, graph_revision, data | error}."""
        try:
            result = self.request("tools/call", {"name": name, "arguments": arguments}, drop_response)
        except AmbiguousWriteError:
            raise
        except ProtocolError as e:
            if name in MUTATING:
                raise AmbiguousWriteError("%s: outcome unknown (%s)" % (name, e))
            raise
        envelope = result["structuredContent"]
        # Transport success is not command success: check both the flag and the envelope.
        if result.get("isError") == envelope["ok"]:
            raise ProtocolError("isError disagrees with envelope.ok")
        if json.loads(result["content"][0]["text"]) != envelope:
            raise ProtocolError("structured and text content disagree")
        return envelope

    def ok(self, name, arguments):
        envelope = self.call(name, arguments)
        if not envelope["ok"]:
            raise AssertionError("%s failed: %s" % (name, json.dumps(envelope["error"])))
        return envelope["data"]

    def close(self):
        """Lifecycle shutdown: close stdin, expect the server to exit 0 by itself."""
        try:
            self.proc.stdin.close()
        except OSError:
            pass
        try:
            code = self.proc.wait(timeout=15)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            raise AssertionError("graph-mcp did not exit after stdin closed")
        if code != 0:
            raise AssertionError("graph-mcp exited with %s after a clean close" % code)


def say(message):
    print(message, file=sys.stderr, flush=True)


def host(args, *command):
    out = subprocess.run([args.host, "host"] + list(command), check=True, capture_output=True,
                         text=True, encoding="utf-8").stdout
    return json.loads(out)


def launch(args, graph_id, role, *extra):
    argv = [args.graph_mcp, "-namespace", args.namespace, "-graph", graph_id,
            "-actor", "python-agent", "-role", role] + list(extra)
    return Session(argv)


def find_nodes(session, kind):
    page = session.ok("graph_list_nodes", {"kind": kind, "limit": 100})
    return page["nodes"]


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--graph-mcp", required=True, help="path to the compiled graph-mcp binary")
    p.add_argument("--host", required=True, help="path to the compiled scripted binary (host control CLI)")
    p.add_argument("--namespace", default="python-" + uuid.uuid4().hex[:12])
    p.add_argument("--no-drill", action="store_true", help="skip the lost-response drill")
    args = p.parse_args()
    if not os.environ.get("EXPLORATION_DATABASE_URL"):
        sys.exit("EXPLORATION_DATABASE_URL is required")

    # --- host: create the graph and goal (the model tools cannot do this)
    created = host(args, "init", "-namespace", args.namespace)
    graph_id = created["graph_id"]
    say("host: graph %s in namespace %s" % (graph_id, args.namespace))

    # --- planner session: discovery, validation behaviour, propose work
    planner = launch(args, graph_id, "planner")
    names = set(planner.list_tools())
    assert names == {"graph_overview", "graph_get_node", "graph_list_nodes", "graph_propose_work",
                     "graph_record_conclusion", "graph_add_support"}, names
    assert all("outputSchema" in t for t in planner.list_tools().values())
    planner.ok("graph_overview", {})
    bad = planner.call("graph_propose_work", {"title": "t", "instruction": "i", "priority": 1, "graph_id": graph_id})
    assert not bad["ok"] and bad["error"]["code"] == "invalid_argument", bad
    hidden = planner.call("graph_record_observation", {"title": "t", "statement": "s", "evidence": "e", "epistemic_status": "observed"})
    assert not hidden["ok"] and hidden["error"]["code"] == "permission_denied", hidden
    work = planner.ok("graph_propose_work", {
        "title": "Reproduce malformed input",
        "instruction": "Run integer parser with twelve; record the negative result.", "priority": 50})
    planner.close()
    work_id = work["work_item_id"]
    say("planner: proposed work %s" % work_id)

    # --- host: claim the work under a new execution identity
    claim = host(args, "claim", "-namespace", args.namespace, "-graph", graph_id, "-work", work_id, "-revision", "1")
    execution = claim["execution_id"]

    # --- worker: produce the real negative result, then record it
    try:
        int("twelve")
        raise AssertionError("parser unexpectedly accepted malformed input")
    except ValueError:
        pass
    resolver = {RESOURCE_URI: LOG}  # the host/consumer owns evidence retrieval, not the graph
    observation = {
        "title": "Malformed input rejection observed", "statement": "The parser rejected twelve.",
        "evidence": LOG, "epistemic_status": "observed", "polarity": "negative",
        "resource_refs": [{"resource_type": "test_log", "uri": RESOURCE_URI}]}
    worker = launch(args, graph_id, "worker", "-work", work_id, "-execution", execution)
    assert len(worker.list_tools()) == 5
    spoof = worker.call("graph_record_observation", dict(observation, execution_id=str(uuid.uuid4())))
    assert not spoof["ok"] and spoof["error"]["code"] == "invalid_argument", spoof
    obs = None
    if not args.no_drill:
        try:
            worker.call("graph_record_observation", observation, drop_response=True)
            raise AssertionError("drill did not lose the response")
        except AmbiguousWriteError as e:
            say("worker: ambiguous write (%s); NOT retrying, reconciling by reading" % e)
        worker.close()  # the 'connection' is gone; open a new session with the same binding
        worker = launch(args, graph_id, "worker", "-work", work_id, "-execution", execution)
        existing = [n for n in find_nodes(worker, "observation") if n["title"] == observation["title"]]
        if len(existing) == 1:
            obs = {"observation_id": existing[0]["id"], "node_revision": existing[0]["revision"]}
            say("worker: reconciled; the write had committed, no retry needed")
        else:
            assert not existing, "duplicate observations before retry"
    if obs is None:
        obs = worker.ok("graph_record_observation", observation)
    conclusion = worker.ok("graph_record_conclusion", {
        "title": "Parser rejection verified", "statement": "Malformed text is rejected by the integer parser.",
        "epistemic_status": "inferred", "supports": [{"id": obs["observation_id"], "revision": obs["node_revision"]}]})
    say("worker: recorded conclusion %s" % conclusion["conclusion_id"])

    # --- host: finish the work; the old execution is now fenced
    host(args, "finish", "-namespace", args.namespace, "-graph", graph_id, "-work", work_id, "-revision", "2", "-state", "done")
    late = worker.call("graph_record_observation", dict(observation, title="late"))
    assert not late["ok"] and late["error"]["code"] == "stale_execution", late
    worker.close()

    # --- fresh planner session with an explicit acceptance grant: knows only the graph
    planner = launch(args, graph_id, "planner", "-acceptance", "validator")
    assert "graph_accept_goal" in planner.list_tools()
    planner.ok("graph_overview", {})
    goal = [n for n in find_nodes(planner, "goal")][0]
    concl = [n for n in find_nodes(planner, "conclusion")][0]
    node = planner.ok("graph_get_node", {"node_id": concl["id"], "revision": concl["revision"]})
    verified = False
    for edge in node["edges"]:
        if edge["relation"] == "supports" and edge["target_id"] == concl["id"]:
            source = planner.ok("graph_get_node", {"node_id": edge["source_id"], "revision": edge["source_revision"]})
            for resource in source["resource_refs"]:
                verified = verified or resolver.get(resource["uri"]) == LOG
    assert verified, "host could not resolve the reviewed evidence"
    support = planner.ok("graph_add_support", {"source": {"id": concl["id"], "revision": concl["revision"]},
                                               "target": {"id": goal["id"], "revision": goal["revision"]}})
    accepted = planner.ok("graph_accept_goal", {"goal": {"id": goal["id"], "revision": goal["revision"]},
                                                "support_edge_ids": [support["edge_id"]], "rationale": RATIONALE})
    assert len(accepted["reviewed_evidence"]) == 2 and len(accepted["reviewed_resource_ids"]) == 1, accepted
    planner.close()
    say("planner: goal accepted (acceptance %s)" % accepted["acceptance_id"])

    print(json.dumps(host(args, "report", "-namespace", args.namespace, "-graph", graph_id), indent=2))


if __name__ == "__main__":
    main()
