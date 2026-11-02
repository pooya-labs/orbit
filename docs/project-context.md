# ORBIT — Project Context & Handoff Notes

This document is the durable project backstory for future contributors and future planning sessions. It records the product intent, implementation history, boundaries, and decisions that should not be accidentally undone.

## 1. Product intent

ORBIT is an engineering-first observability control plane for distributed services. It is intended to help an operator move from service health to metrics, logs, traces, incidents, deployments, and reliability objectives while preserving the origin and freshness of every observation.

It is not a generic dashboard filled with synthetic graphs, and it is not a replacement for Prometheus, Loki, Tempo, or an OpenTelemetry Collector. Those systems remain the telemetry pipeline and stores; ORBIT provides a cohesive operator workspace, bounded API, service catalog, and eventually controlled operations workflows.

## 2. Project history reconstructed from the repository

The repository documents the following progression:

1. **v0.1.0 — Public workspace:** a responsive Next.js/TypeScript observability interface, deployed as a static GitHub Pages site. Initial charts and overview fixtures are explicitly illustrative.
2. **v0.2.0 — Backend foundation:** Go HTTP API, PostgreSQL-backed service and incident read models, OpenAPI contract, typed frontend client, and Docker Compose.
3. **v0.3.0 — Demo/API separation:** the workspace gained an explicit source switch so illustrative fixtures are not silently mixed with API data.
4. **v0.4.0 — Local observability stack:** an optional OpenTelemetry Collector, Prometheus, Loki, and Tempo pipeline.
5. **v0.5.0 — Instrumented workload:** a Go demo service emits a metric, trace, and structured log using OTLP; CI verifies delivery to the three backends.
6. **v0.6.0 — Bounded telemetry API:** service-scoped read endpoints for Prometheus metrics, Loki logs, and Tempo trace search.
7. **v0.7.0 — Live investigation workspace:** dedicated Metrics, Logs, and Traces views with selectable time windows, refresh, query timestamps, and loading/empty/error states.
8. **v0.8.0 — Cross-backend service discovery:** services are discovered from all three stores, duplicates are merged, backend provenance is preserved, and the views share one service selector.
9. **v0.9.0 — Persisted catalog and localization:** a PostgreSQL telemetry-service catalog, first/last-seen metadata, creator attribution, and English/Persian UI with RTL support.
10. **M04 follow-through:** bounded resource-attribute discovery and provenance, cursor-based catalog pagination, allow-listed multi-series request-rate grouping, and Prometheus sample freshness/gap warnings.

The version labels above describe the progression documented by the project; they should not be interpreted as proof that the public static site contains the unreleased M04 follow-through.

## 3. Product and deployment boundaries

- **GitHub Pages is frontend-only.** It serves the static workspace. It does not host the Go API, PostgreSQL, Redis, NATS, or telemetry backends.
- **Demo mode and API mode are separate.** Demo fixtures must remain visibly illustrative. A real API response must never be silently replaced by synthetic values.
- **The Go API and data stack are currently local-development infrastructure.** Docker Compose binds local development ports to loopback. The unauthenticated local stack must not be exposed to a public network.
- **The optional telemetry profile** routes OTLP metrics to Prometheus, traces to Tempo, and logs to Loki. The instrumented demo workload is the verified end-to-end source. The production API itself is not yet instrumented.
- **Prometheus, Loki, and Tempo remain the source of raw telemetry.** PostgreSQL stores service identities, source observations, discovery metadata, and bounded resource attributes; it is not intended to become a second raw metrics/logs/traces store.
- **Telemetry query APIs stay bounded and read-only.** Windows, result counts, dimensions, and query expressions are validated. Never accept arbitrary PromQL or LogQL directly from the browser.
- **M04 attribute extraction is still a preview.** It maps an allowlist of labels/tags into OpenTelemetry-style names. It does not yet ingest the complete resource-attribute set directly from workload SDKs or the Collector.
- **Redis and NATS are provisioned but not yet integrated into application behavior.** Do not claim caching or event delivery until the API actually uses them and tests cover those paths.
- **M06 API and browser security foundation:** OIDC access-token verification through issuer discovery/JWKS, RS256/ES256 allowlisting, centralized viewer/operator/administrator route policy, explicit development mode, fail-closed staging/production startup, body/token caps, and PostgreSQL-backed shared per-IP/per-subject rate limits are merged and CI-verified. The provider-neutral browser Authorization Code + PKCE (S256) client is implemented in PR #12: state/nonce and ID-token signature/issuer/audience validation are enforced, API calls receive an in-memory Bearer token, and access/refresh tokens are not persisted. The browser sign-in controls stay hidden unless a real issuer and public client are configured at build time.
- **Security and production work still open:** real OIDC provider registration and browser/API end-to-end verification (including callback URI, token-endpoint CORS and API audience), provider token revocation/global logout, real DNS/ACME and host/network policy validation, telemetry retention/cardinality, dependency/secrets review, backup/restore, and operational runbooks. Workspace membership, workspace-scoped data reads, authenticated workspace-aware OTLP ingestion and transactional audit for incident/alert-rule-definition mutations are implemented; production tenant ingestion remains disabled until environment-level gates pass.

## 4. Established UX and engineering decisions

- Keep the workspace responsive and usable on desktop and mobile.
- Preserve English and Persian localization, including true RTL layout for Persian. New user-facing copy must be added in both languages.
- Preserve the dark, information-dense observability visual system. New controls should be consistent with existing spacing, typography, focus states, and responsive behavior.
- Show data provenance, query time, partial backend failures, and stale-data conditions. Never present missing telemetry as zero without an explicit reason.
- Keep service names and resource attributes as data, not executable query fragments.
- Use server-side allowlists for dimensions and fixed query templates. Frontend restrictions alone are not a security boundary.
- Bound database and backend calls with request-scoped timeouts, cap response sizes and result counts, and return safe errors with request IDs.
- Test the real integration path in CI when a feature changes it. A frontend build alone does not prove the telemetry pipeline works.
- Roadmap items are checked only when implementation and acceptance tests exist. An item described as “preview” must not be rewritten as full support.

## 5. M04 handoff state

M04 is substantially complete for the current preview. The latest verified CI run covers:
- Go tests, race detector, vet, and build;
- frontend typecheck and production build;
- the local OTLP → Collector → Prometheus/Loki/Tempo → ORBIT API end-to-end smoke test.

Remaining M04 work:
- full OpenTelemetry resource attributes from workload instrumentation, with semantic-convention mapping and provenance;
- arbitrary metric-name discovery and selection, after a safe metrics model and cardinality budget are defined;
- user-facing access to individual traces/log context can be improved in later UX iterations.

Do not widen query access to arbitrary PromQL merely to complete metric selection.

## 6. M05 delivery sequence

M05 adds operator workflows. These are state-changing capabilities and must not be exposed publicly before the security gate in section 7.

1. **Incident lifecycle and timeline:** define allowed state transitions, append-only timeline events, assignment/acknowledgement, resolution metadata, and optimistic concurrency. Every mutation must be auditable.
2. **Alert rules and notification adapters:** store versioned rules, validate a bounded expression model, evaluate rules in a worker (not inside an HTTP request), deduplicate firing alerts, support acknowledgement/silence/cooldown, and send through pluggable adapters. Secrets belong in environment/secret management, never database plaintext or source control.
3. **Deployment annotations and release comparison:** record service, environment, version, commit SHA, deployment time, actor, and optional change reference; correlate annotations with telemetry without inventing causal attribution.
4. **SLOs and error budgets:** define an SLI from an explicit numerator/denominator and valid-event policy, target, and rolling period. Calculate remaining budget and burn rate using documented math and test boundary cases.
5. **Audit trail:** append-only records for actor, role, action, entity, request ID, timestamp, and a redacted before/after representation. Audit writes must be transactional with the state change where possible.

M05 acceptance requires API and database tests, lifecycle-transition tests, worker idempotency tests, and operator-visible history. A UI-only mock does not complete an operations workflow.

## 7. M06 security gate and production hardening

The detailed proposed design is recorded in [ADR 0001 — OIDC access-token validation and API authorization](decisions/0001-oidc-access-token-authentication.md). It deliberately distinguishes OAuth access tokens from OIDC ID tokens and sets a Go 1.24-compatible JOSE dependency constraint.

The initial server-side authentication gate is implemented in `main` (`977ce49`) and CI-verified. It validates OIDC access-token signatures and claims against issuer discovery/JWKS, applies viewer/operator/administrator route policy, rejects missing credentials in staging/production, bounds request bodies and token sizes, and enforces PostgreSQL-backed shared per-IP/per-subject limits (`db45843`).

The remaining security work before multi-user or M05 operations deployment is:

- real OIDC provider registration, callback/CORS/audience end-to-end verification, and provider token revocation/global logout (the provider-neutral PKCE client is implemented in PR #12 but is not configured for the public site);
- workspace/tenant membership and database scoping;
- actor identity and transactional audit records for every state-changing operation;
- bounded backend concurrency for horizontally scaled deployments;
- secret management, CORS allowlists, dependency review, backups/retention, cardinality guardrails, and operational runbooks.

Local development may omit OIDC only when `ORBIT_ENVIRONMENT=development` is explicitly set. Startup must fail closed if staging/production is configured without valid authentication settings. Health/readiness endpoints should reveal status, not credentials or sensitive topology.

## 8. Related portfolio context

The GitHub account also contains separate portfolio projects (Pooya.OS, AuraPulse, NexusCanvas, and LuminaGlass). ORBIT is its own repository and backend architecture, not a shared monorepo. Reuse visual/design conventions where appropriate, but do not couple their runtime state, APIs, or deployment pipelines without an explicit product decision.

## 9. M06 security handoff

The initial server-side OIDC access-token and RBAC gate is merged and CI-verified. The provider-neutral browser Authorization Code + PKCE implementation is in PR #12; it adds S256, state/nonce validation, ID-token signature/issuer/audience verification and an in-memory token lifecycle. A real issuer/client, callback and token-endpoint CORS, API audience alignment, provider revocation/global logout, production deployment, retention, dependency/secrets review, backup/restore and runbooks remain open.

The API accepts access tokens only through `Authorization: Bearer`; it never reads credentials from URLs or persistent browser storage. PR #12 adds the static-dashboard PKCE client. The short-lived PKCE verifier/state/nonce transaction uses `sessionStorage`; access and refresh tokens remain in JavaScript memory. A reload clears the session. No real issuer/client is configured yet, so the public preview does not show sign-in controls by default.

## 10. Continuity rule

When resuming work, start with:
1. `docs/roadmap.md` for milestone status;
2. this file for project history and boundaries;
3. `docs/architecture.md` for current system shape;
4. the latest GitHub Actions run and open PRs for verified code state.

Then continue on a feature branch, update tests and docs with implementation, and keep the public Pages deployment unchanged until the relevant PR is merged and deployment CI succeeds.


## 11. Workspace and audit foundation (M06)

The shared PostgreSQL rate limiter is merged. The provider-neutral browser PKCE implementation is in PR #12; public sign-in remains disabled until a real identity provider, public client, callback URL, CORS policy and API audience are configured and verified.

Migration 005 introduces workspaces, issuer+subject-based memberships, one-owner-per-workspace enforcement, and append-only workspace audit events. Repository operations create a workspace and its owner membership atomically, resolve a principal's memberships, and append/read bounded audit events. Authenticated workspace create/list, membership lookup, and membership-checked audit-read endpoints are being added on `feature/m06-workspace-api-membership`.

This schema and repository layer alone does **not** make ORBIT multi-tenant. Existing service, incident, and telemetry queries are still global. Do not expose workspace creation or claim tenant isolation until every data query and mutation is scoped to a verified workspace membership, existing records are safely assigned, and cross-workspace denial tests pass.


## Workspace data-scope foundation (PR in progress)

Migration 006 assigns pre-existing service, incident, and telemetry-catalog rows to a dedicated `legacy-default` workspace. It adds `workspace_id` foreign keys and lookup indexes; incidents also have a composite foreign key to ensure their service belongs to the same workspace. New repository methods and authenticated routes read services, incidents, and persisted telemetry catalog strictly by workspace after issuer+subject membership verification.

This is an incremental isolation boundary, **not completion of multi-tenancy**. Existing unscoped endpoints remain for backward compatibility, telemetry backend queries (Prometheus/Loki/Tempo) are not yet partitioned by workspace, and legacy telemetry ingestion is intentionally pinned to `legacy-default`. Do not enable tenant-facing production access until legacy routes are retired or explicitly restricted, telemetry backend labels are workspace-aware, and all write/ingestion paths enforce workspace ownership.


## Legacy global read-route safety gate (PR in progress)

The follow-up security slice disables the unscoped `/api/v1/services`, `/api/v1/incidents`, and legacy telemetry catalog/metrics/logs/traces read routes in staging and production. They remain available in explicit development mode for local dashboard compatibility. Workspace management and membership-checked workspace-scoped inventory/catalog routes are not blocked by this gate.

This is a fail-closed compatibility boundary, not full telemetry tenant isolation: Prometheus, Loki, and Tempo queries still need workspace-aware labels/tenant routing, and telemetry ingestion still needs authenticated workspace ownership. Do not enable tenant-facing telemetry until those paths are partitioned and cross-workspace tests cover them.


### Workspace-scoped telemetry query contract

The workspace API exposes separate metrics, logs, and traces query routes under `/api/v1/workspaces/{workspaceID}/telemetry/*`. Every request verifies the caller's workspace membership before contacting a backend. Prometheus and Loki selectors and Tempo TraceQL include the workspace UUID as `orbit_workspace_id` / `resource.orbit.workspace.id`; there is deliberately no fallback to unscoped queries. Telemetry producers must attach the canonical workspace resource attribute, and ingestion must authenticate/authorize the producer's workspace assignment. Until that ingestion trust boundary is implemented and tested, these query routes return only data that carries the workspace label and must not be treated as proof that the ingestion path is tenant-safe.


## 12. Workload identity administration and runtime ingestion (M06)

The authenticated OTLP runtime listeners are wired in `server/cmd/orbit-api`: the human API verifier and the dedicated strict workload-token verifier are separate, and HTTP/gRPC ingestion listeners start only when workload OIDC is explicitly configured. The gateways require the shared PostgreSQL rate-limit store, use bounded request/concurrency limits, and forward only to the private Collector endpoints. The listeners default to loopback. PR #56 adds an optional Caddy TLS edge-ingress Compose overlay; a live deployment still requires real DNS/ACME, host firewall and trusted-proxy verification, production secret delivery, and deployment-specific private-network policy as described in ADR 0002.

PR #34 is merged. It adds administrator-only workload identity creation, listing, and status changes, plus workspace owner/administrator grant listing and per-signal grant/revoke endpoints. Repository mutations write workload audit events transactionally with the identity/grant changes. The routes are API-only; no public management UI is enabled.

The PostgreSQL-backed integration suite verifies valid grants, cross-workspace denial, and revoked-identity denial for metrics, logs, and traces over both OTLP/HTTP and OTLP/gRPC. PR #36 is merged and CI-verified; its additional integration layer forges the reserved `orbit.workspace.id` attribute for each signal and verifies both gateways reject it before forwarding to the Collector. PR #38 is also merged with passing CI and proves that a forged workspace attribute on a later resource rejects the entire mixed-resource export for all three signals over both transports. PR #39 is merged and CI-passing with per-workspace query matcher checks for Prometheus, Loki, and Tempo. PR #40 is merged and CI-passing with route-level proof that non-members cannot trigger any telemetry backend request for metrics, logs, and traces.

PR #42 is merged and CI-passing with oversized-request rejection for every signal over both transports. PR #43 is merged and CI-passing with shared rate-limit enforcement across gateway instances and HTTP/gRPC transports. PR #46 is merged and CI-passing with concurrent shared-rate-limit coverage across two replicas.

Do not enable production tenant ingestion until end-to-end backend/query isolation, ingress TLS/network policy, broader load and revocation-bound tests, and deployment runbooks are verified.


## M06 handoff update — PR #48

PR #48 is merged into `main` (squash merge commit `3c5619d380b498a50660415b68175f8266c48a71`). CI run `41352185701` completed successfully: Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test all passed. The PostgreSQL-backed integration test uses four independent gateway/authorizer instances (two HTTP, two gRPC) and verifies that signal-grant revocation and workload-identity revocation are observed by other replicas for metrics, logs, and traces; denied requests do not reach the Collector forwarders.

The test establishes read-after-commit revocation for requests authorized after the revocation transaction commits. It does not establish cancellation of requests that completed authorization before that commit. In-flight revocation ordering/cancellation remains an explicit open design and test item. Production ingestion remains disabled until TLS/network policy, in-flight semantics, broader load testing, real-provider PKCE, and operational runbooks are complete.


## M06 handoff update — PR #50

PR #50 is merged into `main` (squash merge commit `45680118f44c3cd35f64214072085e6ca1a43de6`). CI run `41352877449` passed all three jobs: Go API tests/build, web typecheck/production build, and local telemetry pipeline end-to-end smoke test.

The PostgreSQL-backed integration test uses blocking forwarders to establish the in-flight revocation contract for both HTTP and gRPC. Authorization is linearized when the shared repository confirms the grant. If an export has already passed authorization and entered the Collector forwarder, it may complete after a later identity-revocation commit. Requests whose authorization begins after that commit are denied and never forwarded. This is deliberately not a promise to cancel or roll back telemetry already admitted to the Collector.

Next M06 work: broader load/capacity and failure-mode testing; production ingress TLS and network policy; real-provider browser PKCE rollout; dependency/secrets and retention/cardinality hardening; backup/restore and operational runbooks. Production tenant ingestion remains disabled until these gates are complete.


## M06 handoff update — PR #51 gateway capacity

PR #51 is merged into `main` (squash merge commit `1ff6a8baf80e50910f4c0b41ebaec73b4fdd26ba`). CI run `41353306270` passed all three jobs: Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

The PostgreSQL-backed HTTP and gRPC integration tests configure one admission slot and block the first export in the Collector forwarder. A second request is rejected (HTTP 503 / gRPC ResourceExhausted) and is not forwarded; after releasing the first request, it completes successfully. This verifies bounded admission under a deterministic saturation condition, not the complete production load/failure envelope.

Next M06 work should cover burst/load behavior and database/Collector outages, request timeouts, and backpressure, then production ingress TLS/network controls, real-provider PKCE, retention/cardinality, dependency/secrets review, backup/restore, and runbooks. Production tenant ingestion remains disabled until all production gates are verified.


## M06 handoff update — PR #53 gRPC failure behavior

PR #53 is merged into `main` (squash merge commit `13cd7b4927b2259b10b5dd94824a70ee11541069`). CI run `41353731713` passed all three jobs: Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

The gRPC gateway now explicitly verifies fail-closed behavior when the shared rate-limit store is unavailable, maps Collector failures to `Unavailable`, and maps forward deadlines to `DeadlineExceeded`. These tests prove the protocol contract and non-forwarding on the rate-limit-store failure; they do not substitute for broader production outage/load tests.

PR #54 is merged with passing CI; HTTP Collector forward timeouts now return 504 Gateway Timeout, matching the explicit gRPC deadline contract. Continue to hold production ingestion disabled until CI-verified failure/load coverage and the remaining TLS/network, provider, data-retention, backup/restore, and runbook gates are complete.


## M06 handoff update — PR #56 TLS edge-ingress scaffold

PR #56 is merged into `main` (squash merge commit `45997fd2ec508e260a51a027208152c9b155ac0f`). CI run `41354509326` passed all three jobs. The new optional production Compose overlay adds Caddy TLS termination for distinct OTLP/HTTP and OTLP/gRPC domains. Automated validation confirms that only Caddy publishes edge ports 80/443, API OTLP listeners and raw Collector receivers are not host-published, and Caddy is not attached to the application default or telemetry-internal network.

This is a topology/configuration scaffold, not a live production deployment. Real DNS and ACME issuance, firewall rules, trusted-proxy behavior, production secret delivery, internal-hop mTLS assessment, and cloud/Kubernetes network controls remain deployment-specific gates. Continue to keep tenant ingestion disabled until these and the remaining load/recovery, provider PKCE, retention, backup/restore, and runbook gates are verified.


## M06 handoff update — PR #58 dependency recovery

PR #58 is merged into `main` (squash merge commit `fd7c7d23d952bdb6f08250c2dcd699c2816620d3`). CI run `41355041031` passed Go API tests/build, web typecheck/production build, and the local telemetry pipeline smoke test.

New HTTP and gRPC recovery tests inject transient failures into the workload-grant repository, shared rate-limit store, and Collector forwarder. They verify fail-closed behavior for database errors, no Collector forwarding while authorization or shared rate limiting is unavailable, and successful processing after dependencies recover without replacing the gateway instance.

The tests use deterministic in-process stubs; they do not prove process restart, network partition, or sustained-load recovery behavior. Next work should expand burst/concurrency/backpressure coverage and add real PostgreSQL/Collector fault-injection in an isolated integration environment. Production tenant ingestion remains disabled.


## M06 handoff update — PR #60 default burst capacity

PR #60 is merged into `main` (squash merge commit `cb85c0afea6647ecc59b9fe59e499ce8383fc526`). CI run `41355534531` passed all three jobs. Tests saturate the default 64 in-flight export slots for HTTP and gRPC, verify over-capacity requests are rejected before Collector forwarding (HTTP 503 / gRPC ResourceExhausted), and verify admitted exports complete after the slots are released.

This is deterministic concurrency-boundary coverage rather than a throughput/load benchmark. Sustained traffic, backpressure under real load, and database/Collector process or network fault tests remain open.


## M06 handoff update — PR #62 real Collector listener restart recovery

PR #62 is merged into `main` (squash merge commit `534f6d02c04616a5d27831460b24fd87db8bfade`). CI run `41357122909` passed all three jobs: Go API race tests/build, web typecheck/production build, and the local telemetry pipeline smoke test.

The new tests exercise the production HTTP and gRPC Collector forwarders against real local TCP servers. They stop each Collector listener, verify failure through the still-running gateway, restart the listener on the same address, and verify the same HTTP handler / gRPC client recovers without recreating the gateway or forwarder.

This covers local listener restart and reconnect semantics, not Docker orchestration, packet loss/network partitions, PostgreSQL restart, or sustained-load recovery. Real PostgreSQL fault/restart testing, extended load/backpressure, production DNS/ACME and firewall validation, real-provider PKCE, retention, backup/restore, and runbooks remain open. Production tenant ingestion remains disabled.


## M06 handoff update — PR #64 PostgreSQL network recovery

PR #64 is merged into `main` (squash merge commit `6b2e3418be2e8fdff0c9634b361acaa5bb6d6da1`). CI run `41357596656` passed all three jobs. A PostgreSQL integration test routes the real pgxpool through a local TCP proxy, warms an established session, closes active tunnels and refuses new connections while offline, observes a repository Ping failure, then restores the same proxy and verifies the existing repository/pool reconnects and becomes healthy.

The PostgreSQL process remains running during the simulated network outage. This is real TCP-path disruption and pgxpool reconnect coverage, not a database process/container restart, packet-loss or cross-zone partition test, or sustained-load benchmark. Continue with DB process lifecycle tests where the deployment environment permits, long-duration load/backpressure, and remaining production go-live gates. Tenant ingestion remains disabled.


## M05 implementation update — PR #66 incident lifecycle backend

PR #66 is merged into `main` (squash merge commit `7a57dd4ef8752e8e18d6dec5f0e75112637167dd`). CI run `41360316146` passed Go race tests/build and vet, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

The first M05 slice provides workspace-scoped incident creation, status transitions, reopening, and immutable timeline reads. Migration 009 adds `incident_timeline_events`, a composite foreign key to the owning incident/workspace, and database triggers against UPDATE/DELETE/TRUNCATE. PostgreSQL mutation methods lock and verify the actor's workspace membership, check service/incident scope, and write incident data, timeline event, and `workspace_audit_events` in the same transaction. Only owner/administrator/operator roles may mutate; workspace members may read the timeline.

HTTP routes:
- `POST /api/v1/workspaces/{workspaceID}/incidents`
- `PATCH /api/v1/workspaces/{workspaceID}/incidents/{incidentID}/status`
- `GET /api/v1/workspaces/{workspaceID}/incidents/{incidentID}/timeline`

The status graph deliberately prevents arbitrary jumps and distinguishes reopening a resolved incident as `incident.reopened`. The OpenAPI 3.1 document now contains these route contracts and schemas.

Scope boundary: this does not yet deliver the dashboard UI for incident operations, alert rules/notification adapters, deployment annotations, SLO/error-budget workflows, or a complete audit trail for all operator actions. Treat M05 as in progress, not complete.

## M06 load-testing handoff

The repository now has an in-process OTLP gateway baseline for both HTTP and gRPC: sustained concurrency regression tests plus handler-path throughput/allocation benchmarks. See [M06 load-testing guide](m06-load-testing.md). The sustained tests use 16 workers × 32 metrics exports for each transport.

Important boundary: the test verifier, workspace-grant repository, shared rate-limit store, and Collector forwarder are local stubs. This validates request-path behavior and bounded concurrency, but not PostgreSQL/Collector capacity, multi-process performance, production latency, TLS, or backpressure under real dependency load. A target-environment load report remains a production gate.


## M05 implementation update — PR #69 workspace alert rule definitions

PR #69 is merged into `main` (squash merge commit `64c4c4f1d36fd0c17ae444ac5736fadad0a32655`). CI run `41361579435` passed Go race tests/build and vet, web typecheck/production build, and the telemetry end-to-end smoke test.

The backend now stores and manages alert-rule definitions per workspace. Migration 010 creates `alert_rules` with a case-insensitive workspace/name uniqueness index and bounded constraints. The authenticated API supports:
- `GET /api/v1/workspaces/{workspaceID}/alert-rules`
- `POST /api/v1/workspaces/{workspaceID}/alert-rules`
- `PUT /api/v1/workspaces/{workspaceID}/alert-rules/{ruleID}`
- `PATCH /api/v1/workspaces/{workspaceID}/alert-rules/{ruleID}/enabled`
- `DELETE /api/v1/workspaces/{workspaceID}/alert-rules/{ruleID}`

Owner, administrator, and operator roles can mutate definitions; any workspace member can list them. Each create/update/enable/disable/delete operation writes a workspace audit event in the same PostgreSQL transaction. Rule configuration is strictly allow-listed: metric `request_rate` only, operators `gt/gte/lt/lte`, thresholds in a finite positive bounded range, windows of 60/300/900 seconds, pending duration of 0–3600 seconds, and known severities. No arbitrary query expression or webhook destination is accepted.

Important boundary: definitions are persisted configuration only. There is not yet a periodic evaluator, alert-instance lifecycle/deduplication, multi-replica scheduler coordination, or notification delivery/retry adapter. The current metric is the request counter emitted by the instrumented demo workload; do not advertise error-rate or latency alerting until those metrics exist and are sourced safely.


## M05 implementation update — PR #71 incident operations dashboard

The dashboard now has a dedicated workspace-scoped Incident Operations view. It loads the signed-in user's workspaces and membership, then retrieves services and incidents only through the workspace-scoped API. Operators (owner, administrator, or operator roles) can create an incident and request only backend-approved status transitions; resolved incidents can be reopened into investigating. The detail view reads the immutable incident timeline. Every mutation is performed through the existing transactional API, which writes the timeline and workspace audit event atomically.

The page's API connection flow now uses workspace-scoped inventory routes rather than the legacy unscoped global service/incident reads. Demo mode remains explicitly read-only. The public sign-in controls remain hidden until real OIDC provider settings are configured; the dashboard operations require a valid browser session.

Validation for PR #71: pending CI at handoff.
