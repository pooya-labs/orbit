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
2. **Alert rules and notification adapters:** store bounded workspace rules, evaluate them in a worker, deduplicate firing alerts, persist delivery outbox state and append-only attempt history, and send through pluggable adapters. The first webhook adapter uses exact hostname allowlists and deployment-managed secret references; raw endpoint URLs and tokens never enter the API or database. Additional adapters, acknowledgement/silence/cooldown, and latency/error-rate policies remain follow-up work. Secrets belong in environment/secret management, never database plaintext or source control.
3. **Deployment annotations and release comparison:** record service, environment, version, commit SHA, deployment time, actor, and optional change reference; correlate annotations with telemetry without inventing causal attribution.
4. **SLOs and error budgets:** define an SLI from an explicit numerator/denominator and valid-event policy, target, and rolling period. Calculate remaining budget and burn rate using documented math and test boundary cases.
5. **Audit trail:** append-only records for actor, role, action, entity, request ID, timestamp, and a redacted before/after representation. Audit writes must be transactional with the state change where possible.

M05 acceptance requires API and database tests, lifecycle-transition tests, worker idempotency tests, and operator-visible history. A UI-only mock does not complete an operations workflow.

## 7. M06 security gate and production hardening

The detailed identity design is recorded in [ADR 0001 — OIDC access-token validation and API authorization](decisions/0001-oidc-access-token-authentication.md), and the machine-ingestion trust boundary is recorded in [ADR 0002 — authenticated workspace OTLP ingestion](decisions/0002-authenticated-workspace-otlp-ingestion.md). Both distinguish human and workload access tokens and require server-side authorization.

The following implementation is merged and CI-verified:

- Human OIDC access-token validation through issuer discovery/JWKS, RS256/ES256 allowlisting, viewer/operator/administrator route policy, fail-closed staging/production startup, request/token bounds, and shared PostgreSQL-backed API rate limits (`977ce49`, `db45843`).
- Provider-neutral browser Authorization Code + PKCE with state/nonce and memory-only access/refresh tokens (PR #12); no real public-site issuer/client is configured.
- Workspace membership, tenant-scoped reads, member/role enforcement, workload identity administration, per-signal workspace grants, and transactional audit records for M05 mutations.
- Authenticated OTLP/HTTP and OTLP/gRPC gateways, canonical workspace attribution, producer-attribute rejection, mixed-resource batch rejection, bounded admission, shared rate limits, cross-replica revocation checks, and fail-closed dependency behavior.
- PR #84 proves the local write-to-query boundary for metrics/logs/traces with real PostgreSQL, Collector and Prometheus/Loki/Tempo backends; workspace A sees its data and workspace B does not.
- PostgreSQL logical backup/restore CI drill (PR #83), seven-day local telemetry retention/cardinality controls, and the repository threat model plus automated dependency/secret scans (PR #85).

Remaining before production tenant ingestion:

- Real OIDC provider registration, browser callback/CORS/audience verification, workload-token issuance/rotation, and provider token revocation/global logout.
- Real DNS/ACME, TLS termination, host firewall/trusted-proxy policy, cloud/Kubernetes network policy, and an explicit internal-hop mTLS decision.
- Target-environment multi-replica load, query latency, backpressure, outage/recovery and revocation-latency measurements with real PostgreSQL and Collector dependencies.
- Managed-backend per-workspace ingestion/cardinality/retention/cost budgets.
- Encrypted off-host database backups, approved retention/PITR/WAL policy, backup alerting, a rehearsed managed restore, and measured RPO/RTO.
- Secret/key rotation, container-image scanning, action SHA pinning, remediation ownership, deployment monitoring, and operator incident runbooks.

Local development may omit OIDC only when `ORBIT_ENVIRONMENT=development` is explicitly set. Staging/production startup must fail closed without valid human and workload authentication settings. Health/readiness endpoints should reveal status, not credentials or sensitive topology.

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

Scope after PR #72: the definitions are evaluated by a scheduled worker for the single allow-listed `request_rate` metric, using the workspace-attributed `orbit_demo_requests_total` counter. PR #74 adds the first outbound webhook delivery adapter and persistent retry history. Do not advertise error-rate or latency alerting until those metrics exist and are sourced safely.


## M05 implementation update — PR #71 incident operations dashboard

The dashboard now has a dedicated workspace-scoped Incident Operations view. It loads the signed-in user's workspaces and membership, then retrieves services and incidents only through the workspace-scoped API. Operators (owner, administrator, or operator roles) can create an incident and request only backend-approved status transitions; resolved incidents can be reopened into investigating. The detail view reads the immutable incident timeline. Every mutation is performed through the existing transactional API, which writes the timeline and workspace audit event atomically.

The page's API connection flow now uses workspace-scoped inventory routes rather than the legacy unscoped global service/incident reads. Demo mode remains explicitly read-only. The public sign-in controls remain hidden until real OIDC provider settings are configured; the dashboard operations require a valid browser session.

Validation for PR #71: CI run `41364624165` passed Go race tests, vet/build, web typecheck/production build, and the complete telemetry pipeline smoke test. PR #71 is merged in `main` as `e5bdc71eaee9edceca0c50f7c7ad2b23df8af7bd`.


## M05 implementation update — PR #72 alert evaluator and durable instances

PR #72 adds a bounded background evaluator for the existing workspace alert-rule model. CI run `41366328220` passed the Go race tests, vet/build, web typecheck/build, and complete OTLP telemetry smoke test.

Migration 011 adds `alert_instances` and append-only `alert_instance_events`. Each active rule/service pair can have only one pending or firing instance. Instances snapshot the rule name, service, metric, operator, threshold, evaluation window, pending duration and severity so history remains readable after rule edits or deletion. State changes and their event rows are written transactionally.

The evaluator:
- Runs in the API process on a 30-second cadence when PostgreSQL alert storage and the Prometheus endpoint are configured.
- Uses a PostgreSQL session advisory lock so only one API replica evaluates a tick at a time, plus a transaction-scoped rule/service lock for idempotent state transitions.
- Evaluates only the fixed request-rate query over `orbit_demo_requests_total`, filtered by `orbit_workspace_id` and grouped by `service_name`. It does not accept arbitrary PromQL.
- Enforces a 1,000 enabled-rule cap per tick and fails the tick on overflow instead of silently skipping rules.
- Supports `pending`, `firing`, `resolved`, and `cancelled`. Rule update, disable, and delete close active instances and append a lifecycle event in the same transaction.
- Resets a pending timer after a 60-second observation gap. Missing telemetry never counts as recovery; a firing alert stays firing until a later valid sample is below the threshold or the rule is changed.

The workspace read API exposes `GET /api/v1/workspaces/{workspaceID}/alerts` with status and bounded-limit filters. The event history is append-only. `fired_at` and `resolved_at` remain explicit nullable JSON fields.

PR #74 delivers the initial outbound webhook adapter, transactional delivery outbox, per-delivery idempotency, bounded retry worker, and immutable attempt history. Still out of scope: additional email/chat adapters, delivery acknowledgements/silences, SLO/error-budget calculations, and alert policies for latency or error rate. The only active metric is the instrumented demo request counter; production tenant ingestion remains disabled pending M06 environment gates.


## M05 implementation update — webhook notification delivery (PR #74)

The notification delivery slice adds migration 012 with workspace-owned webhook channels, a transactional delivery outbox, and immutable attempt history. Alert `firing` and `resolved` events enqueue one delivery per enabled channel in the same PostgreSQL transaction as the event.

The API exposes workspace-scoped channel management and read-only delivery/attempt history. Only owner, administrator, and operator roles may create or enable/disable channels; any workspace member can read history. The API stores only a `secret_ref`; the endpoint URL and credential are injected through deployment environment/secret management and are never accepted from or returned to the browser.

The worker uses `FOR UPDATE SKIP LOCKED` claims and lease recovery for multi-replica execution, a bounded batch, an at-least-once delivery model with a stable per-delivery idempotency key, exponential backoff, and a maximum of eight attempts. Attempt history is append-only. The initial adapter is webhook only; more adapters and acknowledgement/silence policies remain future work.


## M05 implementation update — deployment annotations (PR #77)

Migration 013 and the deployment annotation API add immutable, workspace-scoped release markers containing version, commit SHA, optional change reference/release notes, deployment time, derived service name/environment, and creation time. Owner/administrator/operator roles can create annotations; workspace members can list and read. The annotation row and `deployment.annotation_created` workspace audit event are transactional. PostgreSQL rejects UPDATE, DELETE, and TRUNCATE. Comparison is restricted to two different annotations of the same service and environment in chronological order and reports version/commit changes and elapsed time only. It intentionally makes no causal claim about telemetry.


## M05 implementation update — SLO definitions and error budgets (PR #78)

Migration 014 adds workspace/service-scoped SLO definitions. The API supports create, list, update, enable/disable, delete, and live budget calculation. Definition changes and workspace audit events are transactional. Target is a fraction from 0.90 to 0.9999; allowed rolling windows are 1/3/7 days to match Prometheus retention.

The only SLI is the instrumented HTTP request-success ratio: 2xx/3xx responses are good and other instrumented responses are bad. Query selectors are fixed by the server and include canonical workspace ID, service name, normalized route, and outcome. An empty result vector is `insufficient_data`, never an assumed 100% availability. The local Compose telemetry demo sends unscoped metrics directly to the Collector, so it is excluded from workspace SLO calculations unless an instrumented workload sends through authenticated workspace ingestion. Current SLO evaluation is on-demand; there is no scheduled budget snapshot or alerting/burn-rate notification yet.


## M05 handoff update — workspace audit log UI

PR #79 builds on the membership-protected `GET /api/v1/workspaces/{workspaceID}/audit-events` endpoint already present on `main` and PostgreSQL append-only event store. The audit UI slice adds a typed client method, an authenticated workspace page, category filters, request-ID visibility, expandable bounded event details, 30-second refresh, English/Persian copy, and dark/light responsive styling. It covers the existing audit event families without displaying raw actor issuer/subject identifiers or webhook-secret fields.

PR #82 completes the actor-context follow-through: migration 015 creates a random, stable actor reference scoped to each workspace and snapshots the membership role when an audit event is inserted. Historical events receive a stable actor reference but retain `unknown` role because past membership state cannot be safely reconstructed. The API serializes only the pseudonymous reference and captured role; raw issuer/subject remain server-side. Actor mappings and audit events both reject UPDATE, DELETE, and TRUNCATE. The Audit Log now displays the actor reference and role in English and Persian.


## M05 audit actor context — PR #82

Migration 015 adds immutable workspace-local actor-reference mappings and `actor_role` snapshots to workspace audit events. The insert trigger resolves an existing random actor reference and captures the actor's current workspace membership role. Legacy events are backfilled with stable pseudonymous references while their role stays `unknown`; historical membership roles are not inferred. The API response includes only `actor_ref` and `actor_role`, never raw OIDC issuer/subject. The actor mapping and audit event tables reject UPDATE, DELETE, and TRUNCATE. The PR #79 Audit Log page now displays the pseudonymous actor reference and captured role, while keeping the workspace category filters and sensitive-detail redaction.


## M06 handoff update — telemetry retention and cardinality guardrails

The local Compose telemetry baseline now has a documented, CI-checked seven-day retention window across Prometheus metrics, Loki logs, and Tempo traces. Prometheus Collector scrapes are bounded by sample count, label count/name/value size, and scrape body size. Loki has explicit ingestion, stream, label and log-line limits. Tempo has hourly block-retention scheduling, bounded trace blocks, active-trace count, and attribute-byte limits.

Do not rebuild the Collector memory/batch controls: the existing Collector already uses a 256 MiB memory limit, 64 MiB spike limit, and five-second batch timeout. Do not rebuild API rate-limit expiry either: `api_rate_limit_windows` rows expire after two minutes and are pruned by the existing request-driven cleanup path, with an indexed `expires_at` column.

These are single-tenant local Compose budgets, not production per-workspace quotas. Loki has `auth_enabled: false` and Tempo uses the local single-tenant backend in this profile. The new checks prove config intent and the CI service startup parses the pinned backend YAML; they do not prove capacity or managed-service behavior.

Durable workspace audit events, incident timelines, alert lifecycle events, notification attempts and deployment annotations are intentionally not TTL-pruned because they are immutable operator history. A policy for export/archive, compliance, and eventual deletion remains open before any such data can be purged. Production tenant ingestion remains disabled pending the separate provider, networking, managed-retention, backup/restore, load, and runbook gates.


## M06 handoff update — PostgreSQL backup and restore drill

The repository now contains `scripts/postgres_backup_restore_smoke.sh` and `docs/postgres-backup-restore.md`. The CI drill uses a PostgreSQL 17 custom-format `pg_dump`, restores into a separate `template0` database with `pg_restore --exit-on-error`, compares the full `schema_migrations` ledger, and verifies a workspace owner, audit event, actor reference/role, and append-only triggers survived. It leaves the source database intact except for its explicit test fixture and removes only the temporary restored database.

This is a repeatable logical backup/restore regression test, not a production backup service. Encrypted off-host storage, schedule/retention, PITR/WAL policy, backup monitoring, managed-database restore rehearsal, and measured RPO/RTO remain deployment gates. Durable API history is still not TTL-pruned.


## M06 handoff update — workspace telemetry write-to-query isolation (PR #84)

CI run `41383068747` passed the new `Verify authenticated workspace telemetry write-to-query isolation` step. The test uses real PostgreSQL workspace identities/grants, the authenticated OTLP/HTTP gateway, the private Collector, Prometheus, Loki, Tempo, and the actual workspace-scoped ORBIT query handlers. A workload granted only to workspace A exports metrics, logs, and traces; A-scoped queries return the data, while a member-scoped query for workspace B returns no matching data. Direct writes to B are denied and a producer-supplied `orbit.workspace.id` for B is rejected before forwarding.

The test uses a deterministic workload-token verifier fixture; JWT signature/claim validation remains covered by the separate OIDC verifier tests. The real backend write-to-query round trip runs over OTLP/HTTP; existing PostgreSQL-backed gateway tests cover HTTP/gRPC authorization, forged attributes, mixed-resource batches and revocation. This local Compose CI test does not prove production DNS/TLS, real-provider credentials, cloud/Kubernetes network policy, managed-service per-workspace quotas, or target-environment capacity. Production tenant ingestion remains disabled pending those deployment gates.


## M06 handoff update — security automation and immutable workflow action pins

PR #85 proposes a repository threat-model baseline, full-history Gitleaks scanning, Go vulnerability analysis, npm audit, PR dependency review, and scheduled Dependabot updates. Its pull-request CI currently requires workflow approval; do not mark the slice merged or CI-verified until the required approval is granted and the resulting checks pass.

The same PR pins every external GitHub Action to the full commit SHA resolved from its reviewed release tag, keeps a human-readable version comment beside each pin, and adds \`scripts/check_workflow_action_pins.py\` as a CI policy check. The policy also rejects unpinned Docker action images and validates local action paths. Future action updates should be made through reviewed Dependabot proposals, with the commit pin and version annotation updated together.

These controls improve source/CI hygiene; they do not replace image/base-layer scanning, human triage and remediation SLAs, SBOM/provenance, credential rotation, or target-environment security testing. PR #85 must not be reported as merged or verified until its workflows are approved and pass.
