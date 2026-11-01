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
- Current chart, KPI cards, service inventory, and incidents are illustrative demo fixtures.
- Demo provenance is visible in the UI. The public page does not call the local API.

### Go API

- Standard-library HTTP server with bounded read/write/idle timeouts and graceful shutdown.
- Request IDs, structured JSON request logs, and panic recovery.
- `GET /health/live`, `GET /health/ready`, and `GET /api/v1/meta`.
- `GET /api/v1/services` and `GET /api/v1/incidents?status=...`.
- `GET /api/v1/telemetry/services` discovers and merges service names from Prometheus, Loki, and Tempo, preserving per-service source provenance and partial-backend warnings.
- `GET /api/v1/telemetry/metrics`, `/logs`, and `/traces` expose bounded, service-scoped queries against Prometheus, Loki, and Tempo.
- Data endpoints use explicit JSON envelopes and return safe errors without leaking database details.

### PostgreSQL

- pgx connection pool with bounded pool settings.
- Versioned SQL migration ledger and transactional first migration.
- Service and incident read models with indexed status, environment, and timestamp columns.
- Initial data is deliberately tagged `demo_seed`; it is not observed production telemetry.

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
- Enforce query timeouts, payload limits, and per-workspace rate limits.
- Readiness must verify required dependencies; liveness must remain a process-level check.
- Every asynchronous operation should carry correlation context.
- Retention and cardinality limits must be explicit to prevent uncontrolled telemetry cost.

## Current limitations

- No authentication, workspace isolation, or user management.
- The local OTLP pipeline and a dedicated instrumented demo workload are available; the production Go API is not instrumented yet.
- ORBIT's API now performs fixed, bounded read queries against Prometheus, Tempo, and Loki (15m–24h windows, capped results, 6-second backend timeout, 2 MiB response cap).
- The dashboard has separate Metrics, Logs, and Traces views that call the typed query methods, display backend provenance and query timestamps, and support 15m/1h/6h/24h windows with 30-second refresh.
- A shared service selector is populated from cross-backend service discovery; each option shows which telemetry stores reported that service.
- The UI distinguishes loading, empty, and error results; stale-data and missing-interval detection are still pending.
- Redis and NATS are available in Compose but are not yet used by the Go API.
- The Pages deployment is frontend-only. The Go API is local-only, not publicly hosted.
- The Overview dashboard consumes the service and incident API in API mode; the Metrics/Logs/Traces views consume the bounded telemetry query API and dynamic discovered service list. Discovered resources are not yet persisted as normalized records in the PostgreSQL service inventory.
