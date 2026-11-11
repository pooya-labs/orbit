# ORBIT Local Work edition

Local Work runs the real ORBIT Go API, PostgreSQL-backed workspace data, the authenticated OTLP gateways, OpenTelemetry Collector, Prometheus, Loki, Tempo, and the existing dashboard on one computer. It is separate from the public GitHub Pages / Apps Script portfolio demo.

This edition requires no VM, hosted database, external identity provider, Cloudflare account, domain, or paid service. It is a single-user local runtime, not a multi-user internet deployment.

## Requirements

- Windows 10/11, macOS, or Linux
- Docker Desktop (or Docker Engine) with the Compose plugin
- Node.js 22+ and npm
- A first start needs internet access so Docker images and Go/Node dependencies can be downloaded

## Start

From the repository root:

**Windows PowerShell**

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-local-work.ps1
```

**macOS / Linux**

```bash
chmod +x scripts/start-local-work.sh
./scripts/start-local-work.sh
```

The launcher validates Node.js 22+, Docker Engine, and the Compose plugin before starting. It validates the two separate local credentials, checks the Compose configuration, and repairs missing or inconsistent web dependencies with `npm ci` **before** starting any containers. After starting the backend, it waits up to two minutes for the API readiness endpoint and prints recent container diagnostics if the API does not become ready. The dashboard starts on the fixed loopback address `127.0.0.1:3000`; a port conflict is reported rather than silently switching ports.

Open **http://localhost:3000**. In the Local Work sign-in field, paste `ORBIT_LOCAL_HUMAN_TOKEN` from `.env.local-work`. The browser keeps this key in tab-scoped session storage only; it is not part of the frontend build and is not saved to local storage.

The API is available at `http://localhost:8080`; readiness is `/health/ready`.

## What is enabled

| Area | Local Work behaviour |
| --- | --- |
| Overview and services | PostgreSQL-backed workspace data, plus a live local telemetry workload |
| Incidents | Create, transition, reopen, and inspect immutable timelines |
| Metrics, logs, traces | Real OTLP writes through the authenticated local gateway and workspace-scoped queries against Prometheus, Loki, and Tempo |
| Alert rules | Persisted rules, background evaluation, and alert-instance history |
| Notifications | Persisted webhook channels, retry worker, and delivery/attempt history when a webhook secret is configured |
| Deployments | Append-only annotations and release comparisons |
| SLOs | Persisted objectives, request-success measurements, and error budgets |
| Audit | Workspace audit events with pseudonymous actor references |

The initial workspace is `orbit-local-work`. It includes five clearly marked `demo_seed` scenario services and incidents, plus the `orbit-telemetry-demo` service. The latter is an instrumented Go workload that sends actual metrics, logs, and traces. Synthetic scenario records are not presented as live telemetry.

## Local security boundary

- The web UI listens on `127.0.0.1:3000`; the API is published only on `127.0.0.1:8080`.
- PostgreSQL is published only on loopback. Redis and NATS are not started because the current application does not depend on them.
- The OTLP HTTP/gRPC gateways and raw Collector receivers are not published to the host. The telemetry demo reaches the authenticated gateway over the private Docker network.
- Human and workload credentials are separate, randomly generated values. The workload key is passed only to the telemetry container and never to browser code.
- Local static-token verification is accepted only with the explicit `ORBIT_LOCAL_WORK=true` development configuration. It is not an OIDC replacement and must never be enabled on a public interface, staging, or production.
- `.env.local-work` is ignored by Git. Keep it private. Never paste either token into an issue, commit, screenshot, or public log.

## Webhook notifications

The notification UI, persistence, and delivery worker are included. Outbound delivery needs a destination you control. Add an HTTPS webhook secret to `.env.local-work`, for example:

```dotenv
ORBIT_ALERT_WEBHOOK_ALLOWED_HOSTS=hooks.example.com
ORBIT_ALERT_WEBHOOK_SECRET_OPS_PRIMARY=https://hooks.example.com/services/your/private/path
```

Use the actual hostname and webhook URL supplied by your provider. The hostname must be listed exactly, and the URL must use HTTPS on port 443. Restart the API after changing the file. Do not put a real endpoint in this documentation or source control. Without a configured secret, channel configuration/history remains available but outbound delivery is intentionally inactive.

## Stop and persistence

Stop the dashboard with Ctrl+C. Stop the backend containers from the repository root:

```powershell
docker compose --env-file .env.local-work -f docker-compose.yml -f docker-compose.local-work.yml --profile observability down
```

This preserves PostgreSQL and telemetry data in named volumes. To reset the local database and telemetry history completely (destructive):

```powershell
docker compose --env-file .env.local-work -f docker-compose.yml -f docker-compose.local-work.yml --profile observability down -v
```

Keep `.env.local-work` when restarting the same local database. Deleting it generates new credentials on the next launch; the local owner identity remains the same.

## Fast test plan

The fast Local Work test profile is deliberately narrower than the release gate. It checks local token verification, configuration guardrails, API authorization, workload authorization, PostgreSQL bootstrap, workspace scoping, and bootstrap idempotency. Go module and build caches persist in named Docker volumes.

After starting Local Work, run on Windows:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\test-local-work.ps1
```

The script runs the targeted Go tests, web TypeScript typecheck, API readiness, and authenticated workspace/service/incident smoke checks.

On macOS/Linux, run the equivalent checks from the repository root:

```bash
bash scripts/test-local-work.sh
```

Both platform test runners verify the targeted Go/PostgreSQL profile, the local owner session and seeded workspace, then send requests through the authenticated workload gateway and wait until workspace-scoped Metrics, Logs, and Traces queries return real records. The CI workflow also runs this Local Work overlay as a separate end-to-end job on the exact pull-request head.

For the final release candidate, CI remains responsible for the full Go race suite, vet/build, web production build, authenticated OTLP write-to-query isolation across all three signals, backpressure/recovery, PostgreSQL restart recovery, and Security scans. Those broad suites are run at the release gate, not after every documentation or styling edit. The Local Work branch must be green on the exact PR head before a release is published.
