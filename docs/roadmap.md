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
2. The initial M06 API security gate from [ADR 0001](decisions/0001-oidc-access-token-authentication.md) is merged in `977ce49`. Shared PostgreSQL-backed IP and subject rate limiting is merged in `db45843` and CI-verified. Provider-neutral browser PKCE is implemented in PR #12; it remains isolated from the public site until a real provider is configured and end-to-end validation passes. Workspace membership and tenant-scoped data access remain the next security boundary.
3. Deliver M05 in slices: incident lifecycle and immutable timeline first; alert rules and notification adapters second; deployment annotations third; SLO/error-budget calculations fourth; audit history across every mutation.
4. Complete the remaining M06 operational hardening: retention/cardinality controls, load testing, backup/restore, dependency/security review, production deployment, and runbooks.

M05 state-changing operations are exposed only through authenticated workspace-scoped routes. The backend verifies workspace membership and operator roles, validates state transitions, and records incident and alert-rule-definition mutations in workspace audit history within the same transaction. The dashboard must keep these controls read-only until a real browser OIDC provider is configured; local development may omit OIDC only in explicit non-production mode.

## M05 — Operations workflows

- [x] Backend incident lifecycle API and immutable timeline: workspace-scoped creation, validated status transitions/reopen, and atomic workspace audit (PR #66; migration 009; CI passing).
- [x] Workspace-scoped dashboard for incident creation, allowed status transitions/reopen, detail, and immutable timeline review (PR #71; CI-verified).
- [x] Workspace-scoped alert rule definition CRUD with immutable workspace audit (PR #69; migration 010; CI passing). v1 is limited to the existing `request_rate` metric and rejects arbitrary query expressions.
- [x] Add a scheduled workspace-aware evaluator for the allow-listed `request_rate` metric, with persisted pending/firing/resolved/cancelled instances, append-only events, deduplication, telemetry-gap handling, and PostgreSQL advisory-lock coordination across API replicas (PR #72; migration 011; CI-verified).
- [ ] Complete secure webhook notification delivery and retry history (implementation underway on `feat/m05-alert-notification-delivery`; migration 012).
- [ ] Deployment annotations and release comparison.
- [ ] SLO definitions and error-budget burn calculations.
- [ ] Extend audit-backed history to every operator mutation and workflow.

## M06 — Production hardening

The initial server-side OIDC resource-server and route-role gate and shared PostgreSQL-backed rate limits are implemented and merged. Browser PKCE is implemented in PR #12 but is not yet connected to a real provider or enabled on the public site. Workspace schema, membership/audit repository primitives, authenticated workspace management endpoints, and workload-identity administration APIs (PR #34) are merged. Authenticated OTLP HTTP/gRPC gateways enforce workload identity, workspace grants, and signal permissions. PR #36 verifies forged producer workspace attributes are rejected; PR #38 adds CI-passing mixed-resource batch rejection across all signals and both transports. PR #39 is merged and CI-passing with per-workspace query tenant-filter coverage across Prometheus, Loki, and Tempo. PR #40 is merged and CI-passing with route-level membership-before-backend-access coverage for metrics, logs, and traces. PR #42 is merged and CI-passing with oversized-request rejection for all signals over HTTP/gRPC. PR #43 verifies shared rate limits across gateway instances and transports and is merged with passing CI. PR #46 verifies atomic enforcement under concurrent load across two replicas and is merged with passing CI. PR #48 is merged with passing CI and verifies workspace-grant and workload-identity revocation across independent HTTP/gRPC gateway replicas for metrics, logs, and traces. PR #53 is merged with passing CI and adds gRPC fail-closed coverage for shared rate-limit store outages, Collector errors, and forward deadlines. Tenant-facing production ingestion remains disabled until remaining load/failure checks, deployment network controls, and operational gates pass.

- [x] Merge and CI-verify OIDC discovery/JWKS access-token verification and server-side viewer/operator/administrator route enforcement (`977ce49`; CI includes Go race tests, web build, and the OTLP end-to-end smoke test).
- [x] Implement provider-neutral browser Authorization Code + PKCE, state/nonce checks, memory-only access/refresh tokens, and API Bearer-token injection (PR #12; provider configuration and production rollout remain open).
- [x] Complete workspace-membership-scoped inventory, incident, telemetry-catalog, and Prometheus/Loki/Tempo query routes, with legacy global reads disabled in staging/production (migration 006; PRs #16–#18, #39–#40; CI-verified).
- [x] Implement authenticated workspace-aware OTLP ingestion with workload identity, per-signal workspace grants, canonical tenant attribution, bounded HTTP/gRPC gateways, shared rate limits, revocation checks, and fail-closed behavior (PRs #21–#56; CI-verified).
- [ ] Complete production rollout of the implemented ingestion security boundary: verify real DNS/ACME issuance, host firewall and trusted-proxy policy, deployment-specific network controls, real-provider browser PKCE, sustained multi-replica load/recovery, and operational runbooks. PR #56 supplies an optional Caddy TLS edge-ingress scaffold, not a live deployment. Production tenant ingestion remains disabled until these environment-level gates pass. Reserved producer tenant attributes are rejected, never trusted.
- [x] Merge and CI-verify the shared PostgreSQL rate limiter for horizontally scaled API instances (`db45843`); staging/production require PostgreSQL and the limiter stores only hashed short-lived keys.
- [ ] Add retention policies and cardinality guardrails for telemetry and API state.
- [x] Add deterministic in-process sustained-concurrency tests and repeatable allocation/throughput benchmarks for authenticated OTLP HTTP/gRPC gateway paths; see [M06 load-testing guide](m06-load-testing.md).
- [ ] Run and record target-environment load tests for sustained ingestion and dashboard queries with real PostgreSQL and Collector dependencies; capture throughput, p50/p95/p99 latency, rejection rates, saturation, and recovery.
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
- [x] Verify gRPC fail-closed behavior when the shared rate-limit store is unavailable, map Collector failures to Unavailable, and map forward deadlines to DeadlineExceeded (PR #53; merged and CI-passing).
- [x] Verify deterministic recovery after transient workload-grant database, shared rate-limit-store, and Collector failures for HTTP and gRPC using the same long-lived gateway instance (PR #58; merged and CI-passing).
- [x] Verify the default 64-slot admission bound under a synchronized HTTP/gRPC burst, reject requests over capacity before Collector forwarding, and release admitted requests cleanly (PR #60; merged and CI-passing).
- [x] Add deterministic sustained-concurrency tests and handler-path throughput/allocation benchmarks for OTLP HTTP and gRPC; see [M06 load-testing guide](m06-load-testing.md).
- [x] Verify recovery after real local Collector listener stop/restart (PR #62) and PostgreSQL TCP-path disconnect/restoration (PR #64). These are local integration fault tests, not container/process restart or production network-partition tests.
- [ ] Run sustained target-environment load tests with real PostgreSQL and Collector dependencies, then capture throughput, latency percentiles, saturation, and rejection behavior.
- [ ] Add explicit backpressure and recovery-under-sustained-load tests across the real dependency boundary.
- [x] Add an optional Caddy TLS edge-ingress Compose overlay for OTLP/HTTP and OTLP/gRPC (PR #56; merged with CI passing). CI validates the Caddyfile, the rendered Compose topology, and that only the edge proxy publishes ingress ports.
- [ ] Deploy and verify the ingress scaffold with real DNS/ACME certificates, host-firewall rules, trusted-proxy behavior, and deployment-specific NetworkPolicy/cloud controls; assess internal-hop mTLS.
- [ ] Configure and end-to-end verify a real OIDC provider for browser PKCE, including callback, state/nonce, issuer/audience validation, and key rotation.
- [ ] Complete dependency/secrets review, telemetry retention/cardinality controls, backup/restore, deployment and incident-response runbooks.

The in-flight contract does not cancel or roll back data already admitted to the Collector. Production tenant ingestion remains disabled until the remaining production gates are verified.


## M06 PR #51 — gateway admission capacity

PR #51 is merged into `main` (squash merge commit `1ff6a8baf80e50910f4c0b41ebaec73b4fdd26ba`). CI run `41353306270` completed successfully across Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

The PostgreSQL-backed integration tests exercise both OTLP/HTTP and OTLP/gRPC with a gateway admission capacity of one. The first export is held inside a blocking Collector forwarder; a second export is rejected with HTTP 503 or gRPC ResourceExhausted and does not reach the forwarder. Once the first export is released, it completes successfully. This verifies the admission-capacity boundary, not a full production load or outage envelope.

Remaining M06 work includes broader burst/load testing, database and Collector outage/timeout behavior, explicit backpressure verification, ingress TLS/network controls, real-provider browser PKCE, retention/cardinality, dependency/secrets review, backup/restore, and operational runbooks. Production tenant ingestion remains disabled.


## M06 PR #53 — gRPC dependency and timeout behavior

PR #53 is merged into `main` (squash merge commit `13cd7b4927b2259b10b5dd94824a70ee11541069`). CI run `41353731713` completed successfully across Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

The gRPC gateway now has explicit tests proving that an unavailable shared rate-limit store fails closed with `Unavailable` and does not forward telemetry, Collector failures map to `Unavailable`, and a forward deadline maps to `DeadlineExceeded`. This is deterministic protocol/error-contract coverage; broader outage and load behavior remains open.

PR #54 is merged with passing CI. Collector forward timeouts now return HTTP 504 Gateway Timeout rather than the generic 502 used for other Collector failures.


## M06 PR #56 — optional TLS edge-ingress scaffold

PR #56 is merged into `main` (squash merge commit `45997fd2ec508e260a51a027208152c9b155ac0f`). CI run `41354509326` passed all three jobs: Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test. The ingress validation also passed: Caddy accepted the configuration, the merged Compose model was rendered with both observability and production-ingress profiles, only Caddy publishes 80/443, API OTLP listeners remain unpublished, raw Collector receivers remain unpublished, and Caddy is isolated from the default application and telemetry networks.

The optional overlay routes separate OTLP/HTTP and OTLP/gRPC domains through Caddy TLS termination to the authenticated ORBIT gateways. It is a deployment scaffold, not a completed production rollout: real DNS/ACME issuance, host firewall rules, provider configuration, trusted-proxy validation, internal-hop mTLS assessment, production secrets, and deployment-specific network policies remain open. Production tenant ingestion remains disabled.


## M06 PR #58 — dependency failure recovery

PR #58 is merged into `main` (squash merge commit `fd7c7d23d952bdb6f08250c2dcd699c2816620d3`). CI run `41355041031` passed all three jobs: Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

The new HTTP and gRPC tests inject transient failures into the workload-grant repository, shared rate-limit store, and Collector forwarder. They verify that authorization/rate-limit database failures fail closed without forwarding, Collector failures map to the established transport status, and a subsequent request succeeds through the same gateway instance after the dependency is restored.

These are deterministic in-process fault-injection tests. PR #62 separately adds real local TCP listener stop/restart coverage for both production Collector forwarders; it does not simulate Docker/container orchestration, packet loss, PostgreSQL process restarts, or sustained-load recovery. Broader sustained-load/burst benchmarking, PostgreSQL process/container restart testing, network-partition scenarios beyond the test TCP proxy, and backpressure behavior remain open. Production tenant ingestion remains disabled.


## M06 PR #60 — default gateway burst-capacity boundary

PR #60 is merged into `main` (squash merge commit `cb85c0afea6647ecc59b9fe59e499ce8383fc526`). CI run `41355534531` passed all three jobs. New synchronized tests hold all 64 default HTTP or gRPC admission slots inside blocking Collector forwarders, then verify additional HTTP requests receive 503 and additional gRPC requests receive ResourceExhausted without reaching the forwarder. After capacity is released, all admitted requests complete and the active-forward count returns to zero.

This verifies the configured in-process admission boundary under a deterministic burst. It is not a throughput benchmark or sustained production load test; broader load, network/process fault injection, and backpressure remain open.


## M06 PR #62 — real Collector listener restart recovery

PR #62 is merged into `main` (squash merge commit `534f6d02c04616a5d27831460b24fd87db8bfade`). CI run `41357122909` passed all three jobs: Go API tests/build (including race tests), web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

The new HTTP test starts a real local TCP OTLP/HTTP endpoint behind the production Collector HTTP forwarder, verifies success, stops the listener and verifies the gateway returns 502, restarts a listener on the same address, and verifies the same long-lived gateway/forwarder succeeds again. The gRPC test performs the equivalent listener stop/restart using a real gRPC Collector server and the production gRPC forwarder; the same client connection eventually reconnects and succeeds.

This validates process-local TCP listener recovery and client reconnection behavior. It does not simulate container orchestration, packet loss or network partitions, database process restarts, cross-zone failures, or recovery under sustained load. Real PostgreSQL restart/fault injection, longer-duration load and backpressure, and production deployment verification remain open. Production tenant ingestion remains disabled.


## M06 PR #64 — PostgreSQL pool recovery after network interruption

PR #64 is merged into `main` (squash merge commit `6b2e3418be2e8fdff0c9634b361acaa5bb6d6da1`). CI run `41357596656` passed all three jobs, including Go race tests/build, web typecheck/production build, and the telemetry end-to-end smoke test.

The new PostgreSQL integration test opens the real repository through a local TCP proxy, warms an established pgxpool connection, then closes active TCP tunnels and rejects new dials while the test network path is offline. The existing repository must report a Ping failure. The proxy is then restored and the same pgxpool/repository must reconnect and return to healthy Ping without being recreated.

This proves connection-pool recovery after a real TCP path interruption against the CI PostgreSQL service. The database process remains running; PostgreSQL process/container restart, packet loss and partition simulation, prolonged outage behavior, sustained request load, and backpressure remain open.


## M05 PR #66 — workspace incident lifecycle backend

PR #66 is merged into `main` (squash merge commit `7a57dd4ef8752e8e18d6dec5f0e75112637167dd`). CI run `41360316146` passed all three jobs: Go API race tests/build and vet, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

Migration 009 adds a workspace-scoped incident timeline with a composite foreign key back to the incident/workspace pair. Database triggers reject UPDATE, DELETE, and TRUNCATE. The PostgreSQL Repository provides transactional create and status-transition commands. Every mutation writes its timeline event and workspace audit event in the same transaction. Owners, administrators, and operators may create/change incidents; any workspace member may read the timeline.

The supported state transitions are: open → investigating/monitoring/resolved; investigating → monitoring/resolved; monitoring → investigating/resolved; resolved → investigating (reopen). Resolving sets `resolved_at`; reopening clears it. The authenticated API exposes POST workspace incidents, PATCH incident status, and GET incident timeline routes. The OpenAPI contract describes these endpoints and request/response schemas.

This is the backend API and persistence slice only. Dashboard incident controls/detail/timeline UI, alert rules/notifications, deployment annotations, SLO/error-budget calculations, and wider audit workflows remain open. M05 is not complete.


## M05 PR #69 — workspace alert rule definitions

PR #69 is merged into `main` (squash merge commit `64c4c4f1d36fd0c17ae444ac5736fadad0a32655`). CI run `41361579435` passed all three jobs: Go API race tests/build and vet, web typecheck/production build, and the telemetry end-to-end smoke test.

Migration 010 adds workspace-owned alert rule definitions with a case-insensitive unique name per workspace and database constraints for the supported metric, operators, threshold, evaluation window, pending duration, severity, and description bounds. The API supports list, create, replace, enable/disable, and delete. Owner/administrator/operator roles may mutate; workspace members may list. Every mutation and its append-only workspace audit event are committed in one transaction.

Version 1 deliberately supports only `request_rate`, the metric currently emitted by the instrumented demo. It accepts fixed comparison operators and bounded settings, not arbitrary PromQL/LogQL. PR #69 delivered the workspace-scoped definitions; PR #72 adds scheduled evaluation, durable lifecycle instances, deduplication, and multi-replica coordination. Notification delivery/retry remains open.
