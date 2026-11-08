# ORBIT

<p align="center"><a href="https://github.com/pooya-labs/pooya-labs-platform"><img src="public/brand/pooya-monogram.webp" alt="Pooya Labs Platform" width="56" /></a></p>

**Distributed Systems Intelligence & Observability**

[![Stable release](https://img.shields.io/badge/release-v1.0.0-2ea44f)](https://github.com/pooya-labs/orbit/releases/tag/v1.0.0)
[![CI](https://github.com/pooya-labs/orbit/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/pooya-labs/orbit/actions)
[![Security](https://github.com/pooya-labs/orbit/actions/workflows/security.yml/badge.svg?branch=main)](https://github.com/pooya-labs/orbit/actions)

**Project owner & maintainer:** [Pooya Labs Platform](https://github.com/pooya-labs/pooya-labs-platform) · **Source:** [pooya-labs/orbit](https://github.com/pooya-labs/orbit)

ORBIT is an engineering-focused observability workspace for exploring service health, incidents, operational workflows, and distributed telemetry across metrics, logs, and traces.

> **Release status: v1.0.0 stable portfolio-demo scope is complete. Post-v1 Cloudflare expansion is in progress.**  
> The immutable v1.0.0 release gates passed. The Cloudflare edge gateway is merged into main, but it is not yet connected to a live Go/Keycloak origin; a separate Workers + D1 profile is under development in draft [PR #140](https://github.com/pooya-labs/orbit/pull/140) and is not deployed. Apps Script remains unchanged and synthetic-only. Do not interpret either Cloudflare code path as a production-ready or live dual-runtime deployment.

## Start here

- **[Open the dashboard](https://pooya-labs.github.io/orbit/)**
- **[ORBIT v1.0.0 stable release](https://github.com/pooya-labs/orbit/releases/tag/v1.0.0)**
- **[Release notes](docs/releases/v1.0.0.md)**
- **[Architecture](docs/architecture.md)**
- **[Roadmap and completion record](docs/roadmap.md)**

The GitHub Pages URL serves the static frontend. The persistent interactive portfolio demo runs through the owner-deployed Google Apps Script Web App wrapper, which embeds the dashboard and bridges API requests to the private demo datastore. Its /exec URL is owner-controlled and is intentionally not committed to this public repository.

## What is included in v1.0.0

The workspace includes the following views and workflows:

| Area | Implemented capability |
| --- | --- |
| Overview | Service health, incident summaries, and workspace dashboard |
| Incidents | Workspace-scoped incident lifecycle, detail, and immutable timeline |
| Alerts | Alert-rule definitions, bounded evaluation, and alert instances |
| Notifications | Channel configuration and delivery history; public-demo deliveries are simulations only |
| Deployments | Immutable deployment annotations and same-service release comparison |
| SLOs | HTTP request-success objectives, observed availability, and error budgets |
| Audit | Searchable workspace audit history with pseudonymous actor references |
| Metrics | Curated request, successful-request, and failed-request rate views |
| Logs | Bounded log search with validated trace correlation |
| Traces | Bounded trace search and workspace-authorized span details |
| Localization and accessibility | English/Persian UI, RTL layout, responsive navigation, keyboard semantics, chart table alternatives, and dark/light themes |

The metric catalog is intentionally small and server-owned. The browser cannot submit arbitrary PromQL, metric names, label matchers, or query expressions. Trace and log detail requests are workspace-authorized; high-cardinality trace IDs are not used as Loki index labels.

## Runtime profiles — intentionally separate

ORBIT has three established execution profiles shown below. A fourth, separate Cloudflare-native Workers + D1 profile is being developed in draft PR #140; it is not merged or deployed and is documented separately below. These paths share the dashboard/API contract where applicable, but their deployment state and data guarantees are different.

| Capability | Apps Script portfolio demo | Cloudflare edge profile (post-v1; not live yet) | Local engineering stack |
| --- | --- | --- | --- |
| Frontend | GitHub Pages embedded by the owner-deployed Apps Script Web App | Same static Pages dashboard, configured to send API traffic through orbit-runtime | Same dashboard, served by the local development server |
| Persistence | Private owner-owned Google Spreadsheet; synthetic records only | Canonical Go API + PostgreSQL origin; no application database in the Worker | PostgreSQL with versioned migrations and repository-backed data |
| Identity and isolation | Pseudonymous browser-profile workspaces and bounded demo sessions; not production identity | Existing Keycloak OIDC/PKCE and Go-side workspace authorization, once the origin is deployed | OIDC/role and workspace authorization, workload identities and signal grants |
| Metrics, logs, traces | Bounded synthetic demo data | Existing private Collector → Prometheus/Loki/Tempo pipeline, once the origin is deployed | Real local OTLP/HTTP and OTLP/gRPC ingestion through the Collector |
| Notifications | Simulated delivery history; no outbound requests | Canonical Go outbox/attempt history, once the origin is deployed | Backend delivery/retry implementation for local engineering and test scenarios |
| Hosting | GitHub Pages plus the owner's Google Apps Script deployment | GitHub Pages + Cloudflare Worker + separately hosted Go/Caddy origin; not yet connected end to end | Docker Compose on the developer's machine; not a hosted production service |

### Cloudflare deployment state

The canonical orbit-runtime edge gateway was merged through PR #136. The last inspected Cloudflare state (November 6, 2026) showed Worker version 0.2.3, but its workers.dev route was disabled, no Worker bindings or gateway secret were attached, and the Go/Caddy origin was not provisioned. The Worker therefore remains fail-closed; this repository does not claim that the Cloudflare runtime is live.

A separate Workers + D1 implementation is being developed in draft [PR #140](https://github.com/pooya-labs/orbit/pull/140). It is a different runtime profile, not an upgrade to the thin edge gateway. It currently lacks live telemetry, alert evaluation, SLO budget calculation, notification delivery/attempt history, rate limiting and complete frontend acceptance. Do not deploy it or point Pages at it until those gates pass.



### Public demo safety and quotas

The Apps Script V8 runtime stores only synthetic demo records in the deployer's private Spreadsheet. It uses a fixed-origin google.script.run bridge, short-lived session capabilities, profile isolation, bounded request bodies, and quotas. Current repository limits include 250 API requests and 60 mutations per session, 25 new sessions, 800 API requests and 200 mutations per UTC day, a 20,000-row storage ceiling, and a 500-profile ceiling.

**Never enter customer data, personal/confidential information, credentials, production telemetry, or real webhook destinations into the public demo.** Notification delivery records are simulations: the demo does not send webhooks, email, or messages. Synthetic metrics, logs, traces, alert evaluations, and SLO results must remain visibly identified as demo data.

The Apps Script deployment is owned and administered separately from GitHub. Repository CI validates the Apps Script runtime contract and exercises the browser bridge with a mocked Google runtime; those automated tests are not a live test of Google's deployed Web App, Spreadsheet, or CacheService. The verification environment could not fetch script.google.com, so live owner-side bootstrap, mutation, and refresh-persistence behavior is **not independently verified by this release record**. This is an evidence limitation, not proof that the deployment is broken. See the [Apps Script setup and runtime guide](apps-script/README.md).

## Local engineering stack

The local stack is for development, integration testing, and demonstrating real local telemetry. It is not deployed behind the public Pages URL.

### Requirements

- Git
- Node.js 20 or newer and npm
- Docker Desktop (for PostgreSQL and the Go API)
- Go 1.27 or newer (for direct Go development)

### Run the dashboard

    git clone https://github.com/pooya-labs/orbit.git
    cd orbit
    npm install
    npm run dev

Open [http://localhost:3000](http://localhost:3000).

### Run the backend

From the repository root:

    docker compose up --build

The default Compose stack starts PostgreSQL, Redis, NATS, and the Go API. The local API is available at [http://localhost:8080](http://localhost:8080).

Useful endpoints:

| Endpoint | Purpose |
| --- | --- |
| /health/live | Process liveness |
| /health/ready | PostgreSQL readiness |
| /api/v1/meta | API metadata |
| /api/v1/services | Service inventory |
| /api/v1/incidents | Incident inventory |

To connect the local dashboard, open **Connect a source**, enter http://localhost:8080, and select **Test and connect**. Keep local development ports bound to loopback; the example stack is not configured as a public deployment.

### Run the local telemetry pipeline

The telemetry backends are optional. Start the observability profile from the repository root:

    docker compose --profile observability up -d postgres api prometheus loki tempo otel-collector telemetry-demo

The instrumented Go workload emits a request metric, a trace, and a structured log through OTLP/HTTP:

    curl.exe http://localhost:8081/emit

The Collector routes metrics to Prometheus, traces to Tempo, and logs to Loki. The Go API exposes bounded query endpoints for the configured telemetry stores. The dashboard presents those results in the Metrics, Logs, and Traces views.

The local path also exercises authenticated workspace ingestion and workspace-isolated read queries. A workload authorized for workspace A must not be able to write to or read data from workspace B. The tests reject direct cross-workspace writes and forged tenant attributes.

**Redis and NATS are provisioned by Compose but their functional application/event integration is deferred.** They are not represented as completed application features in v1.0.0.

## Verification and release evidence

The stable release was built from commit d2197244b1c1316cbe4b362d15c838c8eaa4cb1a, tagged v1.0.0, and published as a non-prerelease GitHub Release.

The release-commit verification completed successfully:

- **CI — 4/4 jobs passed:** web typecheck and production build; Apps Script runtime contract; 22 Chromium/Playwright interaction and axe tests; Go tests, vet and build; PostgreSQL backup/restore; full local OTLP Metrics/Logs/Traces end-to-end checks; authenticated workspace A/B write-to-query isolation; PostgreSQL restart and connection-pool recovery; and immutable workflow-action pin policy.
- **Security — 13/13 jobs passed:** dependency checks, full-history secret scanning, and scans/builds for the API, Collector, Tempo, Caddy, Prometheus, Loki, Redis, NATS, and the telemetry demo images.
- **GitHub Pages deployment:** successful on the stable release commit.
- **Stable release workflow:** exact-main CI and Security gates, version alignment, immutable tag creation, and Release publication all succeeded.
- **Windows NVDA Browse Mode navigation:** the automated heading-navigation check passed on the release-candidate source.

Evidence links: [release CI](https://github.com/pooya-labs/orbit/actions/runs/42032402892) · [release Security](https://github.com/pooya-labs/orbit/actions/runs/42032403220) · [Pages deployment](https://github.com/pooya-labs/orbit/actions/runs/42032403198) · [release workflow](https://github.com/pooya-labs/orbit/actions/runs/42032666313) · [NVDA navigation check](https://github.com/pooya-labs/orbit/actions/runs/42024149075).

These results establish the automated and repository-level checks listed above. They do not imply that every third-party deployment, device, or production environment has been manually accepted.

## Explicitly outside the v1.0.0 completion claim

The following are documented follow-up boundaries, not unfinished tasks hidden inside the released portfolio demo:

1. **Live Google deployment acceptance:** an owner-side clean-browser smoke is still needed to independently verify the deployed Apps Script doGet, bootstrap, metadata, a synthetic mutation, refresh persistence, and separate browser profiles. The automated bridge uses a mocked Google runtime.
2. **Human accessibility acceptance:** automated Chromium/axe checks and the Windows NVDA Browse Mode navigation check passed. Human keyboard-only rendered-state review, Android TalkBack, Apple VoiceOver, and audible-output review have not all been recorded. No formal WCAG conformance or certification is claimed. Use the [manual acceptance guide](docs/manual-acceptance-guide.md).
3. **Production identity and hosting:** the Go/PostgreSQL/OTel services, Keycloak, and ingress scaffolding are not a live hosted production service. The separate deferred Keycloak image scan has unresolved HIGH findings; it remains enabled and must be remediated before production identity deployment.
4. **Production operations:** hosted API and managed persistence, DNS/TLS/firewall/private-network policy, encrypted off-host backups and PITR, measured RPO/RTO, approved durable-history retention, sustained target-environment load/capacity, and production recovery exercises require their own deployment evidence.
5. **Deliberately bounded functionality:** arbitrary PromQL/LogQL, unrestricted metric discovery, real public-demo telemetry ingestion, real notification delivery, and Redis/NATS application integration are not v1 demo features.

The public portfolio demo is stable for its stated synthetic, low-volume purpose. It is **not** a production observability service, a customer-ingestion platform, or an uptime-guaranteed multi-tenant SaaS.

## Project documentation

- [Roadmap and v1.0.0 completion record](docs/roadmap.md)
- [Architecture and trust boundaries](docs/architecture.md)
- [Detailed project context and future handoff](docs/project-context.md)
- [v1.0.0 release notes](docs/releases/v1.0.0.md)
- [Apps Script demo setup and runtime](apps-script/README.md)
- [Manual accessibility acceptance guide](docs/manual-acceptance-guide.md)
- [Security threat model](docs/security-threat-model.md)
- [Telemetry retention and cardinality policy](docs/observability-retention.md)
- [PostgreSQL backup and restore runbook](docs/postgres-backup-restore.md)
- [Deployment and operations runbook](docs/operations-runbook.md)
- [OpenAPI 3.1 contract](docs/openapi.yaml)
- [Cloudflare dual-runtime expansion](docs/cloudflare-runtime.md)

## Quality checks

From the repository root:

    npm run typecheck
    npm run build
    cd server
    go test -race ./...
    go vet ./...
    go build ./cmd/orbit-api

GitHub Actions is the authoritative combined check for the web, Go/PostgreSQL, telemetry integration, security, and Pages release paths.

## Repository layout

    app/                         Next.js dashboard and design system
    apps-script/                 Public synthetic demo wrapper and Apps Script API
    lib/orbit/                   Typed API client, bridge, and session contracts
    examples/telemetry-go/       Instrumented OTLP demo workload
    server/cmd/orbit-api/        Go API entry point
    server/internal/             HTTP API, auth, telemetry, workers, and stores
    deploy/                      Local and deferred deployment scaffolding
    docs/                        Architecture, decisions, runbooks, roadmap, release notes
    .github/workflows/            CI, security, Pages, and release automation

## Completion status

**ORBIT v1.0.0 portfolio-demo scope: DONE.** No v1 implementation work remains open. Do not repeat completed features or reopen the immutable release tag for documentation changes. Future work should begin only from a concrete regression, a new issue, or an explicitly approved post-v1 production scope.

## License

No open-source license has been selected. All rights are reserved by default.
