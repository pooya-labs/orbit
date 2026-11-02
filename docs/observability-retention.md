# Telemetry retention and cardinality policy

## Scope

This policy defines the repository's **local Compose baseline**. It makes telemetry expiry and backend resource limits explicit and CI-checked; it is not a production capacity certification and does not create per-workspace budgets.

The local Prometheus, Loki, and Tempo services run in a single-tenant development topology. Loki has `auth_enabled: false`, so its per-user stream cap is effectively a cap for the whole local Loki tenant, not a quota per ORBIT workspace. Tempo's matching limits likewise apply to its single configured tenant. Production deployments must configure equivalent limits and retention on the managed backend, with real tenant separation and per-workspace quotas before tenant ingestion is enabled.

## Retention baseline

| Signal / state | Local policy | Enforcement |
| --- | --- | --- |
| Metrics | 168 hours (7 days) | Prometheus TSDB `--storage.tsdb.retention.time=7d` |
| Logs | 168 hours (7 days) | Loki compactor retention with `retention_period: 168h` |
| Traces | 168 hours (7 days) | Tempo backend-scheduler retention job with `block_retention: 168h` |
| API rate-limit windows | Expires after 2 minutes | PostgreSQL `expires_at` plus request-driven cleanup at most once per 5-minute time slot |

The seven-day signal retention aligns with the current maximum seven-day SLO rolling window. Expired PostgreSQL rate-limit rows are deleted when the API receives a later rate-limited request in a new five-minute cleanup slot; this is intentionally lightweight cleanup, not a general database vacuum/retention service.

## Ingestion and cardinality guardrails

### Prometheus

Only the Collector exporter scrape has the following limits: 20,000 samples, 32 labels per sample, 128-byte label names, 256-byte label values, and a 10 MiB scrape body. Prometheus treats an over-limit scrape as failed rather than silently accepting a partial scrape, so an exporter that exceeds these limits creates a visible scrape failure/gap. See the [Prometheus scrape configuration reference](https://prometheus.io/docs/prometheus/latest/configuration/configuration/).

### Loki

The local instance keeps structured metadata enabled but limits label names to 128 characters, label values to 256 characters, and label count to 12 per stream. It caps active streams at 5,000 for the single local tenant, ingestion at 4 MB/s with an 8 MB burst, and individual log lines at 256 KiB. Samples older than 168 hours are rejected. High-cardinality request IDs, user IDs, and timestamps should remain structured metadata or log-line fields, not indexed labels. See the [Loki configuration reference](https://grafana.com/docs/loki/latest/configure/).

### Tempo

The local instance caps attribute key/value size at 1 KiB, active traces at 10,000 for its single local tenant, and each trace at 5 MiB. The backend scheduler checks retention hourly, keeps blocks for 168 hours, and retains compacted source blocks for one hour to allow blocklist readers to catch up. Oversized individual attributes are truncated by Tempo's distributor; operators should watch `tempo_distributor_attributes_truncated_total`. See the [Tempo configuration reference](https://grafana.com/docs/tempo/latest/configuration/) and [compaction guidance](https://grafana.com/docs/tempo/latest/operations/compaction/).

### Collector

The existing Collector pipeline already has a 256 MiB memory limit, a 64 MiB spike allowance, a one-second check interval, and a five-second batch timeout. This change deliberately keeps those controls instead of adding a second overlapping memory or batching processor.

## Durable API history is not TTL-pruned

The following records are audit or operator history and remain durable/append-only: workspace audit events, incident timeline events, alert-instance lifecycle events, notification delivery attempts, and deployment annotations. Deleting them automatically would undermine the product's current immutable-history contract. A separate retention decision must define legal/compliance requirements, export/archive behavior, workspace ownership, and operator visibility before any TTL or purge is added.

The current API does prune ephemeral hashed rate-limit windows after their expiry. It does not yet define a general retention policy for durable API state, PostgreSQL backups, or restore points.

## Production boundary

The repository's Compose stack and limits are for local development and CI. They are not per-workspace quotas, managed-service configuration, backup guarantees, or proof of production readiness. Before enabling tenant-facing ingestion, provision managed backend retention, per-tenant cardinality/ingestion budgets, database backup/restore, service-specific monitoring, and a tested operational runbook. Keep production tenant ingestion disabled until those gates pass.

## CI verification

`scripts/check_observability_limits.py` checks that the configured budgets and existing API/Collector limits remain present. The observability CI job then starts the pinned Prometheus, Loki, Tempo, and Collector images; their native configuration loaders validate the YAML while the existing OTLP smoke test verifies that metrics, logs, and traces still traverse the pipeline.
