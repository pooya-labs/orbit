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
- **M06 API security foundation:** OIDC access-token verification through issuer discovery/JWKS, RS256/ES256 allowlisting, centralized viewer/operator/administrator route policy, explicit development mode, fail-closed staging/production startup, body/token caps, and PostgreSQL-backed shared per-IP/per-subject rate limits are merged in `main` (`977ce49`) and CI-verified. The shared PostgreSQL fixed-window rate limiter is merged; staging and production require PostgreSQL so rate limiting cannot silently fall back to per-process memory.
- **Security work still open:** real browser OIDC provider registration and end-to-end browser/API verification, token revocation/global logout, workspace membership and tenant-scoped database queries, transactional audit records for M05 operations, and production deployment controls. Until workspace scoping exists, the API must not be advertised as multi-tenant.

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

- real OIDC provider registration, callback/CORS/audience end-to-end verification, and provider token revocation/global logout (provider-neutral browser PKCE is isolated on PR #12);
- workspace/tenant membership and database scoping;
- actor identity and transactional audit records for every state-changing operation;
- bounded backend concurrency for horizontally scaled deployments;
- secret management, CORS allowlists, dependency review, backups/retention, cardinality guardrails, and operational runbooks.

Local development may omit OIDC only when `ORBIT_ENVIRONMENT=development` is explicitly set. Startup must fail closed if staging/production is configured without valid authentication settings. Health/readiness endpoints should reveal status, not credentials or sensitive topology.

## 8. Related portfolio context

The GitHub account also contains separate portfolio projects (Pooya.OS, AuraPulse, NexusCanvas, and LuminaGlass). ORBIT is its own repository and backend architecture, not a shared monorepo. Reuse visual/design conventions where appropriate, but do not couple their runtime state, APIs, or deployment pipelines without an explicit product decision.

## 9. M06 security handoff

The initial server-side OIDC access-token and RBAC gate is merged in `main` as `977ce49` and passed CI run `41288773472`. Its tests cover signature tampering, RS256/ES256, issuer/audience/time-claim failures, unknown `kid` and JWKS rotation, role enforcement, CORS Authorization preflight, request limits, and startup failure when staging/production OIDC settings are absent. This completes only the API resource-server foundation, not all of M06. Browser PKCE/token lifecycle, workspace membership and tenant-scoped queries, real-provider browser PKCE integration, workspace membership enforcement and tenant-scoped queries, secrets, retention, production deployment, and runbooks remain open.

The API currently accepts access tokens only through `Authorization: Bearer`; it never reads credentials from URLs or browser storage. A provider-neutral static-dashboard PKCE client is under review on PR #12. It is not deployed and has no real issuer/client configured yet.

## 10. Continuity rule

When resuming work, start with:
1. `docs/roadmap.md` for milestone status;
2. this file for project history and boundaries;
3. `docs/architecture.md` for current system shape;
4. the latest GitHub Actions run and open PRs for verified code state.

Then continue on a feature branch, update tests and docs with implementation, and keep the public Pages deployment unchanged until the relevant PR is merged and deployment CI succeeds.


## 11. Workspace and audit foundation (M06)

The shared PostgreSQL rate limiter is merged in `db45843`. The provider-neutral browser PKCE client remains isolated on PR #12 until a real identity provider is configured and the browser-to-API flow is tested.

Migration 005 introduces workspaces, issuer+subject-based memberships, one-owner-per-workspace enforcement, and append-only workspace audit events. Repository operations create a workspace and its owner membership atomically, resolve a principal's memberships, and append/read bounded audit events. Authenticated workspace create/list, membership lookup, and membership-checked audit-read endpoints are being added on `feature/m06-workspace-api-membership`.

This schema and repository layer alone does **not** make ORBIT multi-tenant. Existing service, incident, and telemetry queries are still global. Do not expose workspace creation or claim tenant isolation until every data query and mutation is scoped to a verified workspace membership, existing records are safely assigned, and cross-workspace denial tests pass.


## Workspace data-scope foundation (PR in progress)

Migration 006 assigns pre-existing service, incident, and telemetry-catalog rows to a dedicated `legacy-default` workspace. It adds `workspace_id` foreign keys and lookup indexes; incidents also have a composite foreign key to ensure their service belongs to the same workspace. New repository methods and authenticated routes read services, incidents, and persisted telemetry catalog strictly by workspace after issuer+subject membership verification.

This is an incremental isolation boundary, **not completion of multi-tenancy**. Existing unscoped endpoints remain for backward compatibility, telemetry backend queries (Prometheus/Loki/Tempo) are not yet partitioned by workspace, and legacy telemetry ingestion is intentionally pinned to `legacy-default`. Do not enable tenant-facing production access until legacy routes are retired or explicitly restricted, telemetry backend labels are workspace-aware, and all write/ingestion paths enforce workspace ownership.


## Legacy global read-route safety gate (PR in progress)

The follow-up security slice disables the unscoped `/api/v1/services`, `/api/v1/incidents`, and legacy telemetry catalog/metrics/logs/traces read routes in staging and production. They remain available in explicit development mode for local dashboard compatibility. Workspace management and membership-checked workspace-scoped inventory/catalog routes are not blocked by this gate.

This is a fail-closed compatibility boundary, not full telemetry tenant isolation: Prometheus, Loki, and Tempo queries still need workspace-aware labels/tenant routing, and telemetry ingestion still needs authenticated workspace ownership. Do not enable tenant-facing telemetry until those paths are partitioned and cross-workspace tests cover them.
