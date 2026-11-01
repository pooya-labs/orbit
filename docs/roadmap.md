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
2. The initial M06 API security gate from [ADR 0001](decisions/0001-oidc-access-token-authentication.md) is merged in `977ce49`. Shared PostgreSQL-backed IP and subject rate limiting is merged in `db45843` and CI-verified. Provider-neutral browser PKCE is implemented on draft PR #12; it is not enabled on the public site until a real provider is configured and end-to-end validation passes. Workspace membership and tenant-scoped data access remain the next security boundary.
3. Deliver M05 in slices: incident lifecycle and immutable timeline first; alert rules and notification adapters second; deployment annotations third; SLO/error-budget calculations fourth; audit history across every mutation.
4. Complete the remaining M06 operational hardening: retention/cardinality controls, load testing, backup/restore, dependency/security review, production deployment, and runbooks.

M05 mutations must not be exposed until workspace authorization and transactional audit identity are implemented for each operation. The API now enforces bearer authentication and baseline roles, but it does not yet provide tenant membership or audit-backed mutation workflows. Local development can omit OIDC only with an explicit non-production environment; staging and production fail closed.

## M05 — Operations workflows

- [ ] Incident lifecycle and timeline.
- [ ] Alert rules and notification adapters.
- [ ] Deployment annotations and release comparison.
- [ ] SLO definitions and error-budget burn calculations.
- [ ] Audit trail for operator actions.

## M06 — Production hardening

The initial server-side OIDC resource-server and route-role gate and shared PostgreSQL-backed rate limits are implemented and merged. Browser PKCE is implemented in draft PR #12 but is not yet connected to a real provider. Workspace schema, membership/audit repository primitives, authenticated workspace management endpoints, and workload-identity administration APIs (PR #34) are merged. Authenticated OTLP HTTP/gRPC gateways enforce workload identity, workspace grants, and signal permissions. PR #36 verifies forged producer workspace attributes are rejected; PR #38 adds CI-passing mixed-resource batch rejection across all signals and both transports. PR #39 is merged and CI-passing with per-workspace query tenant-filter coverage across Prometheus, Loki, and Tempo. PR #40 is merged and CI-passing with route-level membership-before-backend-access coverage for metrics, logs, and traces. PR #42 is merged and CI-passing with oversized-request rejection for all signals over HTTP/gRPC. PR #43 verifies shared rate limits across gateway instances and transports and is merged with passing CI. PR #46 verifies atomic enforcement under concurrent load across two replicas and is merged with passing CI. PR #48 is merged with passing CI and verifies workspace-grant and workload-identity revocation across independent HTTP/gRPC gateway replicas for metrics, logs, and traces. Tenant-facing production ingestion remains disabled until remaining load/revocation checks, deployment network controls, and operational gates pass.

- [x] Merge and CI-verify OIDC discovery/JWKS access-token verification and server-side viewer/operator/administrator route enforcement (`977ce49`; CI includes Go race tests, web build, and the OTLP end-to-end smoke test).
- [x] Implement provider-neutral browser Authorization Code + PKCE, state/nonce checks, memory-only access/refresh tokens, and API Bearer-token injection (draft PR #12; provider configuration and production rollout remain open).
- [ ] Complete tenant-scoped repository queries for services, incidents, and telemetry before enabling multi-user data access. Migration 006 and membership-checked workspace-scoped inventory/catalog reads are merged and CI-verified. A follow-up gate disables unscoped legacy inventory and telemetry read routes in staging/production while retaining them for development compatibility. Workspace-scoped Prometheus/Loki/Tempo query routes are merged and CI-verified in PR #18; authenticated ingestion remains open.
- [ ] Complete authenticated workspace-aware OTLP ingestion hardening according to [ADR 0002](decisions/0002-authenticated-workspace-otlp-ingestion.md). Workload identity/grants (PR #21), strict RFC 9068 workload-token verification (PR #22), denial audit (PR #23), authorization core (PR #25), canonical OTLP attribution (PR #27), HTTP/gRPC transport handlers (PRs #28–#29), runtime listener wiring, and workload-identity administration/grant APIs (PR #34) are merged. PR #36 verifies forged `orbit.workspace.id` rejection for all signals over HTTP/gRPC; PR #38 is merged and CI-passing with multi-resource mixed-batch rejection before any forwarding. PR #39 verifies per-workspace selectors across Prometheus, Loki, and Tempo; PR #40 verifies non-members are rejected before backend access. Remaining gates: production ingress TLS/network policy, broader load and in-flight revocation-bound tests, real-provider PKCE rollout, and deployment verification. Reserved producer tenant attributes are rejected, never trusted.
- [x] Merge and CI-verify the shared PostgreSQL rate limiter for horizontally scaled API instances (`db45843`); staging/production require PostgreSQL and the limiter stores only hashed short-lived keys.
- [ ] Add retention policies and cardinality guardrails for telemetry and API state.
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


Workspace-scoped telemetry query routes are merged and CI-verified in PR #18. Prometheus, Loki, and Tempo requests are constrained by the verified workspace ID after membership verification. These routes require the canonical `orbit_workspace_id` resource label; ingestion must attach and authenticate that label before tenant telemetry can be enabled. ADR 0002 defines the proposed workload identity, workspace grant, canonical attribution, and private Collector boundary.


## M06 PR #48 revocation verification

PR #48 is merged into `main` (squash merge commit `3c5619d380b498a50660415b68175f8266c48a71`). CI run `41352185701` completed successfully: Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test all passed. The PostgreSQL-backed integration test uses four independent gateway/authorizer instances (two HTTP, two gRPC) and verifies that signal-grant revocation and workload-identity revocation are observed by other replicas for metrics, logs, and traces; denied requests do not reach the Collector forwarders.

The test establishes read-after-commit revocation for requests authorized after the revocation transaction commits. It does not establish cancellation of requests that completed authorization before that commit. In-flight revocation ordering/cancellation remains an explicit open design and test item. Production ingestion remains disabled until TLS/network policy, in-flight semantics, broader load testing, real-provider PKCE, and operational runbooks are complete.


## M06 PR #50 — in-flight revocation contract

- [x] Define and CI-verify in-flight revocation semantics for OTLP/HTTP and OTLP/gRPC (PR #50; merge commit `45680118f44c3cd35f64214072085e6ca1a43de6`; CI run `41352877449` passed all three jobs).
- [x] Establish the authorization boundary: an export already authorized and admitted to the Collector forwarder may finish after a later revocation commit; authorization beginning after the commit must fail closed and must not be forwarded.
- [x] Verify gateway admission-capacity behavior under saturation for OTLP/HTTP and OTLP/gRPC (PR #51; merged and CI-passing). The tests prove a second request is rejected while the single admission slot is occupied, is not forwarded, and the admitted request completes after capacity is released.
- [ ] Add broader burst/load coverage and database/Collector outage, timeout, and backpressure tests.
- [ ] Verify production ingress TLS termination, trusted-proxy behavior, firewall/NetworkPolicy restrictions, and Collector/backend isolation.
- [ ] Configure and end-to-end verify a real OIDC provider for browser PKCE, including callback, state/nonce, issuer/audience validation, and key rotation.
- [ ] Complete dependency/secrets review, telemetry retention/cardinality controls, backup/restore, deployment and incident-response runbooks.

The in-flight contract does not cancel or roll back data already admitted to the Collector. Production tenant ingestion remains disabled until the remaining production gates are verified.


## M06 PR #51 — gateway admission capacity

PR #51 is merged into `main` (squash merge commit `1ff6a8baf80e50910f4c0b41ebaec73b4fdd26ba`). CI run `41353306270` completed successfully across Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

The PostgreSQL-backed integration tests exercise both OTLP/HTTP and OTLP/gRPC with a gateway admission capacity of one. The first export is held inside a blocking Collector forwarder; a second export is rejected with HTTP 503 or gRPC ResourceExhausted and does not reach the forwarder. Once the first export is released, it completes successfully. This verifies the admission-capacity boundary, not a full production load or outage envelope.

Remaining M06 work includes broader burst/load testing, database and Collector outage/timeout behavior, explicit backpressure verification, ingress TLS/network controls, real-provider browser PKCE, retention/cardinality, dependency/secrets review, backup/restore, and operational runbooks. Production tenant ingestion remains disabled.
