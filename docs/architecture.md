# ORBIT — Initial Architecture

## Product boundary

ORBIT is an observability control plane, not an application-specific monitoring dashboard. It should ingest standards-based telemetry, preserve source context, and let operators move between service health, metrics, traces, logs, incidents, and deployments.

## Target system shape

```text
Instrumented services
      |
      | OpenTelemetry SDK / OTLP
      v
OTel Collector tier
      |
      +------ Metrics ------> Prometheus
      |
      +------ Traces -------> Tempo
      |
      +------ Logs ---------> Loki
      |
      +------ Events -------> NATS
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

## Service responsibilities

### Web workspace

- Presents service inventory, topology, dashboards, incidents, and deployment context.
- Reads through a typed API client; it must not query telemetry stores directly from the browser.
- Uses WebSocket updates only for state that benefits from push delivery.
- Marks stale, missing, demo, and live data distinctly.

### Go API

- Owns authentication, workspace boundaries, service metadata, alert configuration, and query orchestration.
- Exposes versioned REST endpoints and a small number of event streams.
- Validates tenant/workspace scope before querying telemetry backends.
- Does not become a proxy for arbitrary unbounded PromQL or LogQL input.

### Ingestion

- Accepts OTLP through an OpenTelemetry Collector.
- Keeps ingestion credentials separate from dashboard/browser credentials.
- Applies resource normalization, redaction, and retention policy before storage.

### Storage

- PostgreSQL: users, workspaces, service catalog, alert rules, incidents, deployment annotations, and audit events.
- Redis: short-lived cache, rate-limit counters, and ephemeral coordination.
- Prometheus, Tempo, and Loki: purpose-built time-series, trace, and log storage.
- NATS: internal event distribution where asynchronous fan-out is needed.

## API conventions

- Prefix public endpoints with `/api/v1`.
- Use UTC timestamps in RFC 3339 format.
- Include stable IDs and explicit pagination cursors.
- Return structured errors with a machine-readable `code`, safe `message`, and request ID.
- Keep units explicit: duration in milliseconds, byte counts in bytes, rates with a named time base.
- Never mix demo fixtures with live data without an explicit source label.

## Initial service contracts

The first backend milestone should introduce these read models:

- `ServiceSummary`: id, name, environment, team, region, health, last_seen_at, source.
- `ServiceMetric`: service_id, metric_name, value, unit, observed_at, aggregation_window, source.
- `IncidentSummary`: id, title, severity, status, started_at, affected_services.
- `DeploymentMarker`: id, service_id, version, environment, deployed_at, outcome.

These are product-level contracts, not final database schemas. Their exact types and validation rules will be settled alongside the Go API.

## Security and reliability requirements

- Authentication and authorization are mandatory before multi-user deployment.
- Secrets stay in environment variables or a dedicated secret manager, never in source control.
- Avoid logging tokens, credentials, or sensitive payload bodies.
- Enforce query timeouts, payload limits, and per-workspace rate limits.
- Health endpoints must distinguish liveness from readiness.
- Every asynchronous operation should carry correlation context.
- Retention and cardinality limits must be explicit to prevent uncontrolled telemetry cost.

## Current implementation status

Implemented:
- Responsive Next.js workspace with clearly labelled illustrative fixture values.
- Go HTTP API process with liveness, readiness, and metadata endpoints.
- Request IDs, structured request logs, panic recovery, server timeouts, and graceful shutdown.
- Unit tests for the initial HTTP contract.

Not implemented yet:
- Database or cache connections.
- Authentication and workspace authorization.
- OTLP ingestion, telemetry storage, or live metrics/traces/logs.
- External dependency probes in the readiness endpoint.

The API foundation is intentionally small and standard-library-only. Additional infrastructure will be introduced when it has a concrete consumer and testable contract.
