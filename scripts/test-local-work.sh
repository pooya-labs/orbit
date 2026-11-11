#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
ENV_FILE="$ROOT/.env.local-work"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing .env.local-work. Start Local Work first with scripts/start-local-work.sh." >&2
  exit 1
fi

HUMAN_TOKEN="$(awk -F= '$1 == "ORBIT_LOCAL_HUMAN_TOKEN" { print substr($0, index($0, "=") + 1); exit }' "$ENV_FILE")"
if [[ ! "$HUMAN_TOKEN" =~ ^[A-Za-z0-9+/=]{32,4096}$ ]]; then
  echo ".env.local-work does not contain a valid ORBIT_LOCAL_HUMAN_TOKEN." >&2
  exit 1
fi

echo "[1/5] Targeted Go tests and PostgreSQL Local Work bootstrap..."
docker compose --env-file "$ENV_FILE" -f docker-compose.yml -f docker-compose.local-work.yml --profile local-work-test run --rm local-work-test

echo "[2/5] Web TypeScript typecheck..."
npm run typecheck

echo "[3/5] Waiting for authenticated API runtime readiness..."
ready=0
for attempt in $(seq 1 60); do
  if curl --silent --show-error --fail http://127.0.0.1:8080/health/ready >/dev/null; then
    ready=1
    break
  fi
  sleep 2
done
if [[ "$ready" -ne 1 ]]; then
  echo "ORBIT API did not become ready at http://127.0.0.1:8080/health/ready." >&2
  exit 1
fi

echo "[4/5] Sending real telemetry through the authenticated local workload gateway..."
for attempt in 1 2 3; do
  docker compose --env-file "$ENV_FILE" -f docker-compose.yml -f docker-compose.local-work.yml --profile observability exec -T telemetry-demo wget -qO- http://127.0.0.1:8081/emit >/dev/null
  sleep 2
done

echo "[5/5] Verifying owner access, seeded workspace data, and all three tenant-scoped telemetry reads..."
ORBIT_LOCAL_HUMAN_TOKEN="$HUMAN_TOKEN" node --input-type=module <<'NODE'
const origin = "http://127.0.0.1:8080";
const token = process.env.ORBIT_LOCAL_HUMAN_TOKEN;
if (!token || token.length < 32) throw new Error("Local owner token is missing or invalid.");

async function get(path, authenticated = true) {
  const headers = authenticated ? { Authorization: "Bearer " + token } : {};
  const response = await fetch(origin + path, { headers, cache: "no-store" });
  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    const detail = payload?.error?.message || JSON.stringify(payload);
    throw new Error("GET " + path + " returned HTTP " + response.status + ": " + detail);
  }
  return payload;
}

const anonymous = await fetch(origin + "/api/v1/workspaces", { cache: "no-store" });
if (![401, 403].includes(anonymous.status)) {
  throw new Error("The workspace API did not reject an unauthenticated request.");
}

const workspaces = (await get("/api/v1/workspaces")).data;
if (!Array.isArray(workspaces)) throw new Error("Workspace response is not an array.");
const workspace = workspaces.find((item) => item.slug === "orbit-local-work");
if (!workspace?.id) throw new Error("The local owner cannot access orbit-local-work.");

const membership = await get("/api/v1/workspaces/" + encodeURIComponent(workspace.id) + "/membership");
if (!membership?.data) throw new Error("The local workspace membership response is missing.");

const base = "/api/v1/workspaces/" + encodeURIComponent(workspace.id);
const services = (await get(base + "/services")).data;
const incidents = (await get(base + "/incidents")).data;
if (!Array.isArray(services) || services.length < 6) {
  throw new Error("Expected at least six local workspace services.");
}
if (!Array.isArray(incidents) || incidents.length !== 2) {
  throw new Error("Expected exactly two clearly marked starter incidents.");
}

const queries = {
  metrics: base + "/telemetry/metrics?window=1h&service=orbit-telemetry-demo&metric=request_rate&group_by=none",
  logs: base + "/telemetry/logs?window=1h&service=orbit-telemetry-demo&limit=20",
  traces: base + "/telemetry/traces?window=1h&service=orbit-telemetry-demo&limit=20"
};

const startedAt = Date.now();
const deadlineMs = 90_000;
let latest = {};
while (Date.now() - startedAt < deadlineMs) {
  const names = Object.keys(queries);
  const results = await Promise.allSettled(names.map((name) => get(queries[name])));
  latest = {};
  results.forEach((result, index) => {
    latest[names[index]] = result.status === "fulfilled"
      ? { count: Array.isArray(result.value?.data) ? result.value.data.length : -1 }
      : { error: result.reason instanceof Error ? result.reason.message : String(result.reason) };
  });

  if (names.every((name) => latest[name].count > 0)) {
    console.log(
      "Local Work passed: authenticated owner, " + services.length + " services, " +
      incidents.length + " incidents; Metrics=" + latest.metrics.count +
      ", Logs=" + latest.logs.count + ", Traces=" + latest.traces.count + "."
    );
    process.exit(0);
  }
  await new Promise((resolve) => setTimeout(resolve, 3000));
}

console.error("The workspace-scoped telemetry did not become queryable before the smoke-test deadline.");
console.error(JSON.stringify(latest, null, 2));
process.exit(1);
NODE
