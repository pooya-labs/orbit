# ORBIT Roadmap

The roadmap is milestone-based. A milestone is complete only when its acceptance criteria are verified in code.

## M01 — Workspace foundation

- [x] Establish Next.js + TypeScript app structure.
- [x] Build responsive observability overview.
- [x] Add service search, time-range controls, and mobile navigation.
- [x] Label all illustrative telemetry as demo data.
- [ ] Verify production build and type-check in CI.
- [ ] Add accessible keyboard and screen-reader review.

## M02 — API foundation

- [ ] Create Go API module and health/readiness endpoints.
- [ ] Define versioned OpenAPI contract for services and incidents.
- [ ] Add typed frontend API client and request error model.
- [ ] Add integration tests for API contracts.
- [ ] Replace dashboard fixtures with a named demo provider behind the same interfaces used by live data.

## M03 — Local platform

- [ ] Add Docker Compose for PostgreSQL, Redis, NATS, and OpenTelemetry Collector.
- [ ] Add Prometheus, Loki, and Tempo local development profiles.
- [ ] Add deterministic seed data and documented reset commands.
- [ ] Add service instrumentation example with OTLP export.

## M04 — Real telemetry

- [ ] Ingest OTLP metrics, traces, and logs.
- [ ] Normalize service/resource attributes.
- [ ] Build service catalog from observed telemetry.
- [ ] Implement query endpoints with bounded time ranges and pagination.
- [ ] Display data freshness and source provenance.

## M05 — Operations workflows

- [ ] Incident lifecycle and timeline.
- [ ] Alert rules and notification adapters.
- [ ] Deployment annotations and release comparison.
- [ ] SLO definitions and error-budget burn calculations.
- [ ] Audit trail for operator actions.

## M06 — Hardening

- [ ] Workspace authentication and role-based access control.
- [ ] Rate limits, retention policies, and cardinality guardrails.
- [ ] Load tests for ingestion and dashboard queries.
- [ ] Threat model, dependency review, and secret scanning.
- [ ] Production deployment guide and operational runbooks.

## Definition of done

- Type-check and production build pass.
- Relevant automated tests pass.
- Empty, loading, stale, partial, and error states are covered.
- No secrets or real customer telemetry are committed.
- Mobile and desktop layouts remain usable.
- Documentation describes what is implemented, not only what is intended.
