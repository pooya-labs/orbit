# ORBIT Local Work v1.1.0

ORBIT Local Work is a separate single-user edition that runs the full observability runtime on one computer. It does not replace or modify the public GitHub Pages / Apps Script portfolio demo released as v1.0.0.

## Included

- The ORBIT Go API with PostgreSQL-backed workspace, incident, alert, notification, deployment, SLO, and audit workflows.
- Separate local human and telemetry-workload credentials, with constant-time verification and explicit development-only configuration.
- Authenticated OTLP/HTTP and OTLP/gRPC ingestion through the workspace authorization gateway.
- OpenTelemetry Collector, Prometheus, Loki, and Tempo for real local metrics, logs, and traces.
- An instrumented Go telemetry workload and clearly marked starter scenarios.
- The existing dashboard connected to the local API after tab-scoped owner sign-in.
- Windows PowerShell and macOS/Linux launchers, targeted tests, and an end-to-end Local Work CI job.

## Start

Requirements: Windows 10/11, macOS, or Linux; Docker Desktop or Docker Engine with the Compose plugin; Node.js 22+ and npm. The first start needs internet access to retrieve container images and dependencies.

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-local-work.ps1
```

macOS / Linux:

```bash
chmod +x scripts/start-local-work.sh
./scripts/start-local-work.sh
```

Open http://localhost:3000 and sign in with `ORBIT_LOCAL_HUMAN_TOKEN` from the generated, Git-ignored `.env.local-work` file. The API readiness endpoint is http://localhost:8080/health/ready.

See [the complete Local Work guide](https://github.com/pooya-labs/orbit/blob/local-work-v1.1.0/docs/local-work.md) for platform requirements, webhook configuration, persistence, stopping, reset, and testing.

## Security and scope

- This is a local single-owner runtime, not a hosted SaaS or production tenant service.
- The dashboard and API bind to loopback. OTLP gateways and raw telemetry stores remain inside the private Docker network.
- Human and workload tokens are separate. Keep `.env.local-work` private and never commit or share it.
- Outbound webhook delivery is opt-in and requires an endpoint and secret reference supplied by the operator.
- No VM, public DNS, external identity provider, Cloudflare deployment, paid hosting, or live customer traffic is included.

The public portfolio demo and immutable `v1.0.0` tag remain unchanged. This release has its own immutable tag: `local-work-v1.1.0`.
