# ORBIT — System Architecture

## Product boundary

ORBIT is an observability control plane, not an application-specific monitoring dashboard. It will ingest standards-based telemetry, preserve source context, and let operators move between service health, metrics, traces, logs, incidents, and deployments.

## Target system shape

```text
Instrumented services
      |
      | OpenTelemetry SDK / OTLP
      v
OpenTelemetry Collector
      |
      +------ Metrics ------> Prometheus
      |
      +------ Traces -------> Tempo
      |
      +------ Logs ---------> Loki
      |
      +------ Events -------> NATS JetStream
                                |
                                v
                         ORBIT Go API
                                |
                    +-----------+-----------+
                    |                       |
                PostgreSQL                Redis
                    |                       |
                    +-----------+-----------+
                                |
                         REST / WebSocket
                                |
                         Next.js workspace
```

## Current implementation

### Web workspace

- Next.js App Router, React, TypeScript, and a responsive dark observability interface.
- Static export to GitHub Pages at `/orbit/`.
- English / Persian localization, locale-aware number and time formatting, persistent language preference, and mirrored RTL layout.
- Current chart, KPI cards, service inventory, and incidents are illustrative demo fixtures.
- Demo provenance is visible in the UI. The public page does not call the local API.

### Go API

- Standard-library HTTP server with bounded read/write/idle timeouts and graceful shutdown.
- Request IDs, structured JSON request logs, and panic recovery.
- `GET /health/live`, `GET /health/ready`, and `GET /api/v1/meta`.
- `GET /api/v1/services` and `GET /api/v1/incidents?status=...`.
- `GET /api/v1/telemetry/services` discovers names from Prometheus, Loki, and Tempo, upserts normalized service/source observations in PostgreSQL, and returns the persisted active catalog through validated cursor-based pages with discovery timestamps and partial-backend warnings. The companion service-attributes endpoint exposes a bounded allowlist of observed attributes with per-key backend provenance; this is not yet a full workload-side OpenTelemetry resource ingestion pipeline. If every telemetry backend is unavailable, it serves the last known PostgreSQL catalog when available.
- `GET /api/v1/telemetry/metrics`, `/logs`, and `/traces` expose bounded, service-scoped queries against Prometheus, Loki, and Tempo. Metrics supports allow-listed dimensions for the demo request-rate series; it does not accept arbitrary PromQL or arbitrary metric names.
- Data endpoints use explicit JSON envelopes and return safe errors without leaking database details.
- Protected `/api/v1/*` data routes validate OAuth access tokens through OIDC discovery and the issuer's JWKS. The verifier pins the allowed algorithms to RS256/ES256, validates issuer, audience, `exp`, `iat`, optional `nbf`, and `sub`, and caches signing keys with bounded refresh and rotation handling.
- Centralized route policy requires `viewer` for reads, `operator` for future operational mutations, and `administrator` for future administrative routes. Unknown role values never grant privileges. `/health/live` and `/api/v1/meta` remain public; readiness reveals only a minimal status.
- Development may omit OIDC only when `ORBIT_ENVIRONMENT=development` is explicitly set. Staging and production require both `ORBIT_OIDC_ISSUER` and `ORBIT_OIDC_AUDIENCE`; startup validates discovery and JWKS before serving requests.

### PostgreSQL

- pgx connection pool with bounded pool settings.
- Versioned SQL migration ledger with sequential, transactional migrations.
- Service and incident read models with indexed status, environment, and timestamp columns.
- Initial data is deliberately tagged `demo_seed`; it is not observed production telemetry.
- A separate `telemetry_services` table persists observed service identities and JSONB resource attributes; `telemetry_service_sources` stores normalized Prometheus / Loki / Tempo provenance with per-source observation timestamps.
- Catalog reads include services observed within the last seven days, ordered by name, with a 250-service cap.

### Local infrastructure

- Docker Compose provisions PostgreSQL, Redis, NATS JetStream, and the Go API.
- An optional `observability` profile adds OpenTelemetry Collector, Prometheus, Loki, and Tempo.
- The Collector receives OTLP over gRPC (`4317`) and HTTP (`4318`), then routes metrics to Prometheus, traces to Tempo, and logs to Loki.
- The opt-in `telemetry-demo` container is instrumented with the OpenTelemetry Go SDK and exports a request counter, a trace, and a structured log through OTLP/HTTP. Its `GET /emit` endpoint is a deterministic smoke-test trigger.
- Prometheus scrapes the Collector's Prometheus exporter on `9464`; the Prometheus, Loki, and Tempo query interfaces use ports `9090`, `3100`, and `3200`.
- Redis and NATS are provisioned but not yet wired into application code.
- All local development ports are bound to `127.0.0.1`; the local telemetry backends have no authentication and must not be exposed to the internet.

## Data model

### services

- `id`: UUID primary key
- `name`, `description`
- `environment`, `region`
- `status`: operational, degraded, incident, unknown
- `latency_p95_ms`, `error_rate`
- `last_seen_at`, `source`
- `created_at`, `updated_at`

### incidents

- `id`: UUID primary key
- `title`, `description`
- `service_id`: foreign key to services
- `severity`: critical, high, medium, low
- `status`: open, investigating, monitoring, resolved
- `started_at`, `updated_at`, `resolved_at`, `source`

## API conventions

- Prefix public endpoints with `/api/v1`.
- Use UTC timestamps in RFC 3339 / JSON time format.
- Return arrays in a `data` envelope.
- Return errors in an `error` envelope with machine-readable `code`, safe `message`, and `request_id`.
- Bound all database calls with request-scoped timeouts.
- Keep units explicit: duration in milliseconds, rates as decimal fractions, bytes in bytes.
- Never mix demo fixtures with live data without an explicit source label.

## Security and reliability requirements

- Authentication and authorization are mandatory before multi-user deployment.
- Secrets stay in environment variables or a dedicated secret manager, never in source control.
- Avoid logging tokens, credentials, or sensitive payload bodies.
- Enforce query timeouts, a 1 MiB request-body cap, a 16 KiB bearer-token cap, and fixed-window per-IP/per-subject request limits. The current limiter is process-local; it is not a distributed quota or tenant boundary.
- Readiness must verify required dependencies; liveness must remain a process-level check.
- Every asynchronous operation should carry correlation context.
- Retention and cardinality limits must be explicit to prevent uncontrolled telemetry cost.

## Current limitations

- The OIDC resource-server and API role-enforcement slice is merged in `main` (`977ce49`). Browser Authorization Code + PKCE, workspace membership/isolation, and user management are not yet implemented.
- Rate limits are process-local and need a shared limiter for multi-instance production deployments.
- M05 mutation routes, transactional audit records, and operator workflows remain disabled/not implemented until the security gate is merged and the corresponding audit/data model is delivered.
- The local OTLP pipeline and a dedicated instrumented demo workload are available; the production Go API is not instrumented yet.
- ORBIT's API now performs fixed, bounded read queries against Prometheus, Tempo, and Loki (15m–24h windows, capped results, 6-second backend timeout, 2 MiB response cap).
- The dashboard has separate Metrics, Logs, and Traces views that call the typed query methods, display backend provenance and query timestamps, and support 15m/1h/6h/24h windows with 30-second refresh.
- A shared service selector reads the active persisted service catalog using cursor pagination; each option shows which telemetry stores reported that service, and a bounded resource-attribute panel displays attribute provenance.
- The UI distinguishes loading, empty, error, and partial results. Metrics series are rendered independently, and Prometheus sample freshness/gaps are flagged using query-step and sample timestamps; those scrape-interval checks are not applied to event-driven logs or traces.
- Redis and NATS are available in Compose but are not yet used by the Go API.
- The Pages deployment is frontend-only. The Go API is local-only, not publicly hosted.
- The Overview dashboard consumes the service and incident API in API mode; the Metrics/Logs/Traces views consume the bounded telemetry query API and persisted observed service catalog. Full resource-attribute extraction from all telemetry backends remains future work.


### Legacy route deployment boundary

Unscoped inventory and telemetry read endpoints are development-only. Staging and production must return a non-disclosing 404 for legacy global reads; workspace-scoped reads require verified membership. This gate does not partition Prometheus, Loki, or Tempo data and does not make telemetry ingestion tenant-aware.


### Workspace-scoped telemetry query contract

The workspace API exposes separate metrics, logs, and traces query routes under `/api/v1/workspaces/{workspaceID}/telemetry/*`. Every request verifies the caller's workspace membership before contacting a backend. Prometheus and Loki selectors and Tempo TraceQL include the workspace UUID as `orbit_workspace_id` / `resource.orbit.workspace.id`; there is deliberately no fallback to unscoped queries. Telemetry producers must attach the canonical workspace resource attribute, and ingestion must authenticate/authorize the producer's workspace assignment. Until that ingestion trust boundary is implemented and tested, these query routes return only data that carries the workspace label and must not be treated as proof that the ingestion path is tenant-safe.


### Workspace operations: incidents and alert-rule definitions

The API now provides workspace-scoped incident lifecycle mutations with an append-only incident timeline and transactional workspace audit. Alert-rule definitions are stored per workspace in PostgreSQL and can be listed, created, updated, enabled/disabled, or deleted through authenticated workspace routes. Mutation roles are owner, administrator, and operator; workspace members can read definitions. A background evaluator processes only the workspace-attributed demo `request_rate` counter through a fixed Prometheus query. Pending/firing/resolved/cancelled instances and append-only lifecycle events are persisted transactionally; a PostgreSQL advisory lock coordinates the evaluator across API replicas, and telemetry gaps do not count toward a pending duration or automatically resolve a firing alert. Notification delivery/retry adapters remain unimplemented. Latency and error-rate alerting are not supported until those metrics exist and are sourced safely.


### Workspace deployment annotations

Deployment annotations are immutable workspace-owned release markers linked by a composite service/workspace foreign key. The service name and environment are derived from the linked service, never trusted from the browser. Creation verifies owner/administrator/operator membership, inserts the annotation, and appends `deployment.annotation_created` to workspace audit in the same transaction. Database triggers reject UPDATE, DELETE, and TRUNCATE. Workspace members can list and read annotations; comparison requires two distinct deployments of the same service in chronological order and returns metadata differences plus elapsed time only. It does not infer telemetry causality.


### Workspace service level objectives and error budgets

`service_level_objectives` definitions are linked to a service through the composite `(service_id, workspace_id)` foreign key. Owner/administrator/operator roles can create, update, enable/disable, and delete definitions; every mutation appends a workspace audit event in the same PostgreSQL transaction. The SLI is fixed to HTTP request-success events for a bounded normalized route; callers cannot submit PromQL or select arbitrary metrics. The evaluator queries `orbit_demo_requests_total` with the authenticated workspace label, service name, route, and `slo_outcome` labels. 2xx/3xx responses are good; all other instrumented responses are bad. Health checks and routes not explicitly instrumented are excluded.

Targets are fractional values from 0.90 through 0.9999. Rolling windows are limited to 1, 3, or 7 days to match the current Prometheus retention. Allowed error events equal `total_events * (1 - target)`; remaining events equal `max(0, allowed - bad)`; burn rate equals `bad / allowed`. Empty result vectors yield `insufficient_data` with nullable ratios, not 100% success. The unscoped local Compose demo bypasses workload authentication and therefore cannot be used as workspace SLO evidence.


### Workspace audit history

Authenticated workspace members can read the most recent audit events through `GET /api/v1/workspaces/{workspaceID}/audit-events`. The dashboard groups the current audited M05 mutation families (incidents, alert rules, webhook channels, deployment annotations, SLO definitions, and workspace creation), exposes request IDs and recorded details, and refreshes periodically. Membership is verified before the audit query. Actor issuer/subject fields remain hidden from the browser response; historical role snapshots and an actor-safe display identity are not yet part of the read model. Webhook delivery attempts and alert-instance transitions retain their own append-only histories.
