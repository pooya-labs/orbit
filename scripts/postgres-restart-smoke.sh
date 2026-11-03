#!/usr/bin/env bash
set -euo pipefail

control_dir="${ORBIT_POSTGRES_RESTART_CONTROL_DIR:-${RUNNER_TEMP:-/tmp}/orbit-postgres-restart}"
if [[ "$control_dir" != /* ]]; then
  echo "ORBIT_POSTGRES_RESTART_CONTROL_DIR must be an absolute path" >&2
  exit 2
fi
export ORBIT_POSTGRES_RESTART_CONTROL_DIR="$control_dir"

container_name="orbit-postgres-restart-smoke-${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-1}"
mkdir -p "$control_dir"
rm -f "$control_dir"/ready "$control_dir"/postgres-stopped "$control_dir"/outage-detected "$control_dir"/postgres-restarted "$control_dir"/recovered

cleanup() {
  local status=$?
  trap - EXIT
  # Never leave the shared observability stack without its database, even if
  # the test fails while PostgreSQL is stopped.
  docker compose --profile observability start postgres >/dev/null 2>&1 || true
  if docker inspect "$container_name" >/dev/null 2>&1; then
    if [[ "$status" -ne 0 ]]; then
      docker logs "$container_name" >&2 || true
    fi
    docker rm -f "$container_name" >/dev/null 2>&1 || true
  fi
  rm -rf "$control_dir"
  exit "$status"
}
trap cleanup EXIT

docker compose --profile observability up -d postgres
ready=0
for _ in $(seq 1 120); do
  if docker compose --profile observability exec -T postgres pg_isready -U orbit -d orbit >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.25
done
if [[ "$ready" -ne 1 ]]; then
  echo "PostgreSQL did not become ready before the restart test" >&2
  exit 1
fi

docker rm -f "$container_name" >/dev/null 2>&1 || true
docker compose --profile observability run --detach --no-deps --name "$container_name" postgres-restart-smoke

wait_for_marker() {
  local marker="$1"
  local attempts="$2"
  for _ in $(seq 1 "$attempts"); do
    if [[ -f "$control_dir/$marker" ]]; then
      return 0
    fi
    if ! docker inspect "$container_name" >/dev/null 2>&1; then
      echo "PostgreSQL restart test container exited before marker '$marker'" >&2
      docker logs "$container_name" >&2 || true
      return 1
    fi
    sleep 0.25
  done
  echo "Timed out waiting for PostgreSQL restart marker '$marker'" >&2
  docker logs "$container_name" >&2 || true
  return 1
}

wait_for_marker ready 120
docker compose --profile observability stop postgres
touch "$control_dir/postgres-stopped"

wait_for_marker outage-detected 120
docker compose --profile observability start postgres
touch "$control_dir/postgres-restarted"

wait_for_marker recovered 180
exit_code="$(docker wait "$container_name")"
docker logs "$container_name"
if [[ "$exit_code" != "0" ]]; then
  echo "PostgreSQL container restart integration test exited with $exit_code" >&2
  exit "$exit_code"
fi
