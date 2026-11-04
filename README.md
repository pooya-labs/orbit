# ORBIT

**Distributed Systems Intelligence & Observability**

**Project owner & maintainer:** [@pooya16pg-spec](https://github.com/pooya16pg-spec) · [Source repository](https://github.com/pooya-labs/orbit)

ORBIT is an engineering-first observability platform for understanding the health, behavior, and reliability of distributed services.

## Live preview

**[Open the ORBIT dashboard](https://pooya-labs.github.io/orbit/)**

GitHub Pages hosts the responsive static workspace and remains the public project front door. The interactive, zero-cost portfolio demo is being moved to a Google Apps Script Web App that embeds this same dashboard and persists synthetic demo changes in a private Google Spreadsheet. **The Apps Script Web App has not yet been deployed by the project owner**; follow [the setup guide](apps-script/README.md) after the repository-side implementation is ready. No production systems or real customer telemetry are connected.

## Current implementation

- Responsive Next.js + TypeScript observability workspace with an explicit demo/API source switch and English / Persian language support, including RTL layout.
- The dashboard reads service/incident inventories and Metrics/Logs/Traces through authenticated workspace-scoped APIs. Telemetry workspace selection and membership checks gate the persisted service/resource catalog, the server-owned metric catalog, and all three signal queries; legacy global telemetry reads remain limited to explicitly configured local development without browser OIDC and are never used in staging/production. Metrics offers three fixed views over `orbit_demo_requests_total`: total request rate, successful HTTP 2xx/3xx request rate, and failed request rate. Metric IDs, grouping labels, query windows, series count, and Prometheus timeout are bounded server-side; arbitrary metric names, label matchers, and PromQL are not accepted. Grouping is limited to instance, service version, and deployment environment. Trace details include span hierarchy/timing/status, and correlated Logs entries can open those details through the same workspace-authorized endpoint. Trace IDs remain in log content rather than high-cardinality Loki index labels.
- Go HTTP API with liveness, readiness, metadata, service/incident inventory, workspace-scoped incident lifecycle and alert-rule definition APIs, immutable incident timelines, and bounded Prometheus/Loki/Tempo query endpoints. Optional API self-telemetry adds bounded request metrics, duration histograms, route-template traces, and static lifecycle/error events; alert-evaluator and notification-delivery workers expose bounded run/item metrics and tick traces through the private Collector (PR #133).
- PostgreSQL repository with versioned migrations, clearly marked seed data, and a normalized telemetry service catalog.
- Docker Compose stack for PostgreSQL, Redis, NATS, and the Go API.
- Optional local OpenTelemetry Collector → Prometheus, Tempo, and Loki pipeline.
- An instrumented Go demo emits a request metric, trace, and structured log through OTLP/HTTP; CI verifies all three in their backend query APIs.
- CI verifies authenticated workspace write-to-query isolation: a workload granted to workspace A can send metrics, logs, and traces visible to A-scoped queries but not B-scoped queries; direct B writes and forged tenant attributes are rejected.
- The API provides bounded read endpoints for curated total/success/failure request-rate metrics, service logs, trace search, and authenticated workspace span details, with source and query-time metadata.
- GitHub Actions for tests, static build, and GitHub Pages deployment.

## Project documentation

- [Roadmap](docs/roadmap.md) — milestones, remaining work, and dependency gates.
- [Architecture](docs/architecture.md) — current system shape and runtime boundaries.
- [Project context & handoff](docs/project-context.md) — implementation history, decisions, and the continuity notes for future work.
- [OIDC security decision](docs/decisions/0001-oidc-access-token-authentication.md) — access-token validation, roles, route policy, and the M05 security gate.
- [Telemetry retention and cardinality policy](docs/observability-retention.md) — local seven-day retention, backend ingestion limits, and the remaining production/database gates.
- [PostgreSQL backup and restore runbook](docs/postgres-backup-restore.md) — full-database logical dumps, safe restore checks, and the remaining production backup gates.
- [Deployment and operations runbook](docs/operations-runbook.md) — production preflight, deployment verification, incident response, rollback, and recovery evidence.
- [GitHub Pages deployment](docs/deployment/github-pages.md) — Actions-only publishing source, automated source validation, and the public project URL.
- [Apps Script demo setup](apps-script/README.md) — owner-side setup, private Sheets datastore, anonymous Web App deployment, and the synthetic demo safety boundary.
- [Security threat model](docs/security-threat-model.md) — current trust boundaries, implemented controls, evidence, and remaining production go-live gates.

## Technology

| Layer | Technology |
| --- | --- |
| Web application | Next.js · React · TypeScript |
| Public portfolio demo API | Google Apps Script Web App |
| Public demo persistence | Private Google Sheets datastore (synthetic records only) |
| Extended local API | Go 1.27+ |
| Extended local persistence | PostgreSQL 17 |
| Cache | Redis 7 (stack provisioned; application integration pending) |
| Event transport | NATS JetStream (stack provisioned; application integration pending) |
| Local telemetry pipeline | OpenTelemetry Collector · Prometheus · Loki · Tempo · instrumented Go demo (opt-in Compose profile) |
| CI/CD | GitHub Actions · GitHub Pages · Docker Compose |

## Run locally on Windows

Requirements: Git, Node.js 20+, and npm.

```powershell
git clone https://github.com/pooya-labs/orbit.git
cd orbit
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

## Connect the dashboard to the local API

For the extended local API, run the dashboard locally with npm run dev and run the Go backend with Docker Compose. Open http://localhost:3000, choose **Connect a source**, enter http://localhost:8080, then select **Test and connect**. This retains the PostgreSQL-backed and real local OpenTelemetry path for development. The public portfolio demo uses the Apps Script Web App wrapper: it embeds this same static dashboard, bridges API calls through google.script.run, and stores only synthetic data in the owner's private Google Spreadsheet. That wrapper avoids a public browser-to-Apps-Script CORS/API-key setup. The Pages URL by itself remains a static frontend; the interactive demo becomes public only after the owner deploys the Apps Script /exec Web App.

When connected, select Metrics, Logs, or Traces to query live telemetry. The service selector merges discovered names across Prometheus, Loki, and Tempo, shows their source backends, and refreshes the catalog every 60 seconds. Demo mode remains available at any time.

## Run the backend stack

Install Docker Desktop, then from the repository root:

```powershell
docker compose up --build
```

This starts PostgreSQL, Redis, NATS, and the Go API. On its first start the API applies the versioned SQL migrations (`001_initial` through `016_workspace_otlp_resource_attributes`) and inserts five sample services and two sample incidents, each marked with source `demo_seed`.

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

The legacy discovery response merges valid names from Prometheus, Loki, and Tempo, records source provenance, and returns partial results with safe warnings if one backend is unavailable. Discovered services and source observations are persisted in a PostgreSQL catalog restricted to the legacy-default workspace and returned through cursor-based pages with first-seen, last-seen, and last-discovered timestamps. Authenticated workspace members read their own telemetry catalog from GET /api/v1/workspaces/{workspaceID}/telemetry/services; service names are unique within a workspace. The OTLP/HTTP and OTLP/gRPC gateways now capture resource metadata after workload token verification, explicit workspace/signal grant checks, rejection of producer tenant attributes, and injection of the server-authorized workspace ID. Only a curated allowlist of low-risk OpenTelemetry attributes is retained (32 keys, at most 8 values per key, strings capped at 256 bytes); credentials, command lines, user identifiers, pod UIDs, and arbitrary attributes are not persisted. Each attribute carries provenance (otlp, prometheus, loki, or tempo). The existing bounded backend discovery remains available for the legacy catalog; the workspace catalog no longer shares service names or metadata across tenants.

These development services bind to loopback and have no authentication. Do not expose their ports to a public network. Prometheus, Loki, and Tempo use a seven-day local retention baseline with explicit scrape, label, stream, attribute, and trace-size bounds; see [the retention policy](docs/observability-retention.md). Durable API audit/lifecycle history is not automatically deleted, and local single-tenant limits are not production per-workspace quotas. Data volumes persist until removed. Stop only these services with `docker compose stop postgres api prometheus loki tempo otel-collector telemetry-demo`.

The local sample service proves end-to-end delivery of all three OTLP signals. The Go API exposes bounded read-only queries to Prometheus, Loki, and Tempo, and the dashboard renders those results in separate Metrics, Logs, and Traces views. The v0.10.0 preview adds bounded resource-attribute discovery with per-attribute provenance, cursor pagination, allow-listed multi-series request-rate grouping, and Prometheus sample freshness/gap warnings. The M04 follow-up adds curated workload-side OpenTelemetry resource capture to the authenticated gateways and persists metadata per workspace; expanding the semantic-convention allowlist remains a reviewed follow-up rather than accepting arbitrary producer attributes. PRs #130–#132 add authenticated workspace trace/span detail, validated Logs-to-Trace correlation, and the safe three-ID workspace metric catalog. PR #133 adds opt-in bounded self-telemetry for the production Go API and alert/notification workers, exported only to the private Collector. Production hosting, identity, networking, retention, recovery, and capacity gates remain open.

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

The v0.10.0 preview is the current documented product baseline. M01–M04 delivered the responsive Next.js workspace, Go/PostgreSQL API foundation, optional local OpenTelemetry pipeline, bounded Metrics/Logs/Traces investigation views, cross-backend service discovery, persisted service catalog, provenance, pagination, and English/Persian localization. GitHub Pages publishes the static frontend only; it does not host the API or telemetry data plane.

**M05 — Operations workflows is implemented on `main`.** The workspace now includes authenticated incident lifecycle and immutable timelines, workspace alert rules and their evaluator, transactional webhook delivery/retry history, deployment annotations and release comparison, HTTP request-success SLOs/error budgets, and the searchable workspace audit log with pseudonymous actor references and captured roles. See [the roadmap](docs/roadmap.md) and [the OpenAPI contract](docs/openapi.yaml).

**M06 — Security and production hardening is substantially implemented in code and CI, but not deployed as a production service.** The backend includes human OIDC verification and role enforcement, workspace membership and scoped reads, workload identities and per-signal grants, authenticated OTLP/HTTP and OTLP/gRPC gateways, canonical tenant attribution, shared PostgreSQL rate limits, bounded admission, revocation semantics, fail-closed dependency handling, and real local PostgreSQL/Collector/backend evidence for the workspace-A/write-to-query boundary across metrics, logs, and traces. M05 state-changing workflows are workspace- and role-protected and audited. CI covers local retention/cardinality limits, PostgreSQL backup/restore, image scans, dependency/security checks, immutable GitHub Actions pins, and CycloneDX SBOM artifacts. A Caddy ingress scaffold, threat model, and operator runbook templates exist; none is evidence of a deployed production environment.

**Remaining M06 work is deployment-specific:** real human/workload OIDC providers and browser callback lifecycle; hosted API, managed PostgreSQL and telemetry backends; DNS/ACME, firewall/trusted-proxy and private-network policy; encrypted off-host backups/PITR with measured RPO/RTO; target-environment load and recovery evidence; approved durable-history retention/archive policy; production secret rotation, monitoring, and exercised runbooks. Production tenant ingestion remains disabled until the relevant gates are verified.

**Production tenant ingestion remains disabled.** Real human/workload OIDC providers, DNS and ACME certificates, host firewall and trusted-proxy rules, deployment network policies, managed-backend quotas, encrypted off-host backups and restore rehearsal, approved durable-history retention policy, and target-environment load/recovery evidence must be configured and verified before production rollout. See [the operations runbook](docs/operations-runbook.md) and [the threat model](docs/security-threat-model.md).

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


## M05 deployment annotations and release comparison

Workspace operators can record immutable deployment annotations for a service with version, commit SHA, optional change reference/release notes, and deployment time. Service name and environment are derived from the workspace-owned service record. Annotation creation and its workspace audit event commit together; database guards reject UPDATE, DELETE, and TRUNCATE.

Workspace members can list deployment history or compare two deployments of the same service in chronological order. The comparison reports only deployment metadata changes and elapsed time; it does not infer that a deployment caused a telemetry change. See [the M05 roadmap](docs/roadmap.md) and [the OpenAPI contract](docs/openapi.yaml).


## M05 service level objectives and error budgets

Workspace owners, administrators, and operators can create, update, enable/disable, and delete HTTP request-success objectives. Definitions are linked to a service in the same workspace and every mutation is audited transactionally. Targets are 90%–99.99%; rolling windows are 1, 3, or 7 days to match the configured Prometheus retention.

The SLI counts only explicitly instrumented routes: HTTP 2xx/3xx responses are good events; all other instrumented responses are bad events. The budget is computed from observed good/bad events using the fixed workspace/service/route selectors. A missing series returns `insufficient_data`, never synthetic 100% availability. The development telemetry demo bypasses workspace authentication and its unscoped metrics are intentionally excluded from workspace SLO calculations. See [the M05 roadmap](docs/roadmap.md), [architecture](docs/architecture.md), and [OpenAPI](docs/openapi.yaml).


## M05 workspace audit history

PR #79 adds the authenticated workspace Audit log page, which reads the latest 100 workspace audit events through `GET /api/v1/workspaces/{workspaceID}/audit-events`. Members can filter incident, alert-rule, notification-channel, deployment, SLO, and workspace events. Each record shows its action, resource, request ID, timestamp, and bounded details; the UI refreshes every 30 seconds and defensively suppresses sensitive-looking detail keys.

The API verifies workspace membership before returning events. Raw OIDC issuer and subject identifiers are intentionally omitted from the browser response. Migration 015 adds a random, stable actor reference within each workspace and snapshots the actor's role at event insertion time. Historical roles remain `unknown` rather than being guessed from current membership. The actor-reference mapping and event stream reject UPDATE, DELETE, and TRUNCATE. The Audit Log displays the pseudonymous reference and captured role.
