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
- **M04 workload resource attributes are now captured directly from authenticated OTLP metrics, logs, and traces.** The gateways inject the canonical workspace after identity/grant checks and persist a bounded, reviewed low-risk allowlist with per-attribute provenance in a workspace-scoped catalog. This intentionally does not mean arbitrary or complete SDK resource data is stored; allowlist expansion remains review-gated.
- **Redis and NATS are provisioned but not yet integrated into application behavior.** Do not claim caching or event delivery until the API actually uses them and tests cover those paths.
- **M06 API and browser security foundation:** OIDC access-token verification through issuer discovery/JWKS, RS256/ES256 allowlisting, centralized viewer/operator/administrator route policy, explicit development mode, fail-closed staging/production startup, body/token caps, and PostgreSQL-backed shared per-IP/per-subject rate limits are merged and CI-verified. The provider-neutral browser Authorization Code + PKCE (S256) client is implemented in PR #12: state/nonce and ID-token signature/issuer/audience validation are enforced, API calls receive an in-memory Bearer token, and access/refresh tokens are not persisted. The browser sign-in controls stay hidden unless a real issuer and public client are configured at build time.
- **Repository security baseline is merged and CI-verified.** PR #90 added the threat model, full-history Gitleaks, govulncheck, npm audit, Dependency Review, immutable full-SHA action pins, production-image scanning, and 11 retained CycloneDX SBOM artifacts. Local telemetry retention/cardinality controls, PostgreSQL backup/restore CI, and operator runbook templates are also complete. **Production gates still open:** real human/workload OIDC provider configuration and end-to-end PKCE, token/key rotation and revocation, DNS/ACME and host/network policy, sustained target-environment load/backpressure, encrypted off-host backups/PITR and measured restore, durable-history retention/archive policy, signed provenance, and hosted API/managed persistence. Production tenant ingestion remains disabled.

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

M04's current bounded telemetry read path and authenticated workspace-scoped resource-attribute capture are implemented and CI-verified. PR #104 added direct capture from authenticated OTLP metrics, logs, and traces, canonical workspace attribution, a reviewed low-risk attribute allowlist, per-attribute provenance, and PostgreSQL/Collector/backend isolation coverage.

Remaining M04 follow-ups are deliberately narrow:
- expand the workload resource semantic-convention allowlist only after privacy, cardinality, and operational review; arbitrary producer attributes remain unsupported;
- add arbitrary metric-name discovery/selection only after a safe metrics model and cardinality budget are defined;
- improve user-facing access to individual trace/log context in a later UX iteration.

Do not widen query access to arbitrary PromQL merely to complete metric selection. See `docs/roadmap.md` and the PR #104 handoff below for the verified implementation boundary.

## 6. M05 delivery sequence

M05's defined operator-workflow scope is implemented and CI-verified; see `docs/roadmap.md` for the current completion record. The sequence below is retained as design history, not an active implementation backlog. State-changing operations must remain workspace-scoped and protected by the existing authentication and role checks.

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
- PostgreSQL logical backup/restore CI drill (PR #83) and seven-day local telemetry retention/cardinality controls are merged and verified.
- The repository security baseline is merged in PR #90 (`4a4c464`). Its final head passed all 13 Security jobs, including Dependency Review, Gitleaks, govulncheck, npm audit, action-pin enforcement, production image scans, and the complete 11-artifact CycloneDX SBOM upload (run `41406376150`).

Remaining before production tenant ingestion:

- Register and end-to-end verify real human and workload OIDC providers, browser callback/CORS/API audience, workload-token issuance/rotation, and provider token revocation/global logout.
- Verify real DNS/ACME, TLS termination, host firewall/trusted-proxy policy, cloud/Kubernetes network policy, and the internal-hop mTLS decision.
- Record target-environment multi-replica ingestion/query load, latency percentiles, backpressure, outage/recovery, and revocation-latency evidence using real PostgreSQL and Collector dependencies.
- Define managed-backend per-workspace ingestion/cardinality/retention/cost budgets and an approved durable-history retention/archive policy.
- Configure encrypted off-host backups, retention/PITR/WAL policy, backup alerting, and a rehearsed managed restore with measured RPO/RTO.
- Complete signed build/release provenance, remediation ownership/SLAs, deployment monitoring, execution of the operator runbooks, and hosted API/managed persistence.

The repository-level dependency, secret-scanning, action-pin, image-scan, SBOM, local retention, and backup/restore CI gates are already complete; do not repeat them as implementation tasks. Production tenant ingestion remains disabled until the environment-specific gates above are evidenced.

Local development may omit OIDC only when `ORBIT_ENVIRONMENT=development` is explicitly set. Staging/production startup must fail closed without valid human and workload authentication settings. Health/readiness endpoints should reveal status, not credentials or sensitive topology.

## 8. Related portfolio context

The GitHub account also contains separate portfolio projects (Pooya.OS, AuraPulse, NexusCanvas, and LuminaGlass). ORBIT is its own repository and backend architecture, not a shared monorepo. Reuse visual/design conventions where appropriate, but do not couple their runtime state, APIs, or deployment pipelines without an explicit product decision.

## 9. M06 security handoff — current status

PR #90 is merged into `main` as `4a4c46445d7f6b0129cba647d1bd71c49bbd0d45`. The final PR head passed CI run `41406376148`, all 13 Security jobs in run `41406376150`, and the Pages-source check in run `41406376156`. All 11 planned CycloneDX SBOM artifacts were uploaded with 30-day retention. Repository-level Gitleaks, Dependency Review, govulncheck, npm audit, immutable action-pin enforcement, and production image scanning are complete.

The provider-neutral browser Authorization Code + PKCE client remains implemented but is not connected to a real issuer/client; public sign-in stays hidden until deployment configuration and end-to-end validation are complete. The API accepts access tokens only through `Authorization: Bearer`; credentials are not read from URLs or persistent browser storage. Access/refresh tokens remain in JavaScript memory, while the short-lived PKCE transaction uses `sessionStorage`.

The remaining M06 work is deployment/policy-specific: real human/workload OIDC and key/token lifecycle, DNS/ACME/firewall/trusted-proxy/network policy, sustained target-environment load and backpressure, encrypted off-host backup/PITR and measured restore, managed retention/quotas, durable-history archive policy, signed provenance, hosted API/managed persistence, and execution of operational runbooks. Production tenant ingestion remains disabled until those gates are evidenced.

## 10. Continuity rule

When resuming work, start with:
1. `docs/roadmap.md` for milestone status;
2. this file for project history and boundaries;
3. `docs/architecture.md` for current system shape;
4. the latest GitHub Actions run and open PRs for verified code state.

Then continue on a feature branch, update tests and docs with implementation, and keep the public Pages deployment unchanged until the relevant PR is merged and deployment CI succeeds.


## 11. Workspace and audit foundation (M06)

> Historical implementation snapshot. For current milestone status and remaining gates, use `docs/roadmap.md` and sections 7 and 9 above; the incremental notes below record how the workspace boundary was built and are not a live checklist.

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

> Historical implementation snapshot. The current ingestion and production-readiness status is maintained in `docs/roadmap.md` and sections 7 and 9 above.

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

The PostgreSQL process remains running during this simulated TCP-path outage. PR #111 now separately covers an actual Compose PostgreSQL container/process stop-start and confirms the original pgxpool reconnects; that clean stop/start test does not prove crash/power-loss recovery, HA failover, disk/PITR recovery, packet-loss or cross-zone partition handling, or production RPO/RTO. Target-environment load/backpressure and remaining production go-live gates remain open. Tenant ingestion remains disabled.


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


## M01 deployment handoff — GitHub Pages source verification

The public ORBIT dashboard is a Next.js static export published from the `out/` artifact by `.github/workflows/deploy.yml`. GitHub Pages must use the Actions publishing source (`build_type=workflow`), not legacy branch/Jekyll publishing, which can race or overwrite the artifact. The repository setting was read successfully by the dedicated verification workflow on 2026-11-02 and returned `workflow`.

The refreshed PR #11 implementation is rebuilt on current `main` as PR #88. It checks the Pages source both on relevant pull requests and immediately before every deployment, failing closed for `legacy`, unknown, or inaccessible settings. The one-time setting and recovery steps are in `docs/deployment/github-pages.md`. The workflow checks but does not mutate repository administration settings.


## M06 handoff update — deployment and operations runbook

PR #86 was merged into `main` as `a412864c8639ed6530bdbe07569e878ee33a9e6a`. It adds `docs/operations-runbook.md`, covering production preflight, safe deployment, post-deployment verification, routine operations, incident response, rollback boundaries, backup/restore coordination, and evidence capture. Its full API, PostgreSQL restore, web, and telemetry CI passed before merge.

The runbook is an operator procedure baseline, not production-readiness evidence. The environment-specific gates remain open: real human/workload OIDC provider configuration, DNS/ACME and firewall/network policy, managed per-workspace quotas, encrypted off-host backups/PITR and measured RPO/RTO, sustained target-environment load/backpressure testing, and hosted API/managed persistence. Production tenant ingestion remains disabled until those gates are evidenced. The README documentation index links the runbook.


## M06 security automation handoff — PR #90

The security work has been rebuilt on the current `main` in PR #90 to avoid the stale base of the superseded PR #85. It adds the threat-model baseline, full-history Gitleaks, Go `govulncheck`, npm audit, scheduled Dependabot, Node 22/`npm ci`, refreshed Go modules, and full-SHA pins for external GitHub Actions enforced by `scripts/check_workflow_action_pins.py`. It also adds a pinned Trivy v0.76.1 scan of the built production API image; the scanner reported zero HIGH/CRITICAL Debian 12.15 OS vulnerabilities and zero HIGH/CRITICAL findings in the Go API binary.

Latest scan evidence on PR #90: Gitleaks passed; `govulncheck` found zero vulnerabilities affecting code paths (two vulnerabilities were reported in required modules that the application does not appear to call); npm audit reported zero vulnerabilities; the workflow action pin policy passed; Trivy v0.76.1 reported zero HIGH/CRITICAL findings for both Debian 12.15 OS packages and the Go API binary.

**External blocker:** GitHub Dependency Review reports that the repository Dependency Graph is not enabled. The repository owner must enable it under Settings → Code security and analysis → Dependency graph, then rerun the Security workflow. Do not mark PR #90's full security gate complete or merge it as fully security-verified while Dependency Review is blocked. This is a repository administration setting and must be enabled by an administrator in GitHub Settings.


### PR #90 security validation update — 2026-11-02

On head `0e06b6ad77ed53610a085bc59cc84dc7e33dc177` (2026-11-02):
- Full-history Gitleaks: passed.
- Workflow action pin policy: passed.
- Pages publishing-source check: passed (`build_type=workflow`).
- Production API image built successfully; Trivy v0.76.1 reported zero vulnerabilities for Debian 12.15 OS packages and the Go API binary at HIGH/CRITICAL severities.
- Web typecheck and production build: passed.
- Go API race tests/vet/build, web typecheck/production build, full telemetry E2E, Prometheus scrape, and authenticated Workspace A/B write-to-query isolation: passed in CI run `41397730088`.
- Security run `41397730087`: Gitleaks, API image scan, `govulncheck`, and npm audit passed. The Security workflow is marked failed only because Dependency Review cannot run while the repository Dependency Graph setting is disabled.
- Pages source verification passed on the same head (`build_type=workflow`).

## M06 security PR #90 — extended image hardening update

The security branch now extends Trivy coverage beyond the ORBIT API to production-relevant Compose and ingress images. The PostgreSQL image keeps the official PostgreSQL entrypoint but rebuilds the pinned upstream `gosu` v1.20 source with Go 1.27. The Python smoke helper was replaced by a static Go binary that uses only the standard library, so the test image no longer carries an unrelated Python runtime and package set. Tempo is rebuilt from the pinned v3.1.1 source with `github.com/go-jose/go-jose/v4` upgraded to v4.1.7. The upstream all-components Collector image is replaced with a custom OCB distribution containing only ORBIT's configured OTLP receiver, memory limiter, batch processor, Prometheus/OTLP/OTLP-HTTP/debug exporters, and health-check extension. Caddy is also rebuilt from pinned v2.11.7 source with current Go and module dependencies; its scan must still be checked for legacy modules that lack a patched upstream release.

Evidence on the pre-Caddy-hardening scan run: the ORBIT API image passed; the rebuilt PostgreSQL image passed; the minimal ORBIT Collector built and passed Trivy; Gitleaks passed; Redis, NATS, Prometheus, Loki, and the Go/Alpine base images passed. The smoke-image CI initially failed because its Dockerfile omitted the new `go.mod`; that copy instruction has been corrected. The latest head's CI/security workflows are queued or running and must be checked again before merging. Dependency Review remains blocked until the repository Dependency Graph is enabled in GitHub Settings. Production tenant ingestion remains disabled.



## M06 PR #90 — extended image scans and SBOM handoff (2026-11-02)

The current PR #90 head at the time of this handoff is `5c253a2bfe8607d8574a27cbf496a1d2962c88b7`. All changes remain on `feat/m06-security-hardening-current`; they have not been merged into `main`.

New repository-controlled work:
- The CI Caddyfile syntax check now uses Caddy `2.11.7`, matching the release used by the pinned-source custom ingress build.
- The Security workflow builds and scans custom Tempo, minimal ORBIT Collector, and Caddy images. On the pre-SBOM head `f3a2a01ba16bfdf2bc2ff306e2f5ead33bfcddc7`, the custom Caddy and Collector jobs passed Trivy with the configured HIGH/CRITICAL fail threshold. Redis, NATS, Prometheus, Loki, Go 1.27 build bases, and Alpine runtime-base image jobs also passed.
- The Security workflow now emits CycloneDX SBOMs for 14 image references: API, PostgreSQL, telemetry smoke runner, telemetry demo, Redis, NATS, Prometheus, Loki, Go/Alpine build/runtime bases, Tempo, ORBIT Collector, and Caddy. The files are uploaded as 11 named workflow artifacts with 30-day retention. This implementation is not yet verified on a completed workflow run; do not describe the SBOM artifact path as passing until the current-head jobs succeed.
- The upload-artifact action is pinned to the verified full SHA for v7.0.2. The workflow action-pin policy remains enforced.

Verification snapshot:
- CI run `41401602903` on the pre-SBOM head: Go API tests, vet/build, web typecheck/build, and action-pin policy passed. The telemetry end-to-end job advanced to authenticated workspace A/B isolation; its final job conclusion was not yet available at the last inspection.
- Security run `41401602920` on the pre-SBOM head: Gitleaks and npm audit passed (npm reported zero vulnerabilities); custom Caddy and Collector build/scan jobs passed; Redis, NATS, Prometheus, Loki, Go/Alpine base scans passed. Tempo source build was still running when last inspected.
- Dependency Review failed with the explicit message that Dependency Graph is not enabled for this repository. This is an owner-side GitHub Settings action, not a code failure. Enable it at https://github.com/pooya16pg-spec/orbit/settings/security_analysis, then rerun Security.
- On current head `5c253a2bfe8607d8574a27cbf496a1d2962c88b7`, CI run `41402304517` and Security run `41402304452` were queued; the Pages publishing-source check `41402304642` passed.

Do not merge PR #90 until current-head CI, all image scans and SBOM uploads are green and Dependency Review succeeds. Signed build/release provenance, credential rotation, target-environment OIDC/network/load/recovery evidence, and production backup/retention remain separate gates. Production tenant ingestion stays disabled.


## M06 PR #90 — security remediation and owner-side gate update (2026-11-02)

The branch remains `feat/m06-security-hardening-current`, based on `main`; PR #90 is open and has not been merged. The latest implementation changes in this handoff are:
- `examples/telemetry-go/go.mod` pins `golang.org/x/net v0.60.0` and `google.golang.org/grpc v1.83.2`, addressing the two HIGH findings reported by Trivy on the earlier demo binary. The Security dependency job refreshes and verifies the demo module files and commits `go.sum` when generated. The patched image still needs a completed latest-head Trivy result.
- `deploy/observability/tempo.Dockerfile` fetches the upstream `v3.1.1` Git tag, asserts that its peeled commit is exactly `9db7df79d2af105a90d96484e6403e1fda0d011e`, and verifies the resulting binary's VCS revision.
- The upstream Tempo tag contains the fix for CVE-2026-28377: the upstream fix commit `bb8ca663db34a0980c9758b40d918fda3b4dbec3` is an ancestor of the release tag and the release changelog documents redaction of the S3 SSE-C `encryption_key`. Trivy reports the local main module with a pseudo-version ending `+dirty`; therefore PR #90 adds an OpenVEX `not_affected` statement scoped to the exact Trivy PURL and this CVE only, loaded explicitly by passing `trivy-config: trivy.yaml` to all 16 Trivy Action invocations, so the image-scan and SBOM steps use the same repository policy. This is not a blanket ignore. The latest Tempo scan must confirm the VEX statement is consumed and the scan succeeds.
- The security workflow creates CycloneDX SBOMs for 14 image references and uploads 11 named CI artifacts with 30-day retention. One older run uploaded the Redis SBOM only; the full set must be verified on the latest head.
- README, roadmap, and threat model now describe the above remediations and distinguish completed checks from pending verification.

Verified evidence:
- CI run `41401602903` completed successfully across all four jobs, including Go API race tests/vet/build, web typecheck/production build, action-pin policy, and the full local telemetry pipeline including Prometheus scrape and authenticated workspace A/B write-to-query isolation.
- Security run `41401602920`: Gitleaks passed; custom Caddy and minimal Collector build/scan jobs passed; Redis, NATS, Prometheus, Loki, Go/Alpine base scans passed. That older run exposed the Tempo metadata/CVE issue and the two Go demo module findings, which this handoff records as addressed in source but awaiting latest-head verification.
- Security run `41402304452`: Dependency Review completed successfully and reported no packages at HIGH-or-higher severity and no denied packages. This confirms the earlier Dependency Graph settings blocker is no longer preventing the action. Revalidate this check on the newest head.
- Pages-source verification passed on several reviewed heads, including `41403398126`.
- The CI, Security, and Pages checks triggered on branch head `cf74aa8bef8e4c293bcea266c52a5602923da2e9` were still running/queued when last inspected. Subsequent documentation commits retrigger checks, so always inspect the current branch head and its own workflow runs instead of reusing those older results.

Next steps after PR #90 merge:
1. Keep production tenant ingestion disabled. Repository-level CI and Security gates are complete, but this does not establish production readiness.
2. Prioritize the remaining environment-specific M06 gates: target-environment sustained ingestion/query load and backpressure; encrypted off-host backups/PITR plus a measured restore rehearsal; real DNS/ACME, host firewall, trusted-proxy and cloud/Kubernetes network policy; and real-provider browser PKCE validation.
3. Define and approve a compliant retention/archive policy for durable workspace audit, incident, alert, notification, and deployment history before implementing any pruning.
4. Keep signed build/release provenance and attestation, remediation ownership/SLAs, credential rotation, and hosted API/managed persistence deployment as distinct release/deployment work.
5. M01/M04/M02 follow-ups remain visible in `docs/roadmap.md`: manual screen-reader/contrast acceptance, reviewed expansion of the OTLP resource allowlist, Redis/NATS integration only where justified, and hosted API integration.

Verified final PR #90 evidence (all tied to the final PR head, not older runs):
- PR #90 merged by squash on 2026-11-02; merge commit on `main`: `4a4c46445d7f6b0129cba647d1bd71c49bbd0d45`.
- CI run `41406376148` completed successfully: Go race tests, `go vet`, build, PostgreSQL backup/restore round-trip, web typecheck/production build, immutable action-pin policy, and the full local telemetry pipeline. The telemetry end-to-end job passed service startup/readiness, OTLP metrics/logs/traces and API queries, Prometheus scrape, authenticated workspace A/B write-to-query isolation, and cleanup.
- Security run `41406376150` completed successfully with all 13 jobs passing: Gitleaks, govulncheck, npm audit, Dependency Review, API/PostgreSQL and telemetry smoke/demo images, patched Tempo build and VEX-aware Trivy scan, minimal Collector, Caddy, Loki, Redis, NATS, Prometheus, and Go/Alpine base images.
- All 11 planned CycloneDX SBOM artifacts were generated and uploaded with 30-day retention, including first-party images, Tempo, Collector, Caddy, Loki, Redis, NATS, Prometheus, Go API build base, and telemetry demo build/runtime bases.
- Pages source verification run `41406376156` passed.
- Tempo is built from upstream v3.1.1 pinned to commit `9db7df79d2af105a90d96484e6403e1fda0d011e`. The Dockerfile checks the exact Go build `vcs.revision` without depending on tab/space formatting. The exact-PURL OpenVEX statement is loaded explicitly by Trivy; the latest Tempo build, scan, and SBOM steps passed.
- The verified evidence above is repository/CI-level only. Production TLS, ingress/network policy, real IdP, external backups/PITR, managed quotas/retention, sustained real-environment load/recovery, and durable-history retention policy are still open. Do not enable production tenant ingestion until those gates are evidenced.

## M06 PR #90 — explicit Trivy VEX wiring (2026-11-02)

- Security workflow now passes `trivy-config: trivy.yaml` explicitly to all 16 Trivy Action invocations (both vulnerability scans and CycloneDX SBOM generation) so use of the repository-root configuration is deterministic. The root configuration points at the exact-PURL OpenVEX statement for Tempo CVE-2026-28377.
- Latest branch head at this handoff before this context-only commit: `f073221fee0541f6b0adc66ee9f26551581ff260`. The GitHub Contents API commits made after the earlier observed runs have not produced a newer PR workflow run in the connector's pull-request-only workflow-run listing. Do not assume the latest source changes have been tested. A user-authenticated push to the PR branch is needed to trigger PR synchronize CI/Security checks if the GitHub App token suppresses workflow events.
- Dependency Review is confirmed to pass on run `41402304452` (no HIGH-or-higher and no denied packages). The remaining latest-head action is to trigger and inspect current CI/Security results. Production tenant ingestion remains disabled.
### Web-based CI verification checkpoint — 2026-11-02

The final M06 security changes are maintained directly through GitHub.
Trigger fresh CI and Security validation on this branch head.

## M06 PR #90 — latest CI/Security evidence and Tempo build-check correction (2026-11-02)

- User-authenticated GitHub Web commit `1ec2193ae9296db0352e19f5ef56ceca07225bdb` triggered fresh CI run `41404735820`, Security run `41404735793`, and Pages-source check `41404735840`.
- CI `41404735820`: Go API tests/race tests, `go vet`, build, PostgreSQL backup/restore round-trip, web typecheck/production build, and immutable action-pin policy passed. The local telemetry smoke job failed while building Tempo. Its log shows that Tempo compiled and that `go version -m` reported the exact pinned `vcs.revision=9db7df79d2af105a90d96484e6403e1fda0d011e`; the Dockerfile check failed only because it searched for a literal space between `build` and `vcs.revision`, while Go emits tab-separated metadata.
- Corrected `deploy/observability/tempo.Dockerfile` in commit `e80bf366a888f8b35299846a8f2dd32ab225119e` to match the exact `vcs.revision=${TEMPO_COMMIT}` key/value independent of whitespace. The pinned tag/commit equality checks remain unchanged. The old CI/Security runs are tied to the prior head and do not validate this fix.
- Security `41404735793`: API/PostgreSQL image, Collector, Caddy, Loki, Redis, NATS, Prometheus, Go build/runtime base images, full-history Gitleaks, and Dependency Review/govulncheck/npm audit passed. Tempo image build failed at the same whitespace-sensitive assertion before Trivy scan, so its VEX-aware scan remains unverified.
- Ten SBOM artifacts were uploaded with 30-day retention on this run, including the first-party bundle and SBOMs for API build base, Collector, Caddy, Loki, Redis, NATS, Prometheus, and telemetry demo build/runtime bases. Tempo SBOM was not generated because its build failed. This is a partial artifact set, not completion of the full planned matrix.
- The corrected branch head is `e80bf366a888f8b35299846a8f2dd32ab225119e`. GitHub Contents API updates did not trigger PR workflows for this commit. A user-authenticated GitHub Web commit is required to trigger fresh CI and Security against this exact head. PR #90 remains open and must not be merged until the new head's CI, Tempo VEX-aware scan, and complete SBOM output pass.
- Production tenant ingestion remains disabled pending the separate real-environment gates listed above.
### Web-triggered CI checkpoint — 2026-11-02

Run fresh CI and Security validation against the corrected Tempo build revision assertion.
Verify the VEX-aware Tempo scan and complete CycloneDX SBOM artifact set.

## M06 PR #90 merge completion — 2026-11-02

PR #90 was squash-merged into `main` as `4a4c46445d7f6b0129cba647d1bd71c49bbd0d45`. The final PR head `d388b4702f1dca3b26f55a5fa8ef1e93328c98ee` passed all CI jobs in run `41406376148`, all 13 Security jobs in run `41406376150`, and Pages-source check `41406376156`. The complete 11-artifact CycloneDX SBOM set was uploaded with 30-day retention.

The final telemetry CI job passed all stages: private network/Compose validation, telemetry stack startup and readiness, OTLP metrics/logs/traces through the ORBIT API, Prometheus Collector scrape, authenticated workspace A-vs-B write-to-query isolation, and cleanup. Security confirmed the pinned Tempo upstream release revision, the VEX-aware Trivy result, the telemetry demo dependency fixes, Gitleaks, govulncheck, npm audit, and Dependency Review.

The corresponding completion checkboxes are updated in `docs/roadmap.md`. Remaining work is explicitly separated into environment-level gates and policy decisions; production tenant ingestion remains disabled until they pass.

## M04 PR #104 — workspace-scoped workload resource attributes — 2026-11-02

Branch: `feat/m04-workload-resource-attributes`.

- Adds migration `016_workspace_otlp_resource_attributes.sql`: telemetry service uniqueness becomes `(workspace_id, name)`, and `otlp` becomes an allowed telemetry source.
- Adds a curated resource attribute extractor for OTLP metrics, logs, and traces. It runs only after successful workload token verification, workspace/signal grant authorization, rejection of producer-supplied tenant fields, and injection of the canonical workspace ID.
- Captures only reviewed low-risk semantic-convention keys; excludes user identifiers, command lines, pod UIDs, arbitrary attributes, and unsupported composite values. Bounds are 32 keys, 8 values per key, and 256 bytes per string.
- Adds PostgreSQL persistence with an independent allowlist check, per-attribute `otlp` provenance, and workspace-keyed upserts. Legacy unscoped catalog queries and attribute updates are explicitly limited to `legacy-default` so the old catalog cannot reveal other workspaces.
- Wires both HTTP and gRPC gateways. Metadata persistence occurs only after a successful Collector export; a metadata-store failure emits an operational warning without changing the successful OTLP response.
- Adds extraction/allowlist tests, gateway recording tests, PostgreSQL workspace A/B persistence tests, and extends the real workspace write-to-query E2E to assert per-workspace resource metadata. Final PR-head CI run `41413122520` passed all four jobs, including Go race tests/build, vet, PostgreSQL backup/restore, web typecheck/build, and the full telemetry pipeline with workspace A/B resource-catalog assertions. Final PR-head Security run `41413122598` passed all 13 jobs and uploaded all 11 CycloneDX SBOM artifacts.
- Adds the workspace telemetry catalog route to OpenAPI and documents the source provenance. The TypeScript client now exposes a separate workspace catalog source type (including `otlp`) and `listWorkspaceTelemetryServices`, leaving the backend-discovery source union unchanged.

PR #104 was squash-merged into `main` on 2026-11-02 as `fa3223e16811f61bf760d8d0b54adee7ad5245f4`. The final PR head was `ec4a5ff70fb9b1344698df5049ebbd63423dd381`; its CI and Security checks passed. GitHub Pages deploy run `41414101646` on the merge commit succeeded. Post-merge `main` CI run `41414101520` and Security run `41414101510` were started to revalidate the merge commit. Production tenant ingestion remains disabled pending the separate production ingress, real identity-provider, encrypted off-host backup/PITR, sustained-load, and operational acceptance gates.

## M01 accessibility baseline — keyboard navigation and landmarks

Branch: `feat/m01-accessibility-review`.

- Adds an English/Persian skip link to the primary content and a focusable `main` landmark. The application shell itself is a neutral container rather than incorrectly acting as the page's main landmark.
- Gives the sidebar an explicitly labelled `nav` landmark, semantic section headings, and `aria-current="page"` on the active view.
- The mobile navigation trigger exposes `aria-expanded` and `aria-controls`. Opening the drawer moves focus to its close control; Escape closes it; Tab/Shift+Tab stay within it; closing returns focus to the trigger. A closed off-canvas drawer is hidden from keyboard navigation.
- Applies a shared `:focus-visible` indicator to buttons, links, inputs, selects, textareas, and focusable targets. Existing reduced-motion support is retained.
- Adds `scripts/check_accessibility_contract.py` to the web CI job to prevent regressions in the static keyboard/landmark contract.

This is a baseline implementation, not a completed screen-reader audit. NVDA, VoiceOver, and TalkBack have not been exercised by CI and require manual verification. Color contrast in both themes, chart/data alternatives, and remaining non-functional demo controls also require manual review. Keep the second M01 accessibility checkbox open until those acceptance checks are performed.

## M01 accessibility baseline — keyboard and landmark improvements

Branch: `feat/m01-accessibility-review`.

- Adds a localized skip link to a focusable `main` landmark; the app shell remains a neutral container.
- Gives the sidebar a named primary-navigation landmark, semantic group headings, and an `aria-current="page"` marker for the active view. The current view heading is announced politely after client-side navigation changes.
- Adds expanded/controlled state to the mobile menu button, initial focus on the drawer close button, Escape close, Tab/Shift+Tab containment, focus restoration, and removes the closed off-canvas drawer from keyboard navigation.
- Adds visible focus styles for links and form controls, including a darker outline against light-theme surfaces. Skip-link positioning uses logical inline-start for RTL.
- Adds `scripts/check_accessibility_contract.py` to the web CI job. This static regression check is intentionally not presented as an axe/WCAG or assistive-technology audit.

The M01 accessibility item remains partially open until manual keyboard-only and screen-reader testing with NVDA, VoiceOver, and TalkBack, rendered-state contrast verification, manual verification of both implemented chart tables, and review of remaining demo-only controls. See `docs/accessibility-review.md` for the acceptance checklist.

## M01 accessibility follow-through — truthful control affordances (2026-11-02)

The existing M01 accessibility branch now also addresses the visible-control audit in PR #109:

- The top-bar Notifications button opens the existing Notifications view instead of acting as a no-op.
- The Docs affordance links to the repository documentation index.
- The workspace switcher, Settings, Help/Support, and incident-options placeholders are native disabled controls with localized unavailable labels and a visible disabled treatment; they no longer masquerade as working actions.
- The static accessibility contract checks guard these destinations, disabled states, and English/Persian unavailable text. docs/accessibility-review.md records the actual behavior and updates the remaining manual control checklist.

PR #109 has since merged this tested keyboard/landmark baseline into `main`. The merge does not close the manual M01 acceptance gate: the application still requires real keyboard-only testing at browser breakpoints, NVDA/VoiceOver/TalkBack verification, rendered contrast review in both themes, and review of chart/data alternatives. Always inspect CI and Security against the exact latest PR head before merging future changes; passing runs on older commits do not validate later code or documentation changes. Do not mark M01 complete or claim screen-reader/WCAG conformance until the human findings are recorded.


## M01 accessibility follow-through — selected-state semantics and verification (2026-11-03)

The implementation described in this historical PR #109 follow-through was merged into `main`. In addition to the keyboard/landmark baseline and truthful control affordances, it exposes selected state to assistive technology for:

- the overview demo chart time-range selector (named group, localized in English/Persian);
- incident status filters and the selected incident;
- alert-instance filters;
- notification-delivery filters;
- the selected SLO objective.

Audit-log filters and telemetry metric-group controls already exposed selected state and were preserved. The static accessibility contract now checks these additions; `docs/accessibility-review.md` carries the user-facing implementation and manual-acceptance checklist.

The code head `086137db0a05fbdf8ca050a6f8ce4730a31bb0ba` passed CI run `41473967465` (all four jobs) and Security run `41473967524` (all 13 jobs). The web job explicitly passed the accessibility contract, TypeScript typecheck, and production build. All 11 expected CycloneDX SBOM artifacts were uploaded and unexpired. These automated results do not close manual acceptance: keyboard-only browser testing, NVDA/VoiceOver/TalkBack, contrast in both themes, chart/data alternatives, and final control review remain open. Do not mark M01 accessibility complete solely from automated CI results; PR #109 has since merged the tested baseline while the human acceptance gate remains open.

Production tenant ingestion remains disabled until the environment-specific M06 gates elsewhere in this document and `docs/roadmap.md` have recorded evidence.

## M01 accessibility follow-through — generated HTML regression audit (2026-11-03)

PR #109 adds `scripts/check_accessibility_html.py` and runs it after `npm run build` in the web CI job. It inspects the actual static HTML export (rather than only checking JSX source strings) for the document language/title, primary landmarks, heading presence, skip-link target, duplicate IDs, accessible names on rendered links/buttons and role-based controls, form-control labels, image alternatives, and ARIA ID references.

The checker uses only Python's standard library and needs the production export in `out/`. It is a structural regression check, not browser automation, an accessibility-tree/axe scan, contrast verification, WCAG certification, or screen-reader evidence. Keep the M01 manual acceptance checkbox open until the keyboard, NVDA/VoiceOver/TalkBack, contrast, chart-alternative, and control review has been completed. The new checker and its CI integration must be verified on the exact latest PR head; do not reuse a run from a preceding commit.

## Temporary Pages preview — M01 accessibility branch (2026-11-03)

PR #109 temporarily added a feature-branch publishing trigger and a separate `github-pages-preview` environment. Pages is a single-site target, so that preview replaced the canonical static site rather than creating a separate URL. After PR #109 merged, commit `5649a1f01fdb832fcf886021cfdf647fc5851cc0` removed the feature-branch trigger and alternate environment routing. The protected `github-pages` environment remains the only deployment environment and `main` is the only push publishing source. The main-only deployment completed successfully in run [41579531993](https://github.com/pooya16pg-spec/orbit/actions/runs/41579531993). Do not weaken the protected environment or re-add feature branches as deployment sources for ordinary review.
## M01 accessibility follow-through — theme contrast regression baseline (2026-11-03)

- The theme palette was checked for low-contrast text. The `--subtle` token was below 4.5:1 on common dark and light surfaces; multiple explicit light-theme text colors were also too pale for normal-sized text.
- Updated dark/light `--text`, `--muted`, `--subtle`, and `--mint` palette values so their tested combinations meet at least 4.5:1. Dark and light keyboard focus indicators are checked against the palette at 3:1 or better.
- Darkened 75 explicit light-theme text/icon/status colors against the known pale dashboard surfaces. The light primary-action background was also darkened so its white label has sufficient contrast.
- Added `scripts/check_accessibility_contrast.py` to the web CI job. It checks base theme tokens, focus outline contrast, and explicit light-theme text colors against the project's pale surface set.
- This is a focused CSS palette regression check, not a rendered-page computed-style scan. Keep the manual contrast acceptance item open for actual browser states, hover/selected/disabled/loading/error states, overlays, and any component-specific surfaces not represented in the static palette set. Do not claim WCAG conformance from this script alone.

The generated HTML structural audit remains in the same CI job after production build. Together, the source contract, contrast palette check, and generated HTML audit are intended to catch regressions early; they do not replace real keyboard-only and NVDA/VoiceOver/TalkBack testing.

## M01 chart alternative follow-through — Metrics time-series data table (2026-11-03)

The live Metrics view now renders an expandable native HTML table under the time-series chart. It exposes a caption naming the selected service and time window plus semantic column headers for localized time, series, and request rate. Values come from the same bounded Prometheus response as the chart, not a separate or synthetic data source. The table is styled for horizontal scrolling on narrow screens and inherits the application's LTR/RTL direction.

The selected locale formats both timestamps and metric values. New English and Persian strings label the disclosure, caption, and columns. The source-level accessibility contract checks the details/summary, caption, headers, locale formatting calls, translations, and responsive table container.

This Metrics-table note predates the Overview table added below. Both charts now expose expandable table alternatives; verify both chart experiences with keyboard-only and screen-reader testing before closing M01.
## M01 chart alternative follow-through — Overview sample table (2026-11-03)

The Overview SVG chart's static coordinate fixture has been factored into a shared coordinate list in `lib/orbit/demo.ts`. The SVG polyline/polygon path and the accessible table rows are derived from the same list. Each illustrative value is explicitly documented as an estimated requests-per-minute sample derived from the displayed 0–30k axis; the table labels the unit as requests/minute and marks it illustrative. These are not production or measured telemetry.

The Overview disclosure table includes a caption stating the data is illustrative, localized time/value columns, and timestamps positioned across the currently selected one-hour, six-hour, or 24-hour window. Its x-axis labels use the same range-aware formatter, fixing the previous mismatch where the graph always displayed 09:00–10:00 regardless of selected range.

The source-level accessibility contract checks the table, caption, selected-window time formatting, shared sample fixture, and Persian/English labels. Both Overview and live Metrics charts now have an expandable table alternative. Manual browser/keyboard/screen-reader checks remain mandatory.

## M01 verification snapshot — pre-merge PR head (2026-11-03)

Pre-merge application/test head: `5e1e37cf789bfed9baf971f914a9df895e328ea3` from PR #109.

- CI run [41578302352](https://github.com/pooya16pg-spec/orbit/actions/runs/41578302352): web verification passed. All **14 Playwright tests** passed, including axe scans across all 12 primary views in both dark and light themes, plus dedicated Overview/Metrics/mobile-drawer/error scans, reduced-motion behavior, keyboard navigation, and 320 CSS-pixel reflow.
- The source accessibility contract passed **68 checks**. The theme contrast regression guard, TypeScript typecheck, production build, and generated-HTML audit also passed; the latter checked **5 pages / 791 elements**.
- Go API tests/race/vet/build, workflow action-pin policy, PostgreSQL backup/restore round-trip, and the full local telemetry pipeline end-to-end job all completed successfully in run [41578302352](https://github.com/pooya16pg-spec/orbit/actions/runs/41578302352).
- Security run [41578302399](https://github.com/pooya16pg-spec/orbit/actions/runs/41578302399) passed all **13 jobs**, including dependency checks, full-history secret scanning, production/telemetry image scans, and SBOM generation.
- Pages-source verification [41578302366](https://github.com/pooya16pg-spec/orbit/actions/runs/41578302366) and temporary branch deployment [41578298625](https://github.com/pooya16pg-spec/orbit/actions/runs/41578298625) passed before merge. After PR #109 merged, the feature-branch publishing trigger was removed; main-only Pages deployment [41579531993](https://github.com/pooya16pg-spec/orbit/actions/runs/41579531993) succeeded from the protected `github-pages` environment. Canonical site: https://pooya16pg-spec.github.io/orbit/.
- PR #109 was squash-merged into `main` as `e9e7d74387dddcf76226cfab32dd979f56af4dc9`.

Automated Chromium/axe checks do not replace M01's manual acceptance: keyboard-only operation across all views/forms, NVDA/VoiceOver/TalkBack behavior, responsive zoom/reflow and RTL, reduced-motion behavior, rendered contrast in all interactive states, and a human review of chart/table reading experience and visible controls remain outstanding. PR #109's automated baseline is merged; do not mark M01 complete solely from CI results.


## M01 accessibility baseline merge and current handoff — 2026-11-03

PR #109 was squash-merged into `main` as `e9e7d74387dddcf76226cfab32dd979f56af4dc9`.

The merged application/test head was `5e1e37cf789bfed9baf971f914a9df895e328ea3`; the two subsequent commits before merge changed only the accessibility review, roadmap, and handoff documentation. Final verification:
- 14/14 Playwright browser tests passed, including 24 axe scans (12 primary dashboard views in both dark and light themes), keyboard navigation and mobile drawer focus handling, reduced motion, responsive reflow, and keyboard chart-table scrolling.
- 68 accessibility source-contract checks, theme contrast regression checks, TypeScript typecheck, production build, and generated HTML audit (5 pages / 791 elements) passed.
- Go race tests, vet, build, module-lock verification, and PostgreSQL backup/restore CI round-trip passed.
- Full local telemetry E2E passed: all three OTLP signals reached and were queryable from the configured local backends through ORBIT API; Prometheus scraped the Collector; authenticated workspace A/B write-to-query isolation passed.
- Latest Security run `41578754427` passed all 13 jobs, including dependency/secret checks, image scans, and CycloneDX SBOM generation.

The temporary feature-branch GitHub Pages publishing trigger and alternate preview environment routing are removed by the post-merge cleanup commit. The canonical Pages deployment remains on the protected `github-pages` environment and is published from `main` only.

**M01 is not fully closed:** the human keyboard-only and rendered-state review, and real NVDA/VoiceOver/TalkBack sessions, remain open in `docs/roadmap.md` and `docs/accessibility-review.md`. Remote automation did not run those assistive technologies and does not establish WCAG conformance. Keep this explicitly open until actual OS/browser/AT versions and findings are recorded.

Repository-level deterministic backpressure/recovery coverage at the real PostgreSQL + Collector boundary is now merged in PR #110 (`b42b4b56a82a2f977b373731ae6e85a1ceb0760e`). The current remaining M06 load gate is target-environment sustained ingestion and dashboard-query testing with p50/p95/p99 latency, rejection/saturation metrics, multiple replicas, and measured recovery; CI-sized local dependencies do not certify production capacity.


## M06 backpressure integration test — current follow-through

The Docker Compose workspace-tenant smoke runs `TestWorkspaceOTLPSustainedBackpressureAndRecovery` against real PostgreSQL-backed workload authorization/shared rate limiting and the real local OTLP Collector. A deterministic HTTP gate holds 64 in-flight exports at the network boundary; eight excess exports must be rejected with 503; the admitted exports must succeed after release; and three 16-export concurrent waves must recover through the same gateway and dependencies. The test then sustains eight producer streams for 20 seconds.

PR #110 was squash-merged into `main` as `b42b4b56a82a2f977b373731ae6e85a1ceb0760e`. CI run `41580189083` passed all four jobs and logged **639 successful exports over 20 seconds** in the sustained real-PostgreSQL + Collector phase. Security run `41580189031` passed all 13 jobs. This closes the repository-level deterministic real-dependency backpressure/recovery regression. It does not certify target-environment throughput, p50/p95/p99 latency, multiple replicas, host/network failure, or production recovery; those gates remain open.


## Current handoff — after M06 backpressure PR #110 (2026-11-03)

Latest main commit at handoff: `b42b4b56a82a2f977b373731ae6e85a1ceb0760e`. PR #110 is merged; there are no unmerged M06 backpressure changes. Exact final PR head `e00b0308bf0bf3d61e095975ee69546e349d0c43` passed:
- CI [41580189083](https://github.com/pooya16pg-spec/orbit/actions/runs/41580189083): all 4/4 jobs successful (14 Playwright tests and web/a11y/build checks; Go tests/race/vet/build and PostgreSQL backup/restore; action-pin policy; full Docker observability integration).
- The actual Compose PostgreSQL + Collector integration test filled 64 gateway admission positions, verified 8 over-capacity HTTP 503 responses, passed three concurrent 16-export recovery waves, and then completed **639 successful exports during the 20-second steady phase**. The new test passed in 20.10 seconds; combined with the existing workspace isolation test the command passed in 66.18 seconds.
- Security [41580189031](https://github.com/pooya16pg-spec/orbit/actions/runs/41580189031): all 13/13 jobs passed, including full-history secret scan, dependency vulnerability checks, image scans, and CycloneDX SBOM generation.

Roadmap item “explicit backpressure and recovery-under-sustained-load tests across the real dependency boundary” is now complete (PR #110). Keep “target-environment load tests” open: measure throughput and p50/p95/p99 on intended hardware, test dashboard queries, multi-replica behavior, rejection/saturation, and recovery. Keep production DNS/ACME/firewall/trusted-proxy/network controls, provider-backed PKCE, off-host backup/PITR and measured restore, durable-history retention policy, hosted API/managed persistence, and operational rollout gates open. Production tenant ingestion remains disabled until these deployment-level gates are evidenced.

PR #109's automated M01 keyboard/landmark baseline is merged. The separate human keyboard-only, rendered-state visual, and actual NVDA/VoiceOver/TalkBack acceptance checklist remains open; no WCAG or screen-reader certification is claimed.


## M06 PostgreSQL container-restart recovery regression — merged PR #111

The next repository-level M06 test uses the Compose PostgreSQL service rather than a TCP proxy. A coordinated host script stops and restarts the actual PostgreSQL container while a Go test retains the original Repository/pgxpool instance. The test requires an established connection to fail during the stop, then a query to succeed from the same pool after restart, and compares `pg_postmaster_start_time()` to prove that the PostgreSQL server process itself restarted.

The test uses a shared temporary marker directory only for runner/test synchronization. The test itself performs all database observations and recovery assertions. The Compose runner always attempts to start PostgreSQL in its cleanup trap so a failed test cannot strand the rest of the telemetry stack. This is a CI single-node clean-restart regression, not crash/failover/PITR or production recovery evidence.


## Current handoff — after PostgreSQL container-restart PR #111 (2026-11-03)

Latest main commit at this handoff: `5b3c0fc344f043640b01c670a84deb6c78d09a5c`. PR #111 is merged; its exact head passed CI 4/4 and Security 13/13. The Docker observability job tested OTLP end-to-end, workspace isolation, and an actual PostgreSQL container stop/start; the same Repository/pgxpool recovered and observed a newer `pg_postmaster_start_time()` after restart.

Repository-level M06 regression coverage now includes: actual Collector listener stop/restart, PostgreSQL TCP-path interruption/recovery, sustained real-PostgreSQL + Collector backpressure (PR #110; 639 successful exports in its 20-second CI phase), and clean PostgreSQL container/process restart (PR #111). Avoid repeating these tests or adding duplicate roadmap entries.

Remaining deployment gates are distinct: target-environment sustained load and p50/p95/p99 measurements, dashboard-query load, multiple gateway replicas under real workload, crash/power-loss and storage failure testing, HA/failover/replication and partition behavior, encrypted off-host backups/PITR with measured RPO/RTO, real DNS/ACME/firewall/trusted-proxy/network controls, provider-backed PKCE, durable-history retention/archive policy, hosted API/managed persistence, and operational rollout. Production tenant ingestion remains disabled until those environment-specific gates are evidenced.
