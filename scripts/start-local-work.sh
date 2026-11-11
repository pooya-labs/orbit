#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

command -v docker >/dev/null || { echo "Docker is required. Install Docker Desktop or Docker Engine." >&2; exit 1; }
command -v node >/dev/null || { echo "Node.js 22+ is required." >&2; exit 1; }
command -v npm >/dev/null || { echo "npm is required." >&2; exit 1; }
command -v openssl >/dev/null || { echo "OpenSSL is required to generate local credentials." >&2; exit 1; }

NODE_MAJOR="$(node -p 'Number(process.versions.node.split(".")[0])')"
if [[ ! "$NODE_MAJOR" =~ ^[0-9]+$ ]] || (( NODE_MAJOR < 22 )); then
  echo "Node.js 22+ is required; detected $(node --version). Upgrade Node.js and rerun this script." >&2
  exit 1
fi

docker info >/dev/null 2>&1 || { echo "Start Docker Desktop or the Docker daemon first." >&2; exit 1; }
docker compose version >/dev/null 2>&1 || { echo "Docker Compose is unavailable. Install the Docker Compose plugin." >&2; exit 1; }

ENV_FILE="$ROOT/.env.local-work"
if [[ ! -f "$ENV_FILE" ]]; then
  umask 077
  printf 'ORBIT_LOCAL_HUMAN_TOKEN=%s\n' "$(openssl rand -base64 48 | tr -d '\n')" > "$ENV_FILE"
  printf 'ORBIT_LOCAL_WORKLOAD_TOKEN=%s\n' "$(openssl rand -base64 48 | tr -d '\n')" >> "$ENV_FILE"
  echo "Created private .env.local-work credentials. Do not share this file."
fi
chmod 600 "$ENV_FILE"

read_env_value() {
  awk -F= -v key="$1" '$1 == key { sub(/^[^=]*=/, ""); print; found=1; exit } END { if (!found) exit 1 }' "$ENV_FILE"
}
HUMAN_TOKEN="$(read_env_value ORBIT_LOCAL_HUMAN_TOKEN || true)"
WORKLOAD_TOKEN="$(read_env_value ORBIT_LOCAL_WORKLOAD_TOKEN || true)"
if (( ${#HUMAN_TOKEN} < 32 || ${#WORKLOAD_TOKEN} < 32 )); then
  echo ".env.local-work must contain two credentials of at least 32 characters. Back up the file, repair both ORBIT_LOCAL_*_TOKEN entries, and rerun this script." >&2
  exit 1
fi
if [[ "$HUMAN_TOKEN" == "$WORKLOAD_TOKEN" ]]; then
  echo "The local human and workload credentials must be different. Replace one token in .env.local-work and rerun this script." >&2
  exit 1
fi

COMPOSE=(docker compose --env-file "$ENV_FILE" -f docker-compose.yml -f docker-compose.local-work.yml --profile observability)
"${COMPOSE[@]}" config --quiet

if ! npm ls --depth=0 >/dev/null 2>&1; then
  echo "Installing web dependencies from the committed lockfile before starting Docker services..."
  npm ci --no-audit --no-fund
fi

echo "Starting PostgreSQL, ORBIT API, Collector, Prometheus, Loki, Tempo, and the authenticated telemetry demo..."
"${COMPOSE[@]}" up -d --build postgres api prometheus loki tempo otel-collector telemetry-demo

echo "Waiting for the local API to become ready..."
ready=0
for ((attempt = 1; attempt <= 60; attempt++)); do
  if node -e 'fetch("http://127.0.0.1:8080/health/ready", { signal: AbortSignal.timeout(1500) }).then((response) => process.exit(response.ok ? 0 : 1)).catch(() => process.exit(1))' >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 2
done
if [[ "$ready" -ne 1 ]]; then
  echo "Local Work did not become ready. Recent container status and logs:" >&2
  "${COMPOSE[@]}" ps --all || true
  "${COMPOSE[@]}" logs --no-color --tail=100 api postgres otel-collector prometheus loki tempo || true
  echo "The API did not become ready within 120 seconds. Review the diagnostics above and rerun the launcher." >&2
  exit 1
fi

export NEXT_PUBLIC_ORBIT_LOCAL_WORK=true
export NEXT_PUBLIC_ORBIT_API_URL=http://localhost:8080
export NEXT_PUBLIC_ORBIT_BASE_PATH=""

echo "ORBIT Local Work: http://localhost:3000"
echo "API readiness: http://localhost:8080/health/ready"
echo "Sign in with ORBIT_LOCAL_HUMAN_TOKEN from .env.local-work."
echo "Telemetry workload credentials remain inside the private Docker network."
echo "Press Ctrl+C to stop the web UI; use the documented compose down command to stop the backend."

npm run dev -- --hostname 127.0.0.1 --port 3000
