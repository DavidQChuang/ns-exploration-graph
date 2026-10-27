#!/usr/bin/env bash
# Shared by hosted CI and the isolated local release check. Requires a disposable DB.
set -euo pipefail
cd "$(dirname "$0")/.."
: "${GRAPH_TEST_DATABASE_URL:?Set GRAPH_TEST_DATABASE_URL to a dedicated PostgreSQL database}"
export GRAPH_REQUIRE_POSTGRES=1 GRAPH_REQUIRE_PYTHON=1 CGO_ENABLED=1 GOTOOLCHAIN=local
export EXPLORATION_DATABASE_URL="$GRAPH_TEST_DATABASE_URL"
python="${GRAPH_PYTHON:-python3}"
"$python" -c 'import sys; assert sys.version_info >= (3, 8)'
command -v "${CC:-gcc}" >/dev/null
go version
"$python" --version
"${CC:-gcc}" --version | head -n 1
go env GOOS GOARCH CGO_ENABLED GOTOOLCHAIN

evidence="${GRAPH_EVIDENCE_DIR:-.local/release-evidence}"
mkdir -p "$evidence" bin
evidence="$(cd "$evidence" && pwd)"
go mod download
go mod verify
"$python" toolkit/generate_contracts.py --check
go vet ./...
go build ./...
go install ./cmd/graph-mcp
go build -o bin/graph-mcp ./cmd/graph-mcp
go build -o bin/scripted ./examples/scripted
go test -race -json -count=1 ./... | tee "$evidence/race-tests.jsonl"
"$python" - "$evidence/race-tests.jsonl" <<'PY'
import collections, json, sys
events = [json.loads(line) for line in open(sys.argv[1], encoding="utf-8")]
counts = collections.Counter(e["Action"] for e in events if e.get("Test"))
packages = [e["Package"] for e in events if not e.get("Test") and e["Action"] == "pass"]
print("Race suite:", counts["pass"], "test/subtest passes;", counts["fail"], "failures;", counts["skip"], "skips;", len(packages), "packages passed")
if counts["fail"] or counts["skip"] or not counts["pass"]:
    raise SystemExit("Release verification requires actual tests and zero skips/failures")
PY
go test -count=1 -v -run 'Stdio|PythonStdio' ./cmd/graph-mcp ./examples/scripted | tee "$evidence/stdio-tests.log"
go run ./examples/scripted -transport direct > "$evidence/direct.json"
go run ./examples/scripted -transport mcp > "$evidence/mcp.json"
"$python" examples/python/parser_scenario.py --graph-mcp bin/graph-mcp --host bin/scripted > "$evidence/python.json" 2> "$evidence/python-transcript.log"
"$python" - "$evidence" <<'PY'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1])
reports = [json.loads((root / name).read_text()) for name in ("direct.json", "mcp.json", "python.json")]
assert reports[0] == reports[1] == reports[2], "Documented example transports disagree"
assert reports[0]["goal_state"] == "met" and reports[0]["resumed_from_graph"]
print("Documented direct, in-process MCP, and Python stdio reports agree")
PY

# The minimum compiler is deliberately old: scan reachable vulnerabilities with the
# patched compiler lane so old standard-library advisories do not hide module findings.
if [[ "${GRAPH_SCAN_VULNERABILITIES:-1}" == 1 ]]; then
  go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
  vulnerability_bin="$(go env GOBIN)"
  if [[ -z "$vulnerability_bin" ]]; then vulnerability_bin="$(go env GOPATH)/bin"; fi
  "$vulnerability_bin/govulncheck" ./... | tee "$evidence/govulncheck.log"
fi
