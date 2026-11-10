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
- [x] Require GitHub Actions as the Pages publishing source and verify it before deployment; document the branch/Jekyll conflict and recovery steps (PR #88; Pages-source check and full CI verified).
- [x] Add a user-selectable, persistent light/dark dashboard theme while retaining the established dark palette (PR #35).
- [x] Add baseline keyboard accessibility: skip-to-main link, explicit main/navigation landmarks, current-page announcement, visible focus indicators, and mobile navigation Escape/Tab/focus-return behavior. CI guards source semantics, the dark/light palette and axe-discovered component text colors, and the generated static HTML export.
- [x] Add expandable native data tables for both charts; live Metrics rows use the bounded Prometheus response, while Overview uses clearly labelled illustrative samples derived from the same SVG fixture.
- [x] Add 14 Chromium/Playwright interaction tests and axe WCAG 2.1 A/AA + WCAG 2.2 AA scans. On code/test head `5e1e37c`, CI run [41578302352](https://github.com/pooya16pg-spec/orbit/actions/runs/41578302352) passed all 14 browser tests, including axe scans of all 12 primary views in both dark and light themes, reduced-motion behavior, keyboard chart-table scrolling, 68 accessibility-contract checks, generated-HTML audit (5 pages / 791 elements), typecheck, and production build. Go API tests/race tests/vet/build, action-pin policy, PostgreSQL backup/restore round-trip, and the full telemetry E2E pipeline also completed successfully. The latter verified metrics, logs, and traces through ORBIT API, Prometheus Collector scraping, authenticated workspace A/B write-to-query isolation, and cleanup. Security run [41578754427](https://github.com/pooya16pg-spec/orbit/actions/runs/41578754427) passed all 13 jobs on the final docs/code head, including API/Collector/Caddy/Tempo/Compose scans and the complete SBOM set. Pages-source verification [41578754453](https://github.com/pooya16pg-spec/orbit/actions/runs/41578754453) and branch deployment [41578749277](https://github.com/pooya16pg-spec/orbit/actions/runs/41578749277) passed. PR #109 was squash-merged into `main` as `e9e7d74387dddcf76226cfab32dd979f56af4dc9`.
- [x] Extend rendered-state axe coverage to keyboard-focused and hovered primary navigation in both themes, plus the API connector's deterministic connecting/disabled-submit and structured request-error states (PR #114). Exact head `0f8d87245840d638d2a6ca886180683848282ca2`; CI [41603839829](https://github.com/pooya16pg-spec/orbit/actions/runs/41603839829) passed all 4 jobs and **16/16 Playwright tests**, and Security [41603839542](https://github.com/pooya16pg-spec/orbit/actions/runs/41603839542) passed all **13/13 jobs**. The additional tests run axe in each interaction state without suppressing rules or reducing contrast thresholds.
- [x] Stabilize navigation focus-state coverage with actual sequential keyboard navigation in each theme: set the theme preference before a fresh document load, use two native Tab presses to focus the skip link and Overview, then assert :focus-visible and run axe (PR #116). Exact head `6bb1157734d39b45feb194961b6343a73b7a4da0`; CI [41606123956](https://github.com/pooya16pg-spec/orbit/actions/runs/41606123956) passed all 4 jobs and **16/16 Playwright tests**; Security [41606123969](https://github.com/pooya16pg-spec/orbit/actions/runs/41606123969) passed all **13/13 jobs**. PR #117's blur-based alternative was closed after CI failed to focus the skip link on the first Tab; no duplicate implementation was merged.
The owner confirmed the manual keyboard/device acceptance is complete on 2026-11-10; no formal WCAG-conformance claim is made.
- [x] Add a Windows-hosted real NVDA Browse Mode smoke test: capture Speech Viewer through Windows UI Automation and verify the three sidebar H2 headings plus the Overview H1 (PR #124; exact NVDA run [41633391216](https://github.com/pooya-labs/orbit/actions/runs/41633391216) passed on NVDA 2026.2).
- [x] Extend real NVDA coverage through all primary dashboard views: verify Ctrl+Home announces the skip link, then use native Tab/Enter to traverse Overview and the other 12 views and assert each page heading is announced in Speech Viewer (PR #125; tested script head `e359be15216cccc9aaf48e11dd89b7475f9a9eff`; exact NVDA run [41637653806](https://github.com/pooya-labs/orbit/actions/runs/41637653806) passed).
- [x] Owner-confirmed manual keyboard, device, and assistive-technology acceptance completed (2026-11-10). This is owner-provided acceptance evidence; it does not constitute a formal WCAG audit or certification.

The contrast regression check covers the base palette plus explicit light-theme text colors against a reviewed set of pale surfaces. Both the live Metrics chart and illustrative Overview chart now expose expandable table alternatives. Metrics uses its Prometheus response; Overview uses clearly labelled samples derived from the same SVG fixture and updates its time labels to the selected window. Manual verification of the chart/table relationship and assistive-technology experience remains open.

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
- [x] Implement the typed API client, Demo/API source switch and browser API configuration. The live host, CORS and OIDC connection is tracked once in the M06 free-only deployment gate.

## M03 — Local platform

The optional `observability` profile accepts OTLP over gRPC/HTTP and routes metrics to Prometheus, traces to Tempo, and logs to Loki. An instrumented Go demo emits all three signals; GitHub Actions verifies the complete path through the backends and ORBIT API. The production Go API and background workers also support opt-in, bounded OTLP self-telemetry to the private Collector (M06; PR #133).

- [x] Add Docker Compose for PostgreSQL, Redis, and NATS JetStream.
- [x] Add the Go API container and database readiness dependency.
- [x] Add deterministic, clearly labelled demo seed rows.
- [x] Add an optional OpenTelemetry Collector and Prometheus profile with a local smoke test.
- [x] Add Loki and Tempo local backends with OTLP export pipelines.
- [x] Add an instrumented Go service example with OTLP export for metrics, traces, and logs, plus an end-to-end backend smoke test.
- [ ] Wire Redis cache and NATS event publishing into the API only if measured demand justifies them (deferred by [ADR 0004](decisions/0004-redis-nats-deferred.md): no current cache bottleneck or second durable event consumer; PostgreSQL remains the source of truth and notification outbox).

## M04 — Real telemetry

The v0.6.0 API queries the local Prometheus, Loki, and Tempo stores through bounded, source-labelled endpoints. The v0.7.0 dashboard renders Metrics, Logs, and Traces views with selectable time windows, refresh, query timestamps, and explicit loading/empty/error states. The v0.8.0 preview adds merged service discovery with backend provenance and a shared dynamic selector. The v0.9.0 preview persists observed services and source relationships in PostgreSQL and adds Persian localization with RTL layout. Queries are fixed and do not accept arbitrary PromQL/LogQL from callers; the current request-rate metric is the instrumented demo counter.

- [x] Verify OTLP metrics, traces, and logs reach all three local backends from an instrumented workload.
- [x] Add bounded ORBIT API queries for Prometheus request-rate metrics, Loki service logs, and Tempo trace search.
- [x] Discover service names across Prometheus, Loki, and Tempo, merge duplicates, and preserve backend provenance.
- [x] Persist observed service names and backend provenance in a normalized PostgreSQL catalog.
- [x] Expose persisted first-seen, last-seen, and last-discovered metadata through the service discovery API.
- [x] Capture curated OpenTelemetry resource attributes directly from authenticated OTLP workloads over HTTP and gRPC, persist them per workspace with otlp provenance, and verify attribute isolation in the real PostgreSQL/Collector/backend E2E pipeline (PR #104; only a reviewed low-risk allowlist is accepted).
- [x] Review the current projection against OpenTelemetry Resource Semantic Conventions 1.44.0, document explicitly deferred identity/sensitive attributes, and add CI checks for ingress/persistence allowlist parity and matching bounds (PR #126 / ADR 0005). No new attributes were added without a concrete use case.
- [ ] Expand the workload resource semantic-convention allowlist only when a concrete consumer, privacy/cardinality review, and operational budget justify it; arbitrary producer attributes remain intentionally unsupported.
- [x] Wire the Metrics/Logs/Traces dashboard to the authenticated workspace-scoped catalog and query APIs; select an authorized workspace, verify membership before querying, and keep legacy unscoped telemetry reads out of staging/production (PR #128; exact head `affc4cd8a018ecfc8e42b0889d08292f427da7db`; CI, Security, and NVDA passed before squash merge `d495dbdf862855f542cb0ac018a0f093f22dd48c`).
- [x] Add the authenticated workspace metric catalog and selector (PR #132): `request_rate`, `successful_request_rate`, and `failed_request_rate` use fixed templates over `orbit_demo_requests_total`; exact workspace/service matchers are server-derived, only three grouping labels are accepted, result series are capped at 10, windows at 24 hours, and Prometheus timeout at five seconds. Arbitrary metric names, label matchers, and PromQL remain rejected. Success/failure views require producer-emitted `slo_outcome=good|bad`; missing outcome data is not inferred.
- [x] Add interactive workspace trace detail with membership and tenant verification on every lookup (PR #130), then explicit Logs-to-Trace correlation (PR #131). Correlation accepts only non-zero 32-character hexadecimal IDs parsed from log bodies or existing stream fields; the UI reuses the authenticated workspace trace-detail endpoint. Trace IDs are not added as high-cardinality Loki index labels.
- [x] Persist bounded service resource attributes and per-attribute backend provenance in PostgreSQL.
- [x] Add validated cursor-based pagination to the service catalog API and dashboard selector.
- [x] Add allow-listed multi-series grouping and separate line rendering for the request-rate metric; arbitrary metric-name selection remains future work.
- [x] Render queried telemetry in Metrics, Logs, and Traces dashboard views with loading/empty/error states, query freshness, and backend provenance.
- [x] Flag stale metric samples and gaps using the actual Prometheus sample timestamps and query step; event-driven logs and traces are not judged against metric scrape intervals.
- [x] Ship Persian localization with right-to-left dashboard layout alongside the persisted telemetry catalog (PR #8).
- [x] Keep the Tempo trace smoke path aligned with the backend's trace-lookup operation rather than treating trace lookup as indexed search (PR #61; CI verified).

## Execution order and dependency gates

ORBIT's milestones are not a reason to expose unsafe operations early. The delivery order is:

1. Finish and merge the verified M04 read path and curated workspace-scoped OTLP resource capture. Expand semantic-convention coverage only through an explicit privacy/cardinality review; do not persist arbitrary producer attributes.
2. The initial M06 API security gate from [ADR 0001](decisions/0001-oidc-access-token-authentication.md) is merged in `977ce49`. Shared PostgreSQL-backed IP and subject rate limiting is merged in `db45843` and CI-verified. Provider-neutral browser PKCE is implemented in PR #12; it remains isolated from the public site until a real provider is configured and end-to-end validation passes. Workspace membership and tenant-scoped data access remain the next security boundary.
3. Deliver M05 in slices: incident lifecycle and immutable timeline first; alert rules and notification adapters second; deployment annotations third; SLO/error-budget calculations fourth; audit history across every mutation.
4. Complete the remaining M06 operational hardening: retention/cardinality controls, load testing, backup/restore, dependency/security review, production deployment, and runbooks.

M05 state-changing operations are exposed only through authenticated workspace-scoped routes. The backend verifies workspace membership and operator roles, validates state transitions, and records incident, alert-rule-definition, and notification-channel mutations in workspace audit history within the same transaction. Alert notification deliveries use a PostgreSQL transactional outbox and append-only attempt history. The dashboard must keep these controls read-only until a real browser OIDC provider is configured; local development may omit OIDC only in explicit non-production mode. The workspace audit log is readable by authenticated workspace members and intentionally omits raw OIDC issuer/subject identifiers from the browser response.

## M05 — Operations workflows

- [x] Backend incident lifecycle API and immutable timeline: workspace-scoped creation, validated status transitions/reopen, and atomic workspace audit (PR #66; migration 009; CI passing).
- [x] Workspace-scoped dashboard for incident creation, allowed status transitions/reopen, detail, and immutable timeline review (PR #71; CI-verified).
- [x] Workspace-scoped alert rule definition CRUD with immutable workspace audit (PR #69; migration 010; CI passing). v1 is limited to the existing `request_rate` metric and rejects arbitrary query expressions.
- [x] Add a scheduled workspace-aware evaluator for the allow-listed `request_rate` metric, with persisted pending/firing/resolved/cancelled instances, append-only events, deduplication, telemetry-gap handling, and PostgreSQL advisory-lock coordination across API replicas (PR #72; migration 011; CI-verified).
- [x] Add the workspace alert-operations dashboard with lifecycle states, timestamps, bounded rule management, localization, and read-only demo mode (PR #73; CI-verified).
- [x] Secure webhook notification delivery and retry history: workspace-scoped channels, transactional outbox, multi-replica claims, bounded retries, and append-only attempts (PR #74; migration 012; CI-verified).
- [x] Add the workspace Notifications page for channel configuration, safe secret-configuration status, delivery filters, and append-only attempt inspection (PR #75; CI-verified).
- [x] Harden notification UI behavior: refresh expanded attempt history correctly and fail closed when an operator tries to enable a channel whose deployment-managed secret is missing (PR #76; CI-verified).
- [x] Workspace-scoped immutable deployment annotations with derived service/environment metadata, transactional workspace audit, same-service/environment chronological release comparison, and the Deployments workspace UI (PR #77; migration 013; CI-verified).
- [x] Workspace-scoped HTTP request-success SLO definitions, live error-budget calculations, explicit insufficient-data handling, audited SLO lifecycle operations, and the localized role-aware SLO workspace UI (PR #78; migration 014; CI-verified).
- [x] Add the authenticated workspace audit-log dashboard with category filters, request IDs, event details, defensive sensitive-field redaction, and automatic refresh (PR #79).
- [x] Complete audit actor context and searchable operator history: workspace-local pseudonymous actor references, role-at-event snapshots, legacy `unknown` roles where history cannot be reconstructed, append-only actor mappings, member-scoped OpenAPI, and actor display in the Audit Log (PR #82; migration 015; CI-verified). Current M05 state-changing mutations retain transactional audit records; alert-instance transitions and webhook delivery attempts retain dedicated append-only histories.

## M06 — Production hardening

The initial server-side OIDC resource-server and route-role gate and shared PostgreSQL-backed rate limits are implemented and merged. Browser PKCE is implemented in PR #12 but is not yet connected to a real provider or enabled on the public site. Workspace schema, membership/audit repository primitives, authenticated workspace management endpoints, and workload-identity administration APIs (PR #34) are merged. Authenticated OTLP HTTP/gRPC gateways enforce workload identity, workspace grants, and signal permissions. PR #36 verifies forged producer workspace attributes are rejected; PR #38 adds CI-passing mixed-resource batch rejection across all signals and both transports. PR #39 is merged and CI-passing with per-workspace query tenant-filter coverage across Prometheus, Loki, and Tempo. PR #40 is merged and CI-passing with route-level membership-before-backend-access coverage for metrics, logs, and traces. PR #42 is merged and CI-passing with oversized-request rejection for all signals over HTTP/gRPC. PR #43 verifies shared rate limits across gateway instances and transports and is merged with passing CI. PR #46 verifies atomic enforcement under concurrent load across two replicas and is merged with passing CI. PR #48 is merged with passing CI and verifies workspace-grant and workload-identity revocation across independent HTTP/gRPC gateway replicas for metrics, logs, and traces. PR #53 is merged with passing CI and adds gRPC fail-closed coverage for shared rate-limit store outages, Collector errors, and forward deadlines. PR #84 now verifies the complete local write-to-query tenant boundary across metrics/logs/traces and A-vs-B workspace queries (CI run `41383068747`). Tenant-facing production ingestion remains disabled until remaining target-environment load/failure checks, deployment network controls, real-provider PKCE, and operational gates pass.

- [x] Merge and CI-verify OIDC discovery/JWKS access-token verification and server-side viewer/operator/administrator route enforcement (`977ce49`; CI includes Go race tests, web build, and the OTLP end-to-end smoke test).
- [x] Implement provider-neutral browser Authorization Code + PKCE, state/nonce checks, memory-only access/refresh tokens, and API Bearer-token injection (PR #12; provider configuration and production rollout remain open).
- [x] Complete workspace-membership-scoped inventory, incident, telemetry-catalog, and Prometheus/Loki/Tempo query routes, with legacy global reads disabled in staging/production (migration 006; PRs #16–#18, #39–#40; CI-verified).
- [x] Implement authenticated workspace-aware OTLP ingestion with workload identity, per-signal workspace grants, canonical tenant attribution, bounded HTTP/gRPC gateways, shared rate limits, revocation checks, and fail-closed behavior (PRs #21–#56; CI-verified).
- [x] Add workspace workload-identity administration endpoints for managing workload identities and their workspace/signal grants (PR #34; CI-verified).
- [x] Persist security audit events for denied workload-telemetry authorization attempts (PR #23; CI-verified).
- [x] Bound gateway admission before expensive authorization work to prevent unbounded verifier/database pressure (PR #33; CI-verified).
- [x] Verify workspace isolation through both OTLP/HTTP and OTLP/gRPC; reject forged producer workspace attributes and mixed-workspace resource batches; verify Prometheus/Loki/Tempo tenant filtering and membership checks before backend access (PRs #32, #36, #38–#40; CI-verified).
- [x] Prove the complete write-to-query tenant boundary: a workload granted only to workspace A writes metrics/logs/traces through the authenticated gateway and real Collector; A-scoped ORBIT queries return the data and B-scoped queries do not. Direct writes to B are denied and forged producer tenant attributes are rejected (PR #84; real PostgreSQL/Prometheus/Loki/Tempo; CI run `41383068747`).
- [x] Reject oversized OTLP requests across metrics, logs, and traces over both HTTP and gRPC (PR #42; CI-verified).
- [x] Verify atomic shared workload rate limits across independent gateway instances and concurrent replicas, plus cross-replica grant/identity revocation (PRs #43, #46, #48; CI-verified).
- [x] Verify deterministic gateway saturation behavior and enforce the configured 64-slot default admission bound for HTTP and gRPC (PRs #51 and #60; CI-verified).
- [x] Verify fail-closed gRPC dependency errors, HTTP Collector timeout mapping, and recovery after transient authorization, rate-limit, and Collector faults (PRs #53, #54, #58; CI-verified).
Production rollout is tracked by the individual environment gates below and in the v1.0 readiness section. PR #56 supplies an optional Caddy TLS edge-ingress scaffold, not a live deployment. Production tenant ingestion remains disabled until the required gates pass. Reserved producer tenant attributes are rejected, never trusted.
- [x] Merge and CI-verify the shared PostgreSQL rate limiter for horizontally scaled API instances (`db45843`); staging/production require PostgreSQL and the limiter stores only hashed short-lived keys.
- [x] Add CI-verified 7-day retention and explicit ingestion/cardinality/size guardrails for the single-tenant local Prometheus, Loki, and Tempo stack; document the limits and their production boundary (`docs/observability-retention.md`).
- [x] Preserve the existing ephemeral API rate-limit retention: two-minute expiry with request-driven pruning and an indexed expiry column.
- [ ] (Deferred — production path, not portfolio v1) Define and implement a compliant retention/archive policy for durable API history (workspace audit, incident timelines, alert lifecycle, notification attempts, and deployment annotations); do not TTL-prune immutable history until its policy is approved.
- [x] Add a documented full-database PostgreSQL logical backup/restore runbook and CI-verified pg_dump/pg_restore round-trip covering the schema migration ledger, workspace audit data, actor context, and post-restore append-only guards (PR #83; `docs/postgres-backup-restore.md`).
- [x] Instrument inbound production Go API requests with bounded OTLP request/duration metrics, route-template traces, and static lifecycle/server-error logs; self-telemetry is optional and uses the private Collector (PR #133).
- [x] Instrument alert-evaluator and notification-delivery worker loops with bounded OTLP run/item counters, duration histograms, tick spans, and static error events; only allow-listed worker names and outcomes are recorded, never workspace, delivery, channel, request, or user identifiers (PR #133).
- [x] Add quota-bounded age-encrypted full-cluster backups to OCI Always Free Object Storage, instance-principal authentication, and a daily systemd timer. The script refuses uploads above 16 GB and never auto-deletes prior backups.
- [ ] (Deferred — production path, not portfolio v1) Create the private Always Free backup bucket, instance dynamic group/policy, and offline age identity; run one target-environment restore before accepting real tenant data. Full retention/PITR, backup alerting, and measured RPO/RTO remain production operational work.
- [x] Add deterministic in-process sustained-concurrency tests and repeatable allocation/throughput benchmarks for authenticated OTLP HTTP/gRPC gateway paths; see [M06 load-testing guide](m06-load-testing.md).
- [ ] (Post-v1) Run and record target-environment load tests for sustained ingestion and dashboard queries with real PostgreSQL and Collector dependencies; capture throughput, p50/p95/p99 latency, rejection rates, saturation, and recovery.
- [x] Merge the repository threat model and automated Gitleaks, govulncheck, npm audit, and Dependency Review workflows (PR #90; squash merge `4a4c46445d7f6b0129cba647d1bd71c49bbd0d45`; all 13 Security jobs passed on final PR head `d388b4702f1dca3b26f55a5fa8ef1e93328c98ee`, run `41406376150`).
- [x] Enable the repository Dependency Graph and obtain a successful PR Dependency Review result (PR #90 Security run `41406376150`; latest-head check passed with no denied or HIGH-or-higher dependency packages).
- [x] Merge the immutable full-commit-SHA GitHub Actions pin policy and scheduled Dependabot updates (PR #90; squash merge `4a4c464`; latest-head action-pin CI check passed in run `41406376148`).
- [x] Merge and verify the production image security gates (PR #90; squash merge `4a4c464`; all 13 Security jobs passed on final head in run `41406376150`, including API/PostgreSQL, static telemetry smoke/demo, patched Tempo with exact-PURL VEX, minimal Collector, Caddy, Redis, NATS, Prometheus, Loki, and Go/Alpine bases. No blanket CVE suppression was used. Remediation ownership/SLAs, signed provenance, and target-environment security testing remain separate open gates.
- [x] Generate and retain CycloneDX SBOM artifacts for the first-party images and Compose/base image inventory (PR #90; all 11 planned CycloneDX artifacts were generated and uploaded with 30-day retention in Security run `41406376150`). Signed build/release provenance and attestation remain a separate release gate.
- [x] Refresh and verify the Go telemetry-demo dependency lockfile and re-scan the image after fixing Trivy HIGH findings in `golang.org/x/net` (0.58.0 → 0.60.0) and `google.golang.org/grpc` (1.83.1 → 1.83.2); the latest dependency review and telemetry demo image scan passed in Security run `41406376150`.
- [x] Publish an operator runbook template covering production preflight gates, safe deployment and post-deploy verification, incident response, rollback boundaries, backup/restore coordination, and evidence capture (`docs/operations-runbook.md`). Executing its environment-specific gates remains open above.
- [ ] (Deferred — production path, not portfolio v1) Provision the Oracle Always Free VM and DNS, deploy API/PostgreSQL/Keycloak/telemetry, and connect GitHub Pages using [the archived free-only deployment guide](deployment/free-only-orbit.md). This is not required for the Apps Script portfolio demo.

## Free-only infrastructure implementation status (2026-11-04)

The repository now contains a zero-cost deployment scaffold: OCI Always Free
single-node topology, self-hosted PostgreSQL and Keycloak, local Collector /
Prometheus / Loki / Tempo, Caddy HTTPS ingress, DuckDNS dynamic DNS, generated
production secrets, and Pages build-time API/OIDC variables. These files do not
prove that a VM or DNS records have already been provisioned.

- [x] Add the production Compose overlay, separate Keycloak database, resource
  caps and API/Auth/OTLP reverse-proxy routes.
- [x] Add a script to generate protected credentials, update DuckDNS and start
  the free stack; ignore generated secrets.
- [x] Inject public API and OIDC configuration into the GitHub Pages build.
- [x] Document the Always-Free-only deployment and provider-side setup.
- [x] Document Always Free VM and DNS provisioning without any paid fallback.
- [x] Document stack startup, Keycloak realm/client configuration and Pages connection.
- [x] Document private OCI bucket/IAM setup, offline age identity and isolated restore.
  The real resource provisioning and restore are tracked once in the M06 gates above.

Manual accessibility and broad capacity testing are deferred until after v1.
Only the minimal health, authentication, authorization and tenant-isolation
smoke checks remain pre-release requirements.

## Free-only infrastructure status (2026-11-04)

- [x] Production Compose overlay and Caddy routes for API, OIDC and OTLP.
- [x] Self-hosted PostgreSQL/Keycloak and the private telemetry stack.
- [x] Free-stack setup, DuckDNS update and secret generation.
- [x] GitHub Pages build variables for API and browser OIDC.
- [x] Encrypted full-cluster backup script with OCI instance-principal access,
  a 16 GB hard bucket cap and daily systemd timer.
- [x] Provider-side setup instructions for the private bucket, dynamic group,
  restricted bucket policy, offline age identity and isolated restore.
- [x] Provisioning and live activation are consolidated into the single
  M06 free-only hosting task above; do not track VM, DNS and Pages connection
  as separate duplicate release blockers.

Human accessibility sessions and broad capacity testing remain deferred until
after v1 as requested. Do not enable customer ingestion before the essential
identity, health, and tenant-isolation smoke checks pass.

## Definition of done

- Type-check, unit tests, integration tests, and production builds pass.
- Empty, loading, stale, partial, and error states are covered.
- No secrets or real customer telemetry are committed.
- Mobile and desktop layouts remain usable.
- Documentation describes what is implemented, not only what is intended.


## ORBIT v1.0.0 stable release — published 2026-11-05

### Published release

- **Stable release:** [ORBIT v1.0.0 — Stable Portfolio Demo](https://github.com/pooya-labs/orbit/releases/tag/v1.0.0)
- **Public dashboard:** https://pooya-labs.github.io/orbit/
- **Source repository:** https://github.com/pooya-labs/orbit
- **Release tag:** `v1.0.0`, immutable, points to main release commit
  `d2197244b1c1316cbe4b362d15c838c8eaa4cb1a`.
- PR [#134](https://github.com/pooya-labs/orbit/pull/134) was squash-merged
  into `main`. The application/package/lockfile/footer, Go API default,
  Compose examples, and OpenAPI metadata are aligned to `1.0.0`.
- The stable Release workflow [42032666313](https://github.com/pooya-labs/orbit/actions/runs/42032666313)
  completed successfully, verified the exact deployed commit and version,
  created the immutable tag, and published a non-prerelease / non-draft
  GitHub Release.
- Exact-main GitHub Pages deployment
  [42032403198](https://github.com/pooya-labs/orbit/actions/runs/42032403198)
  succeeded for commit `d2197244b1c1316cbe4b362d15c838c8eaa4cb1a`.
- Exact-main CI
  [42032402892](https://github.com/pooya-labs/orbit/actions/runs/42032402892)
  passed all four jobs: web typecheck/build, HTML and browser/axe tests,
  Apps Script runtime contract, Go test/vet/build and PostgreSQL backup/
  restore, action pin policy, complete OTLP Metrics/Logs/Traces pipeline,
  Prometheus scrape, authenticated Workspace A/B write-to-query isolation,
  PostgreSQL restart/connection-pool recovery, and cleanup.
- Exact-main Security
  [42032403220](https://github.com/pooya-labs/orbit/actions/runs/42032403220)
  passed all **13/13** jobs, including API, Collector, Tempo, Caddy,
  Prometheus, Loki, Redis, NATS, dependency and full-history secret scans.
- Windows NVDA Browse Mode navigation passed on the release-candidate source
  at [42024149075](https://github.com/pooya-labs/orbit/actions/runs/42024149075).
  The same exact PR source had already passed all four CI jobs and all 13
  Security jobs before merge.

### Delivered scope — complete for v1.0.0

- **M01 UI/accessibility automation:** responsive dashboard, bilingual
  English/Persian RTL, light/dark themes, keyboard/landmark contracts, chart
  table alternatives, generated-HTML checks, 22 Playwright/axe browser tests,
  76 accessibility source checks, and Windows NVDA Browse Mode navigation.
- **M02 API/data foundation:** Go API, PostgreSQL migrations, typed client,
  OpenAPI 3.1, inventory and operational APIs, audit/history persistence and
  repository integration tests.
- **M03 local platform:** Compose, PostgreSQL, optional
  Collector/Prometheus/Loki/Tempo pipeline, instrumented Go demo and
  end-to-end signal checks. Redis and NATS are provisioned only; functional
  application integration remains deferred under ADR 0004.
- **M04 telemetry baseline:** bounded Metrics/Logs/Traces queries,
  cross-backend service discovery, persisted workspace catalog, source/resource
  provenance, pagination, request-rate grouping, staleness/gap handling,
  authenticated OTLP capture, canonical workspace attribution, workspace
  isolation, trace detail and validated Logs-to-Trace correlation.
- **M05 operations:** workspace-scoped incident timeline, alert
  rules/evaluation, webhook outbox and retry history, deployment annotations,
  SLO/error budget, and searchable immutable audit history.
- **M06 code-level security/recovery:** human OIDC verifier/role gate,
  provider-neutral browser PKCE, workspace membership, workload
  identities/grants, authenticated OTLP gateways, rate limits/admission/
  revocation/fail-closed behavior, A/B tenant integration tests, local
  retention/cardinality guardrails, backup/restore CI, pinned Actions, SBOM
  artifacts, fault/recovery tests and bounded API/worker self-instrumentation.
- **Apps Script demo runtime:** V8 Web App, private owner-owned Spreadsheet,
  safe editor initializer `initializeOrbitDemo`, pseudonymous
  browser-profile workspaces, six-hour session capabilities, request/body/
  storage quotas, fixed-origin iframe bridge and synthetic adapter.
- **Browser integration:** Playwright loads the committed HtmlService wrapper
  around the static dashboard across separate origins and verifies bootstrap,
  metadata, session-token forwarding, a write round-trip and absence of
  hydration/runtime errors. The bridge test uses a mocked
  `google.script.run`; see owner-side verification limitation below.

### Release scope and limitations

- v1.0.0 is a stable **zero-cost portfolio demo**, not a production
  observability service or customer-ingestion platform.
- GitHub Pages serves the static frontend only. It does not host the Go API,
  PostgreSQL, Prometheus, Loki or Tempo data plane.
- The Apps Script owner supplied the `/exec` deployment URL, but the
  verification environment could not fetch `script.google.com`. Therefore
  live Google initialization, `doGet`, bootstrap, a live mutation and
  refresh persistence are not claimed as independently verified. This is
  disclosed in the release notes. Do not recreate or redeploy the supplied
  Apps Script project without a concrete runtime error; PR #134 did not
  change Apps Script runtime files or its manifest.
- Apps Script data and notifications are synthetic only. The backing
  Spreadsheet remains private; no customer data, production secrets, real
  telemetry, outbound webhooks, email or messaging calls are allowed.
- The deferred production Keycloak identity image is scanned independently
  by `.github/workflows/security-deferred-identity.yml`. Historical run
  [42032403489](https://github.com/pooya-labs/orbit/actions/runs/42032403489)
  reported HIGH OpenJDK/SQLite/Guava findings on the superseded
  `quay.io/keycloak/keycloak:26.8.1` image. The main-branch production Compose file remains on Keycloak 26.8.1.
  PR #138 now runs the deferred scan against the patched 26.8.2 GHCR artifact
  pinned by immutable digest, with native AMD64/ARM64 validation. The first exact-image scan
  [42225793814](https://github.com/pooya-labs/orbit/actions/runs/42225793814)
  failed Trivy with three HIGH findings. The completed package inventory in
  [run 42227758708](https://github.com/pooya-labs/orbit/actions/runs/42227758708)
  confirms the CA package's full RPM version is
  `2026.2.92_v9.0.319-90.3.el9_8`, the RHEL 9 update in
  [RHSA-2026:88459](https://access.redhat.com/errata/RHSA-2026:88459).
  Red Hat explains that RHEL OpenJDK uses the system CA bundle; the original
  CVE-2026-91403 was an insufficient upstream mitigation, while root removal
  is tracked under CVE-2026-114406 / CVE-2026-114820
  ([CVE-2026-91403](https://access.redhat.com/security/cve/cve-2026-91403),
  [CVE-2026-114406](https://access.redhat.com/security/cve/cve-2026-114406)).
  The Java trust-store inventory confirmed the compromised Comodo AAA Services
  and emSign ECC Root CA G3 aliases are absent. A narrowly scoped OpenVEX now
  addresses only the exact RHEL OpenJDK package in this image.
  The image contains `sqlite-libs 3.34.1-11.el9_8` but no `sqlite3` CLI.
  CVE-2026-119586 is specifically in the CLI `.recover` implementation; a
  package-version-specific OpenVEX covers only that false match.
  SQLite FTS5 is compiled into the library (confirmed by
  `sqlite3_compileoption_used("ENABLE_FTS5")`), so the FTS5 code is present.
  A separate exact-image, exact-package OpenVEX now covers CVE-2026-119373
  with justification `vulnerable_code_not_in_execute_path`, only for the
  x86_64 and aarch64 `sqlite-libs 3.34.1-11.el9_8` PURLs in Keycloak 26.8.2.
  Evidence:
  - The exact downstream Red Hat source RPM
    `nss-3.124.0-5.el9_6.src.rpm` was downloaded from the official UBI 9
    AppStream source repository (SHA-256 recorded in the CI audit artifact),
    and its declared patches were applied with `rpmbuild -bp`.
  - The prepared NSS `lib/softoken` code has no FTS5 references, no
    `CREATE VIRTUAL TABLE`, no `MATCH` query and no SQLite extension-loading
    API (`FTS_SOURCE_REFERENCES_FOUND=0`,
    `SQLITE_DYNAMIC_EXTENSION_LOADING_FOUND=0`). Its SQLite call sites use
    ordinary tables, temporary tables, regular indexes and parameterized
    CRUD queries.
  - Runtime inventory found no SQLite CLI or Keycloak SQLite JDBC/application
    library. The only inspected native object linked to `libsqlite3` is
    `libsoftokn3.so`, the NSS Softoken consumer audited above.
  Red Hat says exploitation requires an application exposing FTS5 virtual
  full-text indexing with suppressed detail storage, negative row values and
  targeted prefix MATCH expressions. This VEX is limited to the exact package
  PURLs and Keycloak 26.8.2 image; it is not a general SQLite exemption.
  See [Red Hat's CVE statement](https://access.redhat.com/security/cve/cve-2026-119373).
  The strict scan [run 42525007580](https://github.com/pooya-labs/orbit/actions/runs/42525007580)
  now filters the narrowly covered SQLite findings and reports only the
  upstream Guava 33.6.0-jre HIGH finding on both amd64 and arm64.
  The patched candidate has now been published to GHCR as an immutable
  multi-architecture manifest. Its exact digest passed strict raw-versus-VEX
  validation on native AMD64 and ARM64; the deferred identity workflow and an
  independent source rebuild also passed on the PR branch. Keep production
  Compose on upstream 26.8.1: publishing and validating the artifact does not
  constitute production promotion or identity provisioning.
  The two VEX records are narrow, package-specific and evidence-backed; no
  blanket ignore or image-wide exception was added. The production identity
  gate remains blocked for production deployment until the real origin, identity,
  DNS/ingress and operational gates are approved. Keycloak is not used by the Pages + Apps Script
  demo and remains outside the v1 release gate.
- The release does not claim formal WCAG conformance. Human TalkBack/VoiceOver
  device acceptance and broad sustained-load/capacity benchmarking remain
  post-v1 work.

### Release status

**v1.0.0 is published and the repository release gate is closed for the
portfolio-demo scope.** Do not repeat PR #134 or the completed M01–M06
repository checks unless a new regression or issue is identified.

### Deferred until after v1



- Broad sustained-load/capacity benchmarking and longer operational rehearsals.
- Full backup automation and formal RPO/RTO drills beyond the encrypted backup
  and one restore check required before accepting real tenant traffic.

### Explicitly deferred — not v1 blockers unless measured evidence changes the decision

- Redis cache and NATS application/event integration: provisioned in Compose but deferred under [ADR 0004](decisions/0004-redis-nats-deferred.md) until profiling or a second durable consumer demonstrates need.
- Expanding the OpenTelemetry resource attribute allowlist: current review and ingress/persistence drift guard are complete (PR #126 / ADR 0005). New attributes require an actual consumer, stability/privacy review, cardinality analysis, and an operational budget.
- Arbitrary PromQL/LogQL access: intentionally unsupported; v1 metric selection must remain allow-listed and cost-bounded.

### Audit notes and documentation consistency

- Canonical repository and Pages origin are \`pooya-labs/orbit\` and \`https://pooya-labs.github.io/orbit/\`. Personal creator attribution may remain linked to the maintainer's separate GitHub profile. Runtime OIDC callback/CORS examples and operator/user links must use the canonical Pages origin.
- PostgreSQL migration history currently runs from \`001_initial.sql\` through \`016_workspace_otlp_resource_attributes.sql\`; docs must not describe only the first three migrations as the current schema.
- The historical Go module namespace still uses \`github.com/pooya16pg-spec/orbit\`. Do not mechanically rename imports in this audit; decide separately whether a canonical module path migration is required before publishing reusable Go modules.
- The current license notice says no open-source license has been selected. This is not a blocker for a private/proprietary v1 deployment; choose and add a license before claiming/distributing the repository as open source.


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
- [x] Verify recovery after real local Collector listener stop/restart (PR #62), PostgreSQL TCP-path disconnect/restoration (PR #64), clean PostgreSQL Compose container stop/start (PR #111), and ungraceful PostgreSQL SIGKILL + WAL recovery through the same pool with a committed-data probe (PR #112; CI run `41596787239`). These are single-node process/container recovery tests, not host power-loss, storage-loss, HA failover, PITR, or production RPO/RTO certification.
Target-environment sustained load and recovery evidence is tracked once in the M06 checklist above; do not count this historical PR #50 note as a second task.
- [x] Add explicit backpressure and recovery-under-sustained-load tests across the real dependency boundary (PR #110, squash merge `b42b4b56a82a2f977b373731ae6e85a1ceb0760e`; CI run `41580189083` passed using real CI PostgreSQL + OTLP Collector: 64 admitted, 8 over-capacity HTTP 503, three 16-export recovery waves, and 639 successful exports over 20 seconds).
- [x] Add an optional Caddy TLS edge-ingress Compose overlay for OTLP/HTTP and OTLP/gRPC (PR #56; merged with CI passing). CI validates the Caddyfile, the rendered Compose topology, and that only the edge proxy publishes ingress ports.
DNS/ACME, firewall, trusted-proxy, private-network policy, and internal-hop mTLS assessment are tracked once in the M06 production rollout checklist above.
Real-provider browser PKCE and token/key lifecycle verification are tracked once in the M06 production rollout checklist above.
- [x] Complete repository dependency/secrets review, local telemetry retention/cardinality controls, CI backup/restore verification, and publish deployment/incident-response runbook templates (latest review: Security run `41406376150`; see `docs/observability-retention.md`, `docs/postgres-backup-restore.md`, and `docs/operations-runbook.md`). Production-specific execution and durable-history retention policy remain open below.

The in-flight contract does not cancel or roll back data already admitted to the Collector. Production tenant ingestion remains disabled until the remaining production gates are verified.


## M06 PR #51 — gateway admission capacity

PR #51 is merged into `main` (squash merge commit `1ff6a8baf80e50910f4c0b41ebaec73b4fdd26ba`). CI run `41353306270` completed successfully across Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

The PostgreSQL-backed integration tests exercise both OTLP/HTTP and OTLP/gRPC with a gateway admission capacity of one. The first export is held inside a blocking Collector forwarder; a second export is rejected with HTTP 503 or gRPC ResourceExhausted and does not reach the forwarder. Once the first export is released, it completes successfully. This verifies the admission-capacity boundary, not a full production load or outage envelope.

Remaining M06 work is environment- and policy-dependent: sustained target-environment load and backpressure evidence; encrypted off-host backups/PITR and measured restore rehearsal; real DNS/ACME, firewall, trusted-proxy and deployment network controls; real-provider browser PKCE; and an approved durable-history retention/archive policy. Repository dependency/secrets checks, local retention/cardinality guardrails, backup/restore CI, and runbook templates are complete. Production tenant ingestion remains disabled.


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

These are deterministic in-process fault-injection tests. PR #62 separately adds real local TCP listener stop/restart coverage for both production Collector forwarders. PR #111 verifies clean PostgreSQL Compose stop/start; PR #112 verifies SIGKILL-triggered PostgreSQL startup/WAL recovery and committed-data survival through the same pool. Host power loss, storage corruption/loss, packet loss or multi-zone network partition, HA failover, and production recovery behavior remain open. Production tenant ingestion remains disabled.


## M06 PR #60 — default gateway burst-capacity boundary

PR #60 is merged into `main` (squash merge commit `cb85c0afea6647ecc59b9fe59e499ce8383fc526`). CI run `41355534531` passed all three jobs. New synchronized tests hold all 64 default HTTP or gRPC admission slots inside blocking Collector forwarders, then verify additional HTTP requests receive 503 and additional gRPC requests receive ResourceExhausted without reaching the forwarder. After capacity is released, all admitted requests complete and the active-forward count returns to zero.

This verifies the configured in-process admission boundary under a deterministic burst. It is not a throughput benchmark or sustained production load test; broader load, network/process fault injection, and backpressure remain open.


## M06 PR #62 — real Collector listener restart recovery

PR #62 is merged into `main` (squash merge commit `534f6d02c04616a5d27831460b24fd87db8bfade`). CI run `41357122909` passed all three jobs: Go API tests/build (including race tests), web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

The new HTTP test starts a real local TCP OTLP/HTTP endpoint behind the production Collector HTTP forwarder, verifies success, stops the listener and verifies the gateway returns 502, restarts a listener on the same address, and verifies the same long-lived gateway/forwarder succeeds again. The gRPC test performs the equivalent listener stop/restart using a real gRPC Collector server and the production gRPC forwarder; the same client connection eventually reconnects and succeeds.

This validates process-local Collector TCP listener recovery and client reconnection behavior. It does not simulate packet loss or network partitions, cross-zone failures, or recovery under sustained load. PR #111 and PR #112 separately cover PostgreSQL clean restart and SIGKILL/WAL recovery; longer-duration target-environment load/backpressure and production deployment verification remain open. Production tenant ingestion remains disabled.


## M06 PR #64 — PostgreSQL pool recovery after network interruption

PR #64 is merged into `main` (squash merge commit `6b2e3418be2e8fdff0c9634b361acaa5bb6d6da1`). CI run `41357596656` passed all three jobs, including Go race tests/build, web typecheck/production build, and the telemetry end-to-end smoke test.

The new PostgreSQL integration test opens the real repository through a local TCP proxy, warms an established pgxpool connection, then closes active TCP tunnels and rejects new dials while the test network path is offline. The existing repository must report a Ping failure. The proxy is then restored and the same pgxpool/repository must reconnect and return to healthy Ping without being recreated.

This proves connection-pool recovery after a real TCP path interruption against the CI PostgreSQL service; the database process remains running during this specific test. PR #111 separately proves clean PostgreSQL container/process stop-start; PR #112 proves SIGKILL-triggered server restart/WAL recovery and committed probe-row survival through the same pgxpool. Host power-loss, disk failure/corruption, packet-loss/partition, HA failover, prolonged outage, and production RPO/RTO evidence remain open.


## M05 PR #66 — workspace incident lifecycle backend

PR #66 is merged into `main` (squash merge commit `7a57dd4ef8752e8e18d6dec5f0e75112637167dd`). CI run `41360316146` passed all three jobs: Go API race tests/build and vet, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

Migration 009 adds a workspace-scoped incident timeline with a composite foreign key back to the incident/workspace pair. Database triggers reject UPDATE, DELETE, and TRUNCATE. The PostgreSQL Repository provides transactional create and status-transition commands. Every mutation writes its timeline event and workspace audit event in the same transaction. Owners, administrators, and operators may create/change incidents; any workspace member may read the timeline.

The supported state transitions are: open → investigating/monitoring/resolved; investigating → monitoring/resolved; monitoring → investigating/resolved; resolved → investigating (reopen). Resolving sets `resolved_at`; reopening clears it. The authenticated API exposes POST workspace incidents, PATCH incident status, and GET incident timeline routes. The OpenAPI contract describes these endpoints and request/response schemas.

This is the backend API and persistence slice only. Dashboard incident controls/detail/timeline UI, alert rules/notifications, deployment annotations, SLO/error-budget calculations, and wider audit workflows remain open. M05 is not complete.


## M05 PR #69 — workspace alert rule definitions

PR #69 is merged into `main` (squash merge commit `64c4c4f1d36fd0c17ae444ac5736fadad0a32655`). CI run `41361579435` passed all three jobs: Go API race tests/build and vet, web typecheck/production build, and the telemetry end-to-end smoke test.

Migration 010 adds workspace-owned alert rule definitions with a case-insensitive unique name per workspace and database constraints for the supported metric, operators, threshold, evaluation window, pending duration, severity, and description bounds. The API supports list, create, replace, enable/disable, and delete. Owner/administrator/operator roles may mutate; workspace members may list. Every mutation and its append-only workspace audit event are committed in one transaction.

Version 1 deliberately supports only `request_rate`, the metric currently emitted by the instrumented demo. It accepts fixed comparison operators and bounded settings, not arbitrary PromQL/LogQL. PR #69 delivered the workspace-scoped definitions; PR #72 adds scheduled evaluation, durable lifecycle instances, deduplication, and multi-replica coordination. Notification delivery/retry remains open.


## M05 PR #77 — immutable deployment annotations

Migration 013 adds workspace-scoped `deployment_annotations` linked to the owning service and workspace. The API derives service name and environment from that service, validates release/version metadata and deployment time, and writes the annotation plus `deployment.annotation_created` audit event in one transaction. Database triggers prevent UPDATE, DELETE, and TRUNCATE.

Workspace members can list or read deployment annotations. Release comparison accepts two different annotations for the same service and environment in chronological order and reports version/commit changes plus elapsed time. It does not claim that a release caused a telemetry change. The operator dashboard follows in the next UI slice.


## M05 SLO and error-budget semantics

SLO definitions are per workspace service and use one bounded HTTP request-success SLI. Only instrumented low-cardinality routes contribute events: response codes 200–399 are good, other instrumented responses are bad; health checks and uninstrumented routes are excluded. Definitions allow targets from 90% to 99.99% and rolling windows of 1, 3, or 7 days, matching the Prometheus seven-day retention. PromQL is server-owned and filters by the authenticated workspace, service, route, and outcome.

For observed good and bad events, availability is `good / (good + bad)`, allowed bad events are `total × (1 − target)`, remaining budget is `max(0, allowed − bad)`, and burn rate is `bad / allowed`. No observed events return `insufficient_data`; absence of telemetry is never reported as 100% availability. The local Compose demo is unscoped and cannot populate workspace SLOs until telemetry is sent through authenticated workspace ingestion.


M01 baseline merge record (2026-11-03): PR #109 was squash-merged into `main` as `e9e7d74387dddcf76226cfab32dd979f56af4dc9`. The automation and implementation checkboxes above describe the merged baseline. The separate manual keyboard/screen-reader and rendered-state review checkbox intentionally remains unchecked; it is a gate for declaring the full accessibility review complete, not a claim that automation was skipped.


## M06 PR #110 — real-dependency backpressure and sustained recovery

PR #110 was squash-merged into `main` as `b42b4b56a82a2f977b373731ae6e85a1ceb0760e`. Exact PR head `e00b0308bf0bf3d61e095975ee69546e349d0c43` passed [CI run 41580189083](https://github.com/pooya16pg-spec/orbit/actions/runs/41580189083) and [Security run 41580189031](https://github.com/pooya16pg-spec/orbit/actions/runs/41580189031).

The Docker Compose workspace smoke runs the new `TestWorkspaceOTLPSustainedBackpressureAndRecovery` beside the existing real write-to-query workspace isolation test. It uses the real PostgreSQL-backed workload authorization and shared rate-limit repository plus the real OTLP Collector. A deterministic in-process HTTP gate holds the 64 HTTP gateway admission slots; eight additional requests must receive HTTP 503 and must not be forwarded. After release, all 64 admitted requests succeed. Three more concurrent recovery waves of 16 exports each succeed. The test then sustains eight producer loops over 20 seconds and requires at least 100 successful exports.

Observed CI result: **639 successful sustained exports over 20 seconds**. The new test passed in 20.10 seconds; both workspace tests completed in 66.18 seconds. The full CI run passed all 4 jobs; Security passed all 13 jobs, including dependency/secret scans, first-party and Compose image scans, and SBOM generation.

This closes the deterministic repository-level real-dependency regression only. The separate target-environment test at line 160 remains open for intended deployment hardware/topology: sustained ingestion and dashboard-query throughput, p50/p95/p99, rejection rates, queue/pool saturation, multiple replicas, and measured recovery. Production tenant ingestion remains disabled until all environment-specific M06 gates are evidenced.


## M06 PR #111 — PostgreSQL container restart and same-pool recovery

PR #111 was squash-merged into `main` as `5b3c0fc344f043640b01c670a84deb6c78d09a5c`. Exact PR head `ac70f81103ced4e9c5ef5e4968f35ccd3115cce1` passed [CI run 41581322810](https://github.com/pooya16pg-spec/orbit/actions/runs/41581322810) and [Security run 41581322826](https://github.com/pooya16pg-spec/orbit/actions/runs/41581322826).

The Compose test starts one Repository/pgxpool, warms a real PostgreSQL connection, stops the actual PostgreSQL container, waits until the original pool observes a failed Ping, and starts the same service again. It verifies a successful query through the original pool and checks that `pg_postmaster_start_time()` advanced, proving the server process restarted. Observed timestamps: `2026-11-03T05:07:11.802744Z` before and `2026-11-03T05:10:29.836093Z` after restart. The test passed in 1.094 seconds; the observability E2E job and all four CI jobs passed. Security passed all 13 jobs.

This closes the repository-level clean-stop PostgreSQL container/process restart regression. It does not establish crash/power-loss recovery, disk failure recovery, HA failover/replication, network partition behavior, PITR, or production RPO/RTO. The broader production operational gates remain open.


## M06 PR #112 — PostgreSQL crash/WAL recovery and committed-data durability

PR #112 was squash-merged into `main` as `15fc476f422afbc632542e292d1c17d9a0af2796`. Exact PR head `7fb08afb6b312d3c3878fb4011021e39d5b14f51` passed [CI run 41596787239](https://github.com/pooya16pg-spec/orbit/actions/runs/41596787239) and [Security run 41596787190](https://github.com/pooya16pg-spec/orbit/actions/runs/41596787190).

The same Repository/pgxpool instance completed two sequential failure phases:
1. Graceful PostgreSQL Compose stop/start: postmaster time `2026-11-03T07:35:45.535327Z` → `2026-11-03T07:38:44.698761Z`.
2. Ungraceful PostgreSQL container SIGKILL followed by start with the same persistent volume: `2026-11-03T07:38:44.698761Z` → `2026-11-03T07:38:45.596894Z`.

The test required the existing pool to observe both outages, reconnect, and query after each server process restart. It also committed a unique database probe row before the two phases and read the unchanged payload back after both restarts, validating committed-data survival through PostgreSQL startup/WAL recovery. Both subtests passed; total test time was **2.768 seconds**. Full CI passed 4/4 jobs, and Security passed 13/13 jobs.

This closes the repository-level clean-stop and SIGKILL crash-recovery regression for a single PostgreSQL container with a persistent Docker volume. It does not simulate host power loss, corrupted/lost storage, disk/controller failure, replication failover, network partitions, PITR, or production RPO/RTO. Those remain distinct deployment and operational gates.


## M01 PR #113 — stable contrast during navigation transitions

PR #113 was squash-merged into `main` as `661ee0b7ea3faaa39197d03579e3775a5cdefd3b`. It fixes the real axe failure observed on merge commit `15fc476f422afbc632542e292d1c17d9a0af2796` in CI run `41597808873`: axe sampled two blended sidebar text colors during route/theme transitions (4.42:1 and 4.09:1), below the required 4.5:1. The stable light-theme colors were already above threshold; the failure was caused by interpolating text color during the transition.

The nav label color now changes instantly while only its background may animate. The axe matrix waits for finite sidebar transitions to finish after a route/theme change. The static contrast guard prevents reintroducing a text-color or `all` transition and checks the settled foregrounds on both endpoint backgrounds:
- Normal nav `#5b6c76`: 5.45:1 on white and 4.82:1 on the active pale background.
- Active nav `#127868`: 5.37:1 on white and 4.74:1 on its pale background.

Exact PR head `8af075d01fd3178857e6aafe867d5f091a3525e6` passed [CI run 41599111532](https://github.com/pooya16pg-spec/orbit/actions/runs/41599111532) (**4/4 jobs, 14/14 Playwright tests**) and [Security run 41599111683](https://github.com/pooya16pg-spec/orbit/actions/runs/41599111683) (**13/13 jobs**). The same CI verified the full Docker telemetry pipeline, workspace isolation, and PostgreSQL restart regression. Manual screen-reader and rendered-state review remains open.


### PR #120 — release-version display consistency (2026-11-03)

PR #120 was squash-merged into main as 0348f8831d8f008970ad6c42532620a12ff5a0e0. The dashboard footer now displays the existing application release version v0.10.0; a browser regression reads the version from package.json and asserts the footer matches it. Exact PR head 23625b2916457947b44611872508d469ca064130 passed all four CI jobs, including 18/18 Playwright tests and the full Docker telemetry/workspace-isolation/PostgreSQL-recovery job, plus all 13/13 Security jobs. After merge, Pages deployment [41612768102](https://github.com/pooya16pg-spec/orbit/actions/runs/41612768102), release workflow [41612863226](https://github.com/pooya16pg-spec/orbit/actions/runs/41612863226), and post-merge Security [41612767938](https://github.com/pooya16pg-spec/orbit/actions/runs/41612767938) succeeded. See project-context.md for the exact remaining gates and final post-merge CI status.

At this historical checkpoint the human M01 gate was still open. The owner later confirmed manual keyboard/device/assistive-technology acceptance on 2026-11-10; see the current-scope closeout at the end of this roadmap.


## Post-v1 CF01 — Cloudflare edge gateway (2026-11-06)

This is a post-v1 entry-point extension, not a reopening of the stable Apps Script v1 demo and not a replacement for the existing Go/PostgreSQL/OTel architecture.

- [x] Inspect the accepted Go API, PostgreSQL migrations, OIDC/PKCE model, workload identity/grant model, private Collector topology, Prometheus/Loki/Tempo stores, Caddy ingress, retention, backup and free-only deployment runbooks.
- [x] Record the architectural correction in ADR 0006: Cloudflare Worker is a narrow API edge; Go remains the only auth/role/business-logic authority; PostgreSQL and the existing telemetry stores remain canonical.
- [x] Implement Worker proxy contract with exact Pages-origin CORS, explicit route allowlist, Bearer-token forwarding, request IDs, trusted Cloudflare client IP, streaming request/response bodies, no response cache and fail-closed origin configuration.
- [x] Add Worker unit tests for liveness, CORS, origin validation, path allowlisting, authorization forwarding, header replacement, streamed mutations and upstream cookie handling.
- [x] Protect the existing Caddy API origin with a random gateway secret; reject bypass requests, strip the secret before Go and forward the trusted edge IP.
- [x] Generate the private gateway secret in the existing mode-600 `.env.production` setup; add a Wrangler deployment script that uses a temporary protected secrets file and never prints the secret.
- [x] Add Worker contract tests and forwarded-client-IP tests to CI; validate the configuration/deployment shell script with `bash -n`.
- [x] Preserve exact-origin CORS headers on JSON error responses and align the Worker config with Free-plan defaults (no Standard-only custom CPU override).
- [x] Require an explicit Workers Free confirmation before the Worker deployment script can publish, and add a full Cloudflare Free deployment runbook.
- [x] Replace the deployed prototype Worker with the canonical edge-gateway module; Cloudflare settings confirm `bindings: []` and Worker version 0.2.3 is the active upload.
- [x] Delete the prototype D1 database after verifying the Worker had no D1 binding; the deleted database ID returns HTTP 404. Other account D1 resources were left untouched.
- [x] Confirm the account Billing → Subscriptions page displays Workers Free — Active (owner-provided dashboard evidence); leave the per-Worker API `usage_model: standard` field unchanged and do not use legacy `bundled`.
- [x] Publish a patched Keycloak 26.8.2 multi-architecture image and pass strict HIGH/CRITICAL scans against its exact immutable digest on AMD64 and ARM64; keep findings visible and do not blanket-suppress them. Artifact validation is complete; production promotion remains separate.
- [x] Align the Cloudflare Free deployment acceptance and stop conditions with the account-level Billing evidence; do not mistake the per-Worker `usage_model: standard` metadata default for a Paid subscription.
- [ ] Provision and verify the original Go/PostgreSQL/Keycloak/Caddy origin. No origin VM, domain or identity-provider configuration is claimed as provisioned by this handoff.
- [ ] Run the Worker deployment script against the provisioned origin; verify Caddy rejects direct API bypass without the Worker secret.
- [ ] Build Pages using the Worker URL and the exact same Keycloak PKCE issuer/client/audience; test every existing typed API-client route through the Worker.
- [ ] Run live viewer/operator/administrator and workspace A/B tests through the Worker, then test OTLP/HTTP protobuf and OTLP/gRPC through the existing dedicated ingress hostnames.
- [ ] Complete clean-browser dual-runtime acceptance; Apps Script remains synthetic-only and unchanged.

Do not introduce D1/Queues/Redis/NATS, duplicate JWT verification, raw telemetry storage in the Worker, or paid Cloudflare services to satisfy this milestone. Existing CI for Go/PostgreSQL, security, backup/restore, telemetry end-to-end and workspace isolation remains authoritative.


## Post-v1 CF02 — Cloudflare-native D1 runtime (merged foundation, PR #140, updated 2026-11-08)

This is a separate Workers + D1 profile. It does **not** replace CF01's `orbit-runtime` edge gateway, the Go/PostgreSQL engineering core, GitHub Pages, or the Apps Script demo. PR #140 merged the staged foundation; the profile has not been deployed.

- [x] Add the isolated `cloudflare-native` Worker and initial SQLite/D1 schema for workspace membership, service/incident data, immutable audit/timeline records, alert rules, notification metadata, deployments and SLO definitions.
- [x] Add OIDC discovery/JWKS verification for RS256/ES256 access tokens, exact issuer/audience, standard claim validation and D1-backed workspace-role checks.
- [x] Implement the first workspace, service, incident, alert-rule, deployment, SLO-definition and notification-metadata routes with tenant scoping and audit records.
- [x] Map the 42 current typed API-client methods to native routes or explicit unsupported responses; unavailable telemetry/evaluation/budget/delivery functions return honest 503 responses.
- [x] Add SQLite/D1 schema and tenant-isolation smoke coverage plus signed-token, role-policy and bounded OIDC-document tests.
- [x] Add two authenticated Workers Rate Limiting bindings: 120 reads/60s and 30 writes/60s per pseudonymous OIDC subject; checks run before D1 and fail closed when a binding is missing or unavailable.
- [x] Add tests for read/write limiter selection, 429 responses, pseudonymous keys, missing-binding fail-closed and provider-failure behavior; the browser can read `Retry-After` through CORS.
- [ ] Verify the Wrangler configuration against the deployment account and confirm the two rate-limit namespace IDs are unused before any first deployment.
- [ ] Implement a trustworthy native telemetry source and a dedicated UI that distinguishes runtime telemetry from synthetic samples.
- [ ] Implement alert evaluation and bounded scheduled execution within Workers Free limits.
- [ ] Implement SLO error-budget calculation using a trustworthy request-metrics source.
- [ ] Implement delivery dispatch, bounded retries and immutable notification-attempt history.
- [ ] Configure and test a real compatible OIDC tenant; no Auth0 tenant or issuer has been created.
- [ ] Exercise every frontend API-client method against the native profile and complete role, workspace-isolation, browser and accessibility acceptance.
- [ ] Create the dedicated D1 database, apply remote migrations, configure secrets/variables, deploy the Worker, and only then consider a guarded Pages API-origin switch.

**Rate-limit boundary:** Workers Rate Limiting is approximate, per Cloudflare location and eventually consistent. It is an abuse/burst guard, not a strict cross-region quota and not a substitute for Workers Free daily requests or D1 row-read/write limits. Strict global limits would need a separately reviewed Durable Object design.

**Verification:** exact PR-head CI [42401556604](https://github.com/pooya-labs/orbit/actions/runs/42401556604) passed 4/4 jobs; Security [42401556495](https://github.com/pooya-labs/orbit/actions/runs/42401556495) passed 13/13 jobs. This includes 23 Worker tests, D1/Wrangler config smoke, the complete OTLP pipeline, Prometheus scrape, workspace isolation, PostgreSQL recovery, browser/a11y tests, and all image/security scans.

**Release boundary:** PR #140 merged a staged foundation, not feature parity and not a deployable v1. No D1 database, OIDC tenant, Worker route or Pages API switch has been provisioned. Keep Apps Script unchanged and preserve CF01 as the separate canonical-Go edge profile.

## Post-v1 Security 01 — Go x/net advisory response (2026-11-07)

- [x] Record CVE-2026-78661 / GO-2026-6860 and CVE-2026-97045 / GO-2026-6866 in [issue #141](https://github.com/pooya-labs/orbit/issues/141).
- [x] Raise the ORBIT API and telemetry-demo module floors and lockfiles to `golang.org/x/net v0.62.0`; align `golang.org/x/text v0.43.0`.
- [x] Patch the custom Caddy and Tempo source builds to the fixed module floor.
- [x] Upgrade the custom OpenTelemetry Collector distribution to v0.165.0 and revalidate its resolved modules.
- [x] Replace the vulnerable prebuilt Prometheus/Loki images with pinned upstream source builds (Prometheus v3.16.0; Loki v3.7.9) that embed and verify `x/net v0.62.0`.
- [x] Refresh the Security workflow dependency floors and lockfiles while retaining Trivy HIGH/CRITICAL gates, non-zero exit behavior and `ignore-unfixed: false`.
- [x] Rebuild and scan all affected images; pass Go tests, backup/restore, telemetry end-to-end, workspace isolation, Prometheus scrape, PostgreSQL recovery and full Security CI before merge.

**Result:** PR [#142](https://github.com/pooya-labs/orbit/pull/142) was squash-merged as `1caed65d11bbf9b5d419e4da5429115c98ebe3ac`. Exact PR-head CI [42396679559](https://github.com/pooya-labs/orbit/actions/runs/42396679559) passed 4/4 jobs, including the complete local telemetry stack; Security [42396679567](https://github.com/pooya-labs/orbit/actions/runs/42396679567) passed 13/13 jobs. All 11 CycloneDX SBOM workflow artifacts were uploaded and remained unexpired at verification. Issue #141 is closed. No HIGH/CRITICAL scanner threshold or VEX/ignore policy was weakened.


## Post-v1 Security — Keycloak 26.8.2 FTS5 and Guava follow-up (2026-11-09)

- [x] Fetch the exact Red Hat NSS 3.124.0-5.el9_6 SRPM, verify SHA-256 and official Red Hat RPM signature, apply the downstream patches, and audit prepared `lib/softoken`.
- [x] Confirm patched NSS Softoken has no FTS5, `CREATE VIRTUAL TABLE`, `MATCH`, or SQLite extension-loading API references.
- [x] Run published Keycloak 26.8.2 on native amd64 and arm64 runners; both readiness and OIDC discovery checks passed, and neither architecture mapped `libsqlite3.so` or `libsoftokn3.so` in the running container.
- [x] Strengthen the exact-package CVE-2026-119373 OpenVEX with signed downstream-source and dual-architecture runtime evidence. Scope remains the exact `sqlite-libs 3.34.1-11.el9_8` x86_64/aarch64 PURLs in Keycloak 26.8.2; it does not claim FTS5 code is absent.
- [x] Preserve the strict HIGH/CRITICAL gate and `ignore-unfixed: false`. Deferred scan [42526403390](https://github.com/pooya-labs/orbit/actions/runs/42526403390) completes source audit and dual-architecture runtime probes; the remaining upstream HIGH is Guava 33.6.0-jre.
- [x] Configure native amd64 and arm64 candidate-build jobs in [42526403434](https://github.com/pooya-labs/orbit/actions/runs/42526403434); both passed source fetch, patch application and effective-POM version checks.
- [x] Complete both native core/Quarkus builds; verify the generated distributions contain only Guava 33.7.2-jre; pass readiness/OIDC, live runtime-map and FTS5 compile-option checks on amd64 and arm64.
- [x] Confirm raw Trivy still detects the FTS5 package finding and the exact-package VEX removes only that scoped finding on both architectures.
- [x] Fix the workflow lifecycle error: execute the runtime-map assertion while the named Keycloak container is still alive, before the startup step's cleanup trap runs.
- [x] Make the VEX-filtered candidate scan a real HIGH/CRITICAL gate with Trivy `exit-code: 1`; `ignore-unfixed: false` remains unchanged.
- [x] Publish the reproducible multi-architecture candidate and record its immutable digest; strict scans against that exact published image passed on AMD64 and ARM64 in [run 42654381960](https://github.com/pooya-labs/orbit/actions/runs/42654381960). Do not infer production deployment from this artifact validation.

**Candidate validation passed:** exact PR head `1d120b95df1f30daa8136583527ab4c653a72e12` passed both native jobs in [run 42537493841](https://github.com/pooya-labs/orbit/actions/runs/42537493841). Both architectures passed source build, patched distribution verification, candidate image build, readiness/OIDC, runtime-map checks, raw scan, strict VEX-filtered scan and final assertions.

**Historical upstream finding:** run [42537493850](https://github.com/pooya-labs/orbit/actions/runs/42537493850) exposed Guava 33.6.0-jre / CVE-2026-102554 in the unpatched upstream image; that finding was not suppressed. The patched image has since been published with Guava 33.7.2-jre, and exact-digest strict scans passed on both architectures in [run 42654381960](https://github.com/pooya-labs/orbit/actions/runs/42654381960). PR #138 now scans the patched digest. Production Compose remains on 26.8.1 pending a separate promotion decision.


## Post-v1 maintenance closeout — 2026-11-09

The immutable `v1.0.0` tag still points to release commit `d2197244b1c1316cbe4b362d15c838c8eaa4cb1a`. Post-release maintenance is tracked separately and does not reopen the public Pages + synthetic Apps Script release scope.

- [x] Merge [PR #163](https://github.com/pooya-labs/orbit/pull/163): update `github.com/jackc/pgx/v5` to 5.12.0 and `github.com/jackc/puddle/v2` to 2.2.3.
- [x] Merge [PR #164](https://github.com/pooya-labs/orbit/pull/164): update `lucide-react` to 1.70.0 while preserving Next.js 16.4.5.
- [x] Verify main commit `1ddb14ff59426a62656c33952d207ce7a94cccd2`: CI [42628810334](https://github.com/pooya-labs/orbit/actions/runs/42628810334), Security [42628810320](https://github.com/pooya-labs/orbit/actions/runs/42628810320), Pages deployment [42628810160](https://github.com/pooya-labs/orbit/actions/runs/42628810160), and stable-release workflow [42629033390](https://github.com/pooya-labs/orbit/actions/runs/42629033390) all succeeded.
- [x] [PR #157](https://github.com/pooya-labs/orbit/pull/157): OpenTelemetry Go modules 1.47.0; squash-merged as `8759eda737f88958fc24e511b5a8f24357b915c9`. Exact PR-head CI [42624823853](https://github.com/pooya-labs/orbit/actions/runs/42624823853) and Security [42624823618](https://github.com/pooya-labs/orbit/actions/runs/42624823618) passed. CI included Go tests/build, typecheck/build, complete OTLP E2E, authenticated workspace write/query isolation, and PostgreSQL restart/recovery; Security passed dependency, secret-history and image scans.


**Scope boundary:** the two open PRs are post-v1 maintenance/production-hardening work, not blockers for the already-published portfolio demo. Cloudflare origin provisioning, real OIDC tenancy, D1 deployment, production ingestion, human mobile screen-reader acceptance, and target-environment capacity/recovery evidence remain separate post-v1 gates. Keep the monthly infrastructure budget at $0; do not create paid resources or claim a production deployment.



- [x] Verify post-merge Security [42631275307](https://github.com/pooya-labs/orbit/actions/runs/42631275307) and Pages [42631275337](https://github.com/pooya-labs/orbit/actions/runs/42631275337) on application-code baseline `8759eda737f88958fc24e511b5a8f24357b915c9`. Its CI [42631275304](https://github.com/pooya-labs/orbit/actions/runs/42631275304) was cancelled when the documentation-only closeout superseded that commit; it was not a failing test.
- [x] Verify final documentation-only main commit `dd5d05baf8291b0ba7de1a3b5f797a6d44fd2bd3`: CI [42633782680](https://github.com/pooya-labs/orbit/actions/runs/42633782680), Security [42633782856](https://github.com/pooya-labs/orbit/actions/runs/42633782856), Pages [42633783397](https://github.com/pooya-labs/orbit/actions/runs/42633783397), and stable-release verification [42633998789](https://github.com/pooya-labs/orbit/actions/runs/42633998789) all succeeded. Go and telemetry jobs were correctly skipped for its documentation-only diff. The immutable `v1.0.0` tag still points to `d2197244b1c1316cbe4b362d15c838c8eaa4cb1a`.


## Latest ORBIT Keycloak security closeout — 2026-11-09

- [x] Publish patched Keycloak 26.8.2 as `ghcr.io/pooya-labs/orbit-keycloak@sha256:97cc4590f5abd77e23522d6d94c3fe77c82b91c44a4e7acef22aec28c61bf81d` (linux/amd64 + linux/arm64); both exact-digest strict scans passed in [run 42654381960](https://github.com/pooya-labs/orbit/actions/runs/42654381960).
- [x] Re-run deferred identity security against that exact GHCR digest; [run 42663232467](https://github.com/pooya-labs/orbit/actions/runs/42663232467) passed, including native ARM64 runtime reachability and multi-architecture SBOM generation.
- [x] Rebuild patched Keycloak source independently on AMD64 and ARM64; [run 42663232531](https://github.com/pooya-labs/orbit/actions/runs/42663232531) passed both jobs.
- [x] Pass full general Security checks on the synchronized PR head; [run 42663232449](https://github.com/pooya-labs/orbit/actions/runs/42663232449) passed every job.

- [ ] Decide separately whether to promote the immutable artifact into production. Main Compose remains on upstream Keycloak 26.8.1; package visibility was not changed, and this work did not provision identity, DNS, routing, or production tenant traffic.


## ORBIT current-scope closeout — 2026-11-10

The owner has chosen to stop at the existing stable portfolio-demo scope and defer all production hosting and Cloudflare runtime expansion until a future request.

- [x] Owner-confirmed manual accessibility/device review recorded. This does not claim formal WCAG conformance.
- [x] PR [#138](https://github.com/pooya-labs/orbit/pull/138) merged to main as 3c8ebc56d34a4c9ed553338c7d8531bb1c4a0e20.
- [x] Patched Keycloak 26.8.2 GHCR artifact published for AMD64/ARM64; exact immutable digest sha256:97cc4590f5abd77e23522d6d94c3fe77c82b91c44a4e7acef22aec28c61bf81d passed strict scans on both architectures; independent source rebuild and deferred identity scan passed.
- [x] Main Pages deployment and stable-release verification succeeded after the merge. The immutable v1.0.0 tag was not moved.
- [x] Closeout verified: PR [#169](https://github.com/pooya-labs/orbit/pull/169) passed exact-head CI [42723914925](https://github.com/pooya-labs/orbit/actions/runs/42723914925) and all Security jobs [42723915044](https://github.com/pooya-labs/orbit/actions/runs/42723915044), including full-history Gitleaks, dependency checks, HIGH/CRITICAL image scans and CycloneDX SBOMs. After merge as 5da415f53fb7e50ac5645158ff7f4f1e5df02466, main CI [42724757890](https://github.com/pooya-labs/orbit/actions/runs/42724757890), Security [42724757906](https://github.com/pooya-labs/orbit/actions/runs/42724757906), Pages deployment [42724758064](https://github.com/pooya-labs/orbit/actions/runs/42724758064), and stable-release verification [42724838160](https://github.com/pooya-labs/orbit/actions/runs/42724838160) all succeeded. The v1.0.0 tag remains unchanged.
- Production Compose remains on Keycloak 26.8.1. The patched image is a validated registry artifact, not a production deployment.
- No origin VM, real OIDC tenant, DNS/ingress, live tenant traffic, D1 deployment, or paid service was provisioned. The Pages + synthetic Apps Script portfolio demo remains the delivered product.
- Production backup scheduling, durable-history retention policy, target-environment load/capacity measurement, RPO/RTO drills, and any Cloudflare/Go production runtime remain deferred. They are not blockers for the current portfolio-demo scope.
