# ORBIT

**Distributed Systems Intelligence & Observability**

**Project owner & maintainer:** [@pooya16pg-spec](https://github.com/pooya16pg-spec) · [Source repository](https://github.com/pooya16pg-spec/orbit)

ORBIT is an engineering-first observability platform for understanding the health, behavior, and reliability of distributed services.

## Live preview

**[Open the ORBIT dashboard](https://pooya16pg-spec.github.io/orbit/)**

The public preview is a static, responsive workspace. Demo mode uses clearly labelled illustrative data; API mode can read service and incident inventories from an explicitly configured ORBIT API. No production systems are connected by default.

## Current implementation

- Responsive Next.js + TypeScript observability workspace with an explicit demo/API source switch and English / Persian language support, including RTL layout.
- The dashboard reads service/incident inventories and telemetry queries from a configured ORBIT API. Metrics, Logs, and Traces share a cursor-paginated service selector populated from Prometheus, Loki, and Tempo; the Metrics view supports allow-listed multi-series grouping and timestamp-based stale/gap warnings.
- Go HTTP API with liveness, readiness, metadata, service/incident inventory, workspace-scoped incident lifecycle and alert-rule definition APIs, immutable incident timelines, and bounded Prometheus/Loki/Tempo query endpoints.
- PostgreSQL repository with versioned migrations, clearly marked seed data, and a normalized telemetry service catalog.
- Docker Compose stack for PostgreSQL, Redis, NATS, and the Go API.
- Optional local OpenTelemetry Collector → Prometheus, Tempo, and Loki pipeline.
- An instrumented Go demo emits a request metric, trace, and structured log through OTLP/HTTP; CI verifies all three in their backend query APIs.
- The API provides bounded read endpoints for request-rate metrics, service logs, and trace search, with source and query-time metadata.
- GitHub Actions for tests, static build, and GitHub Pages deployment.

## Project documentation

- [Roadmap](docs/roadmap.md) — milestones, remaining work, and dependency gates.
- [Architecture](docs/architecture.md) — current system shape and runtime boundaries.
- [Project context & handoff](docs/project-context.md) — implementation history, decisions, and the continuity notes for future work.
- [OIDC security decision](docs/decisions/0001-oidc-access-token-authentication.md) — access-token validation, roles, route policy, and the M05 security gate.

## Technology

| Layer | Technology |
| --- | --- |
| Web application | Next.js · React · TypeScript |
| API | Go 1.24+ |
| Persistence | PostgreSQL 17 |
| Cache | Redis 7 (stack provisioned; application integration pending) |
| Event transport | NATS JetStream (stack provisioned; application integration pending) |
| Local telemetry pipeline | OpenTelemetry Collector · Prometheus · Loki · Tempo · instrumented Go demo (opt-in Compose profile) |
| CI/CD | GitHub Actions · GitHub Pages · Docker Compose |

## Run locally on Windows

Requirements: Git, Node.js 20+, and npm.

```powershell
git clone https://github.com/pooya16pg-spec/orbit.git
cd orbit
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

## Connect the dashboard to the local API

For a local API, run the dashboard locally with `npm run dev` and run the backend with Docker Compose. Open `http://localhost:3000`, choose **Connect a source**, enter `http://localhost:8080`, then select **Test and connect**. The dashboard verifies metadata and reads services and incidents. The URL is stored only in this browser. The public GitHub Pages site does not host the Go API; connecting that hosted site requires a separately hosted HTTPS API whose CORS allowlist includes the Pages origin.

When connected, select Metrics, Logs, or Traces to query live telemetry. The service selector merges discovered names across Prometheus, Loki, and Tempo, shows their source backends, and refreshes the catalog every 60 seconds. Demo mode remains available at any time.

## Run the backend stack

Install Docker Desktop, then from the repository root:

```powershell
docker compose up --build
```

This starts PostgreSQL, Redis, NATS, and the Go API. On its first start the API applies the versioned SQL migrations (`001_initial`, `002_telemetry_service_catalog`, and `003_telemetry_resource_attribute_sources`) and inserts five sample services and two sample incidents, each marked with source `demo_seed`.

| Endpoint | Purpose |
| --- | --- |
| [http://localhost:8080/health/live](http://localhost:8080/health/live) | Process liveness |
| [http://localhost:8080/health/ready](http://localhost:8080/health/ready) | PostgreSQL readiness |
| [http://localhost:8080/api/v1/meta](http://localhost:8080/api/v1/meta) | API metadata |
| [http://localhost:8080/api/v1/services](http://localhost:8080/api/v1/services) | Service inventory |
| [http://localhost:8080/api/v1/incidents](http://localhost:8080/api/v1/incidents) | Incident inventory |
| [http://localhost:8080/api/v1/incidents?status=investigating](http://localhost:8080/api/v1/incidents?status=investigating) | Filter incidents |

Stop containers with `docker compose down`. To delete local database/cache volumes too, use `docker compose down -v` (this permanently deletes their local data).

## Start the local observability stack

The telemetry backends are optional and do not start during a normal `docker compose up`:

```powershell
docker compose --profile observability up -d postgres api prometheus loki tempo otel-collector telemetry-demo
```

The `telemetry-demo` service is an instrumented Go workload. Trigger one request with `curl.exe http://localhost:8081/emit`; it emits a request counter, a trace (the response includes its trace ID), and a structured log. You can also point another workload at `http://localhost:4318` (OTLP/HTTP) or `localhost:4317` (OTLP/gRPC). The Collector routes metrics to Prometheus, traces to Tempo, and logs to Loki. Once the stack is ready, the Go API discovers service names across the configured stores and exposes bounded read-only query endpoints (allowed windows: 15m, 1h, 6h, 24h):

```powershell
Invoke-RestMethod "http://localhost:8080/api/v1/telemetry/services?window=1h"
Invoke-RestMethod "http://localhost:8080/api/v1/telemetry/metrics?window=1h&service=orbit-telemetry-demo"
Invoke-RestMethod "http://localhost:8080/api/v1/telemetry/logs?service=orbit-telemetry-demo&window=1h&limit=50"
Invoke-RestMethod "http://localhost:8080/api/v1/telemetry/traces?service=orbit-telemetry-demo&window=1h&limit=20"
```

The discovery response merges valid names from Prometheus, Loki, and Tempo, records source provenance, and returns partial results with safe warnings if one backend is unavailable. Discovered services and source observations are persisted in a normalized PostgreSQL catalog and returned through cursor-based pages with first-seen, last-seen, and last-discovered timestamps. If all telemetry backends are unavailable, the API serves the last known PostgreSQL catalog with a warning when cached services exist. The service-scoped resource-attributes endpoint extracts a bounded allowlist from Prometheus series labels, Loki labels, and Tempo resource tags, preserves per-attribute source provenance, and stores successful observations as JSONB. This is a preview, not yet a complete arbitrary OpenTelemetry resource-attribute ingestion pipeline. The API uses fixed, bounded queries instead of accepting arbitrary PromQL or LogQL from callers.

These development services bind to loopback and have no authentication. Do not expose their ports to a public network. Prometheus keeps seven days of metrics and Loki is configured for seven-day log retention; data volumes persist until removed. Stop only these services with `docker compose stop postgres api prometheus loki tempo otel-collector telemetry-demo`.

The local sample service proves end-to-end delivery of all three OTLP signals. The Go API exposes bounded read-only queries to Prometheus, Loki, and Tempo, and the dashboard renders those results in separate Metrics, Logs, and Traces views. The v0.10.0 preview adds bounded resource-attribute discovery with per-attribute provenance, cursor pagination, allow-listed multi-series request-rate grouping, and Prometheus sample freshness/gap warnings. Full OpenTelemetry resource-attribute ingestion and production API instrumentation remain open work.

## Quality checks

```powershell
npm run typecheck
npm run build
cd server
go mod tidy
go test -race ./...
go vet ./...
go build ./cmd/orbit-api
```

GitHub Actions runs web typecheck/build, Go tests against a real PostgreSQL service container, and a Docker Compose smoke test that verifies all three OTLP signals both in their backends and through the ORBIT API.

## Repository map

```text
app/                         Next.js dashboard and design system
examples/telemetry-go/        Instrumented OTLP demo workload
server/cmd/orbit-api/        Go API entry point
server/internal/httpapi/     HTTP routes and middleware
server/internal/store/       API read models and repository contract
server/internal/store/postgres/ PostgreSQL implementation and migrations
docs/                        Architecture, roadmap, local development
.github/workflows/            CI and Pages deployment
```

## Roadmap

See [docs/roadmap.md](docs/roadmap.md), [docs/architecture.md](docs/architecture.md), and the [OpenAPI 3.1 contract](docs/openapi.yaml). The TypeScript API client and contracts live in `lib/orbit/`.

## Release status

The v0.1.0 public preview introduced the dashboard. The v0.2.0 backend foundation added PostgreSQL-backed service/incident APIs, OpenAPI, a typed client, and Docker Compose. The v0.3.0 preview added a verified browser source switch between demo fixtures and a configured ORBIT API. The v0.4.0 preview added the optional local OTLP pipeline backed by Prometheus, Loki, and Tempo. The v0.5.0 preview adds an instrumented Go workload and CI verification that metrics, logs, and traces reach their respective backends. The v0.6.0 preview adds bounded ORBIT API read endpoints for Prometheus metrics, Loki logs, and Tempo trace search. The v0.7.0 preview adds live Metrics, Logs, and Traces dashboard views with selectable time windows, 30-second refresh, query timestamps, and explicit loading/empty/error states. The v0.8.0 preview added cross-backend service discovery, source provenance, a shared dynamic service selector, and service-scoped request-rate queries. The v0.9.0 preview adds a persisted PostgreSQL telemetry service catalog, owner attribution, and English / Persian UI localization with RTL support. The v0.10.0 preview completes the bounded M04 follow-through: resource-attribute discovery with per-attribute backend provenance, cursor-paginated service catalog, allow-listed multi-series request-rate grouping, and per-series Prometheus stale/gap warnings. Full workload-side OpenTelemetry resource ingestion, arbitrary metric-name selection, production API instrumentation, and most M05 operations workflows remain open; the first workspace-scoped incident lifecycle backend slice (PR #66) is merged, while dashboard controls/timeline UI, alerts, deployment annotations, and SLO workflows are not yet implemented. M06 now includes server-side OIDC/RBAC, workspace membership and scoped query APIs, authenticated OTLP HTTP/gRPC gateways, workload identity and grant administration, and CI-verified rejection of forged producer workspace attributes across metrics, logs, and traces. Mixed-batch OTLP rejection is now covered by merged, CI-passing PR #38 for metrics, logs, and traces over HTTP and gRPC. Backend query isolation is covered by merged, CI-passing PR #39; merged, CI-passing PR #40 verifies non-members are rejected before any telemetry backend access. Oversized OTLP request rejection is covered by merged, CI-passing PR #42 for all three signals over HTTP and gRPC. Shared rate limits across gateway instances and transports are covered by merged, CI-passing PR #43. Concurrent multi-replica rate-limit coverage is merged and CI-passing in PR #46. PR #48 adds CI-passing PostgreSQL-backed grant and workload-identity revocation checks across independent HTTP/gRPC gateway replicas. Production ingress TLS/network policy, broader in-flight revocation/load testing, real-provider browser PKCE rollout, and deployment runbooks remain open. Production ingress TLS/network policy, broader load and revocation-bound testing, real-provider browser PKCE rollout, and operational runbooks remain open; production tenant ingestion is not yet enabled.

## License

No open-source license has been selected. All rights are reserved by default.


## M06 verification update — PR #48

PR #48 is merged into `main` (squash merge commit `3c5619d380b498a50660415b68175f8266c48a71`). CI run `41352185701` completed successfully: Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test all passed. The PostgreSQL-backed integration test uses four independent gateway/authorizer instances (two HTTP, two gRPC) and verifies that signal-grant revocation and workload-identity revocation are observed by other replicas for metrics, logs, and traces; denied requests do not reach the Collector forwarders.

The test establishes read-after-commit revocation for requests authorized after the revocation transaction commits. It does not establish cancellation of requests that completed authorization before that commit. In-flight revocation ordering/cancellation remains an explicit open design and test item. Production ingestion remains disabled until TLS/network policy, in-flight semantics, broader load testing, real-provider PKCE, and operational runbooks are complete.
 

## M06 verification update — PR #50

PR #50 is merged into `main` (squash merge commit `45680118f44c3cd35f64214072085e6ca1a43de6`). CI run `41352877449` completed successfully: Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test all passed.

The PostgreSQL-backed HTTP and gRPC integration tests define the in-flight revocation boundary: an export that has already passed authorization and entered the Collector forwarder may complete after a later identity-revocation commit. A request whose authorization begins after that commit must be denied and must not reach the forwarder. This is an authorization linearization guarantee, not cancellation or rollback of telemetry already admitted to the Collector. Production tenant ingestion remains disabled pending ingress TLS/network policy, broader load/capacity and failure testing, real-provider PKCE rollout, and operational runbooks.


## M05 backend update — PR #66

The first M05 slice is merged: authenticated workspace-scoped incident creation and status transitions, immutable timeline events, and atomic workspace audit writes. See the [PR #66 implementation](https://github.com/pooya16pg-spec/orbit/pull/66), [M05 roadmap](docs/roadmap.md), and [OpenAPI contract](docs/openapi.yaml). The dashboard incident operations UI, alerting, deployment annotations, and SLO workflows remain in progress.


## M05 alert-rule definitions — PR #69

Workspace members can list alert rule definitions; owners, administrators, and operators can create, update, enable/disable, and delete them. Definitions are workspace-scoped and every mutation is audited transactionally. PR #72 adds a scheduled evaluator for the allow-listed `request_rate` metric, persisted pending/firing/resolved/cancelled instances, append-only transition events, telemetry-gap handling, and PostgreSQL advisory-lock coordination across API replicas. Notification adapters and delivery/retry history remain open; see [the M05 roadmap](docs/roadmap.md) and [PR #72](https://github.com/pooya16pg-spec/orbit/pull/72).


## Webhook secret configuration

The next M05 slice adds workspace-owned webhook channels, a PostgreSQL transactional outbox, idempotent delivery claims across API replicas, bounded exponential retries, and append-only attempt history. The webhook sender requires HTTPS, an exact deployment hostname allowlist, public-unicast DNS results, no redirects, and bounded request/response timeouts. The API accepts only a deployment-managed `secret_ref`; raw webhook URLs and credentials are not accepted or returned.

For local Compose use, set these variables in an untracked `.env` file or inject them through your secret manager:

```dotenv
ORBIT_ALERT_WEBHOOK_ALLOWED_HOSTS=hooks.example.com
ORBIT_ALERT_WEBHOOK_SECRET_OPS_PRIMARY=https://hooks.example.com/your/provider/path
```

A workspace owner, administrator, or operator can then create a channel with `secret_ref: "OPS_PRIMARY"`. Never commit a real endpoint token or a populated `.env` file. Additional adapter types, delivery acknowledgements/silences, and latency/error-rate alerting remain separate work.


## M05 webhook notification delivery — PR #74

ORBIT can route alert firing/resolved events to workspace-owned webhook channels. Delivery rows are queued transactionally with alert lifecycle events, claimed using PostgreSQL leases and `SKIP LOCKED`, and retried with bounded exponential backoff. Every completed or lease-expired attempt is recorded in append-only history.

Only a deployment-configured secret reference is stored in PostgreSQL. The actual HTTPS endpoint URL/token must be injected through a secret manager or an untracked local `.env` file:

```dotenv
ORBIT_ALERT_WEBHOOK_ALLOWED_HOSTS=hooks.example.com
ORBIT_ALERT_WEBHOOK_SECRET_OPS_PRIMARY=https://hooks.example.com/your/provider/path
```

The allowlist is exact-host, HTTPS-only and restricted to port 443. The sender rejects non-public DNS results, follows no redirects, and applies bounded timeouts. The API accepts `secret_ref: "OPS_PRIMARY"`, never the raw URL or credential. Additional adapter types and delivery acknowledgement/silence controls remain future work.
