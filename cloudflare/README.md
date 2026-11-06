# ORBIT Cloudflare runtime

This is a second, independently deployed runtime. It does not replace or proxy the Apps Script demo.

## Resource

- Worker: `orbit-runtime`
- D1 database: `orbit-runtime` (`579805c5-7688-4a1a-a26f-4ce8ff2af04e`, EU jurisdiction)
- Apps Script remains the low-volume synthetic portfolio demo and keeps its private Google Spreadsheet.
- GitHub Pages remains the shared static dashboard.

## Runtime contract

The first slice provides health/meta endpoints, OIDC/JWKS validation for human and workload tokens, workspace membership checks, workspace-scoped telemetry queries, and OTLP/HTTP JSON ingestion for metrics, logs, and traces.

OTLP/gRPC and OTLP/HTTP protobuf are not advertised as supported by this Worker slice. Producers must send OTLP/HTTP JSON to `/v1/otlp/{metrics|logs|traces}`. The ingestion gateway resolves the issuer/subject/signal grant server-side and never trusts a caller-supplied workspace attribute.

The D1 migration is in `cloudflare/migrations/0001_initial.sql`. It creates isolated tables for workspace membership, workload grants, telemetry, incidents, alerting, notifications, deployments, SLOs, and audit history. Only the telemetry query/ingest slice is implemented in the initial Worker; the operational workflow endpoints are staged in the follow-on milestones.

## Required Worker bindings and variables

- D1 binding `DB` → `orbit-runtime`
- `OIDC_ISSUER` (non-secret issuer URL)
- `OIDC_AUDIENCE` (API audience)
- `ALLOWED_ORIGIN` (default GitHub Pages origin)

Do not commit credentials. OIDC issuer/audience values and the public client configuration must match the real identity provider. No identity provider has been provisioned by this change.

## Deployment status (2026-11-06)

- Worker `orbit-runtime` is deployed at https://orbit-runtime.pooyagame141.workers.dev.
- D1 database `orbit-runtime` is provisioned in the EU jurisdiction and the initial schema migration is applied.
- Worker Logs and Traces are enabled.
- The deployed Worker upload was accepted by Cloudflare. The current verification network blocks direct requests to workers.dev, so external HTTP smoke is still pending; do not claim the endpoint has passed live request tests.
- OIDC variables are intentionally unset until a real provider is configured. Authenticated telemetry routes therefore remain closed by default.

Keep the Worker on the account's no-cost limits; do not enable paid billing or automatic overages.

The Worker uses Cloudflare observability with logs and traces enabled. Never log bearer tokens or telemetry payload bodies.

## Explicit boundaries

- The Apps Script version remains synthetic-only, with simulated notifications and no public OTLP ingestion.
- Cloudflare's first slice stores ingested telemetry in D1 and exposes bounded workspace queries. It is not yet a substitute for Prometheus, Loki, or Tempo at high volume.
- Real webhook delivery, email, alert evaluation, incident operations, SLO evaluation, full audit mutation flows, OIDC provider configuration, and end-to-end frontend runtime selection remain subsequent milestones.
- No real user or customer data should be sent until the OIDC issuer, workload grants, quotas, retention, and security review are configured and tested.
