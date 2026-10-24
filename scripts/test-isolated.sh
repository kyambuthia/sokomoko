#!/usr/bin/env bash
# Run database-backed tests without touching a configured development database.
set -euo pipefail

container_id=""
cleanup() {
  if [[ -n "$container_id" ]]; then
    docker rm -fv "$container_id" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Use the returned ID for cleanup: a failed create must never remove an existing
# container. No host directory or named data volume is mounted.
container_id="$(docker run -d --rm \
  -p 127.0.0.1::5432 \
  -e POSTGRES_USER=sokomoko \
  -e POSTGRES_PASSWORD=isolated-test-only \
  -e POSTGRES_DB=sokomoko_test postgres:17-alpine)"

ready=false
for ((attempt = 0; attempt < 60; attempt++)); do
  if docker exec "$container_id" pg_isready -h 127.0.0.1 -U sokomoko -d sokomoko_test >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  printf '%s\n' 'Disposable PostgreSQL did not become ready.' >&2
  docker logs "$container_id" >&2
  exit 1
fi

mapping="$(docker port "$container_id" 5432/tcp)"
if [[ ! "$mapping" =~ ^127\.0\.0\.1:([0-9]+)$ ]]; then
  printf '%s\n' 'Expected a single loopback PostgreSQL port mapping.' >&2
  exit 1
fi
port="${BASH_REMATCH[1]}"
# Override both variables so no test helper can select an inherited database.
export DATABASE_URL="postgres://sokomoko:isolated-test-only@127.0.0.1:${port}/sokomoko_test?sslmode=disable"
export SOKOMOKO_TEST_DATABASE_URL="$DATABASE_URL"
export SOKOMOKO_REQUIRE_DB_TESTS=1

if [[ "$#" -eq 0 ]]; then
  set -- -race -count=1 ./...
fi
"${GO:-go}" test "$@"
