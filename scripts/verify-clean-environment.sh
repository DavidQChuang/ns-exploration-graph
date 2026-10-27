#!/usr/bin/env bash
# Linux amd64 + Docker; no published ports, host service changes, or developer caches.
set -euo pipefail
repo="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo"
revision="$(git rev-parse --verify "${1:-HEAD}^{commit}")"
version="${2:-1.25.0}"
case "$version" in
  1.25.0) checksum=2852af0cb20a13139b3448992e69b868e50ed0f8a1e5940ee1de9e19a123b613; scan=0 ;;
  1.27.3) checksum=65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40; scan=1 ;;
  *) echo 'Supported verification compilers: 1.25.0 and 1.27.3' >&2; exit 2 ;;
esac
run_id="$(date -u +%Y%m%dT%H%M%SZ)-${revision:0:12}-go$version-$$"
run_dir="$repo/.local/clean-environment/$run_id"
mkdir -p "$run_dir/source" "$run_dir/evidence" "$run_dir/tool-context"
git archive --format=tar "$revision" > "$run_dir/source.tar"
sha256sum "$run_dir/source.tar" | tee "$run_dir/source.sha256"
tar -xf "$run_dir/source.tar" -C "$run_dir/source"
test -f "$run_dir/source/scripts/verify-release.sh"
test -f "$run_dir/source/scripts/clean-environment.Dockerfile"
printf 'Revision: %s\nCompiler: %s\nGo archive SHA256: %s\n' "$revision" "$version" "$checksum" > "$run_dir/environment.txt"
docker version >> "$run_dir/environment.txt"
image="ns-graph-release-tools:go$version"
cp "$run_dir/source/scripts/clean-environment.Dockerfile" "$run_dir/tool-context/Dockerfile"
docker build --build-arg "GO_VERSION=$version" --build-arg "GO_SHA256=$checksum" -t "$image" "$run_dir/tool-context" 2>&1 | tee "$run_dir/provision.log"
pg_image='postgres:17.11@sha256:b8faeff7712663c2773a190181e1246e37e364b887611c04658c28b44e2408fe'
docker pull "$pg_image" >> "$run_dir/provision.log" 2>&1
network="ns-graph-$run_id"
database="ns-graph-pg-$run_id"
runner="ns-graph-run-$run_id"
cleanup() {
  result=$?
  trap - EXIT
  docker logs "$database" > "$run_dir/postgres.log" 2>&1 || true
  docker cp "$runner:/src/.local/release-evidence/." "$run_dir/evidence" 2>/dev/null || true
  docker rm -f -v "$runner" "$database" > /dev/null 2>&1 || true
  docker network rm "$network" > /dev/null 2>&1 || true
  printf 'Exit: %s\nEvidence: %s\n' "$result" "$run_dir"
  exit "$result"
}
trap cleanup EXIT
docker network create "$network" > /dev/null
docker run -d --name "$database" --network "$network" --network-alias postgres \
  -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=exploration_graph_test \
  "$pg_image" > /dev/null
ready=0
for attempt in {1..60}; do
  if docker exec "$database" pg_isready -U postgres -d exploration_graph_test > /dev/null 2>&1; then ready=1; break; fi
  sleep 1
done
[[ "$ready" == 1 ]] || { echo 'Isolated PostgreSQL did not become ready' >&2; exit 1; }
docker exec "$database" psql -U postgres -d exploration_graph_test -Atc 'select version()' >> "$run_dir/environment.txt"
docker run -d --name "$runner" --network "$network" \
  -e CI=1 -e GRAPH_TEST_DATABASE_URL='postgres://postgres:postgres@postgres:5432/exploration_graph_test?sslmode=disable' \
  -e GRAPH_REQUIRE_POSTGRES=1 -e GRAPH_REQUIRE_PYTHON=1 -e "GRAPH_SCAN_VULNERABILITIES=$scan" \
  "$image" sleep infinity > /dev/null
docker cp "$run_dir/source/." "$runner:/src"
docker exec "$runner" bash -c 'go version; python3 --version; gcc --version | head -n 1; go env GOTOOLCHAIN GOCACHE GOMODCACHE; test ! -e /isolated/modcache; test ! -e /isolated/buildcache' >> "$run_dir/environment.txt"
docker exec "$runner" bash scripts/verify-release.sh 2>&1 | tee "$run_dir/verification.log"
