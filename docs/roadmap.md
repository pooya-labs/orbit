# ORBIT Roadmap

A milestone is complete only when its acceptance criteria are implemented and verified.

## M01 — Public workspace

- [x] Establish Next.js + TypeScript app structure.
- [x] Build responsive observability overview.
- [x] Add service search, chart range controls, and mobile navigation.
- [x] Label all illustrative telemetry as demo data.
- [x] Verify production type-check and build.
- [x] Configure static export for GitHub Pages.
- [x] Add automated Pages deployment.
- [ ] Accessibility review with keyboard and screen readers.

## M02 — API and persistence foundation

- [x] Create Go API module and server lifecycle.
- [x] Add liveness, readiness, and metadata endpoints.
- [x] Add request IDs, structured HTTP logs, safe error envelopes, and panic recovery.
- [x] Add PostgreSQL connection pooling and readiness probe.
- [x] Add versioned initial migration and schema migration ledger.
- [x] Add service and incident read endpoints.
- [x] Add PostgreSQL-backed integration tests in CI.
- [x] Publish OpenAPI 3.1 contract.
- [x] Add typed frontend API client and explicit API error types.
- [ ] Add a separate demo-data provider and connect the dashboard through a source switch.
- [ ] Connect the public dashboard to a hosted API after backend deployment and CORS/auth design.

## M03 — Local platform

- [x] Add Docker Compose for PostgreSQL, Redis, and NATS JetStream.
- [x] Add the Go API container and database readiness dependency.
- [x] Add deterministic, clearly labelled demo seed rows.
- [ ] Add OpenTelemetry Collector and Prometheus.
- [ ] Add Loki and Tempo development profiles.
- [ ] Add service instrumentation example with OTLP export.
- [ ] Wire Redis cache and NATS event publishing into the API where justified.

## M04 — Real telemetry

- [ ] Ingest OTLP metrics, traces, and logs.
- [ ] Normalize service/resource attributes.
- [ ] Build service catalog from observed telemetry.
- [ ] Implement bounded telemetry queries and pagination.
- [ ] Display data freshness, gaps, and source provenance.

## M05 — Operations workflows

- [ ] Incident lifecycle and timeline.
- [ ] Alert rules and notification adapters.
- [ ] Deployment annotations and release comparison.
- [ ] SLO definitions and error-budget burn calculations.
- [ ] Audit trail for operator actions.

## M06 — Production hardening

- [ ] Workspace authentication and role-based access control.
- [ ] Rate limits, retention policies, and cardinality guardrails.
- [ ] Load tests for ingestion and dashboard queries.
- [ ] Threat model, dependency review, and secret scanning.
- [ ] Production deployment guide and operational runbooks.
- [ ] Hosted API and managed persistence deployment.

## Definition of done

- Type-check, unit tests, integration tests, and production builds pass.
- Empty, loading, stale, partial, and error states are covered.
- No secrets or real customer telemetry are committed.
- Mobile and desktop layouts remain usable.
- Documentation describes what is implemented, not only what is intended.
