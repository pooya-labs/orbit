#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
command -v docker >/dev/null || { echo "Docker is required." >&2; exit 1; }
command -v node >/dev/null || { echo "Node.js 22+ is required." >&2; exit 1; }
command -v npm >/dev/null || { echo "npm is required." >&2; exit 1; }
docker info >/dev/null || { echo "Start Docker Desktop or the Docker daemon first." >&2; exit 1; }

if [[ ! -f .env.local-work ]]; then
  umask 077
  printf 'ORBIT_LOCAL_HUMAN_TOKEN=%s\n' "$(openssl rand -base64 48 | tr -d '\n')" > .env.local-work
  printf 'ORBIT_LOCAL_WORKLOAD_TOKEN=%s\n' "$(openssl rand -base64 48 | tr -d '\n')" >> .env.local-work
  chmod 600 .env.local-work
  echo "Created private .env.local-work credentials. Do not share this file."
fi

docker compose --env-file .env.local-work -f docker-compose.yml -f docker-compose.local-work.yml --profile observability up -d --build postgres api prometheus loki tempo otel-collector telemetry-demo
export NEXT_PUBLIC_ORBIT_LOCAL_WORK=true
export NEXT_PUBLIC_ORBIT_API_URL=http://localhost:8080
export NEXT_PUBLIC_ORBIT_BASE_PATH=""
[[ -d node_modules ]] || npm ci
echo "ORBIT Local Work: http://localhost:3000"
echo "API readiness: http://localhost:8080/health/ready"
echo "Sign in with ORBIT_LOCAL_HUMAN_TOKEN from .env.local-work."
npm run dev -- --hostname 127.0.0.1
