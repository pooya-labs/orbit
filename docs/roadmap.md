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

The browser dashboard reads service and incident inventories from a configured API endpoint. The telemetry API now also exposes bounded metrics, logs, and traces, which are rendered in the dedicated investigation views.

- [x] Create Go API module and server lifecycle.
- [x] Add liveness, readiness, and metadata endpoints.
- [x] Add request IDs, structured HTTP logs, safe error envelopes, and panic recovery.
- [x] Add PostgreSQL connection pooling and readiness probe.
- [x] Add versioned initial migration and schema migration ledger.
- [x] Add service and incident read endpoints.
- [x] Add PostgreSQL-backed integration tests in CI.
- [x] Publish OpenAPI 3.1 contract.
- [x] Add typed frontend API client and explicit API error types.
- [x] Separate demo fixtures from the dashboard and add an explicit Demo / ORBIT API source switch with connection validation.
- [ ] Connect the public dashboard to a hosted API after backend deployment and CORS/auth design.

## M03 — Local platform

The optional `observability` profile accepts OTLP over gRPC/HTTP and routes metrics to Prometheus, traces to Tempo, and logs to Loki. An instrumented Go demo emits all three signals; GitHub Actions verifies the complete path through the backends and ORBIT API. The production Go API remains uninstrumented.

- [x] Add Docker Compose for PostgreSQL, Redis, and NATS JetStream.
- [x] Add the Go API container and database readiness dependency.
- [x] Add deterministic, clearly labelled demo seed rows.
- [x] Add an optional OpenTelemetry Collector and Prometheus profile with a local smoke test.
- [x] Add Loki and Tempo local backends with OTLP export pipelines.
- [x] Add an instrumented Go service example with OTLP export for metrics, traces, and logs, plus an end-to-end backend smoke test.
- [ ] Wire Redis cache and NATS event publishing into the API where justified.

## M04 — Real telemetry

The v0.6.0 API queries the local Prometheus, Loki, and Tempo stores through bounded, source-labelled endpoints. The v0.7.0 dashboard renders Metrics, Logs, and Traces views with selectable time windows, refresh, query timestamps, and explicit loading/empty/error states. The v0.8.0 preview adds merged service discovery with backend provenance and a shared dynamic selector. The v0.9.0 preview persists observed services and source relationships in PostgreSQL and adds Persian localization with RTL layout. Queries are fixed and do not accept arbitrary PromQL/LogQL from callers; the current request-rate metric is the instrumented demo counter.

- [x] Verify OTLP metrics, traces, and logs reach all three local backends from an instrumented workload.
- [x] Add bounded ORBIT API queries for Prometheus request-rate metrics, Loki service logs, and Tempo trace search.
- [x] Discover service names across Prometheus, Loki, and Tempo, merge duplicates, and preserve backend provenance.
- [x] Persist observed service names and backend provenance in a normalized PostgreSQL catalog.
- [x] Expose persisted first-seen, last-seen, and last-discovered metadata through the service discovery API.
- [ ] Populate the full OpenTelemetry resource attribute set from instrumented workloads (the current preview extracts a bounded allowlist of labels and tags from Prometheus, Loki, and Tempo).
- [x] Persist bounded service resource attributes and per-attribute backend provenance in PostgreSQL.
- [x] Add validated cursor-based pagination to the service catalog API and dashboard selector.
- [x] Add allow-listed multi-series grouping and separate line rendering for the request-rate metric; arbitrary metric-name selection remains future work.
- [x] Render queried telemetry in Metrics, Logs, and Traces dashboard views with loading/empty/error states, query freshness, and backend provenance.
- [x] Flag stale metric samples and gaps using the actual Prometheus sample timestamps and query step; event-driven logs and traces are not judged against metric scrape intervals.

## Execution order and dependency gates

ORBIT's milestones are not a reason to expose unsafe operations early. The delivery order is:

1. Finish and merge the verified M04 read-path work. Keep complete workload-side OpenTelemetry resource ingestion as an explicit M04 follow-up.
2. Implement the M06 security gate (OIDC/JWT verification, server-side RBAC, actor identity, request limits, and workspace boundary) before adding public state-changing operations.
3. Deliver M05 in slices: incident lifecycle and immutable timeline first; alert rules and notification adapters second; deployment annotations third; SLO/error-budget calculations fourth; audit history across every mutation.
4. Complete the remaining M06 operational hardening: retention/cardinality controls, load testing, backup/restore, dependency/security review, production deployment, and runbooks.

M05 mutations must not be publicly enabled while the API has no authentication or role enforcement. Local development can keep an explicit non-production mode, but production configuration must fail closed.

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
