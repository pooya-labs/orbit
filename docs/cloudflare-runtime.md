# ORBIT Cloudflare runtime — post-v1 expansion

This is an explicitly separate post-v1 runtime. It preserves the released Apps Script portfolio demo and shares the existing static dashboard/API contracts where compatible.

## Deployment checkpoint — 2026-11-06

- Git branch: `feat/cloudflare-dual-runtime`
- Worker: `orbit-runtime`
- Worker URL: https://orbit-runtime.pooyagame141.workers.dev
- D1 database: `orbit-runtime`, UUID `579805c5-7688-4a1a-a26f-4ce8ff2af04e`, EU jurisdiction.
- Worker upload succeeded with Workers Logs and Traces enabled.
- Initial D1 schema migration was applied successfully.
- Cloudflare account access is connected. R2 is not enabled in this account and is not a dependency of this first slice.
- The live HTTP smoke request could not be completed from the current verification network because requests to the workers.dev hostname are blocked there. The Cloudflare control-plane confirms the Worker upload; this is not a successful external endpoint smoke test.

## Runtime split

| Runtime | Purpose | Data and capabilities |
| --- | --- | --- |
| Apps Script | Keep the existing zero-cost public synthetic demo unchanged | Private Google Spreadsheet, browser-profile demo workspaces, simulation-only notifications, no real customer telemetry |
| Cloudflare | New full-feature runtime | Dedicated D1 database, OIDC/JWKS validation, workspace membership/grants, OTLP/HTTP JSON ingestion, bounded Metrics/Logs/Traces reads |
| GitHub Pages | Shared static UI | No duplicated dashboard or forked frontend |

The Apps Script restrictions remain intentional. Do not weaken its quotas, synthetic-only boundary, bridge validation, or private Spreadsheet access to make it imitate the Cloudflare runtime.

## Initial implementation

- `cloudflare/worker.js`: Cloudflare Worker API.
- `cloudflare/migrations/0001_initial.sql`: dedicated D1 schema for identity, workspace membership, workload grants, telemetry, incidents, alerts, notifications, deployments, SLOs, and audit.
- Worker health and metadata routes: `/health/live`, `/health/ready`, `/api/v1/meta`.
- OIDC access-token verification accepts only RS256/ES256, validates issuer, audience and time claims, then checks membership/grants in D1.
- OTLP/HTTP JSON routes: `POST /v1/otlp/metrics`, `POST /v1/otlp/logs`, `POST /v1/otlp/traces`.
- Workspace-scoped reads: `GET /api/v1/workspaces/{workspaceID}/telemetry/{services|metrics|logs|traces}`.
- Every workload is mapped to a workspace through a server-side issuer/subject/signal grant; producer-supplied workspace attributes are not trusted.
- Query windows and row counts are bounded. No browser-supplied SQL, PromQL, LogQL, or TraceQL is accepted.

## Not yet enabled — do not imply otherwise

1. Real OIDC provider and browser PKCE configuration; no identity provider or issuer has been provisioned for this Cloudflare runtime.
2. Provisioning/admin APIs for workspaces, principals, memberships, and workload grants.
3. Complete compatibility for all existing API routes: services, incident lifecycle/timeline, alert CRUD/evaluation, notification channel management/delivery attempts, deployment annotations/comparison, SLO budgets, audit search, trace detail, and resource-attribute catalog.
4. Real webhook/email delivery with the existing HTTPS/host allowlist, DNS/public-address, redirect, timeout, retry, and idempotency protections.
5. OTLP/HTTP protobuf and OTLP/gRPC. The first Worker slice accepts OTLP/HTTP JSON only.
6. Complete frontend/API parity and GitHub Pages build configuration; an explicit `?orbitRuntime=cloudflare` entry point now pre-fills the Worker URL and opens the source panel, but does not imply that all dashboard routes are implemented yet.
7. Live clean-browser smoke tests, cross-workspace negative tests, ingestion-to-query tests, retention/cleanup, load/capacity, and backup/recovery acceptance.
8. Cost/limit review for all Cloudflare services before enabling ongoing ingestion or scheduled work.

## Next milestones

### CF01 — Identity and onboarding
- Configure a real OIDC issuer, public browser client, API audience, and callback.
- Add owner-only workspace and workload-grant provisioning with immutable audit records.
- Add rate limits and per-workspace quotas; fail closed when identity or grant checks fail.

### CF02 — Full API parity
- Implement every existing typed client route against D1, preserving the existing response contracts and role policy.
- Port incident timelines, alert evaluation, notification outbox, deployment annotations, SLO semantics, audit history, and trace detail.
- Keep mutations transactional where D1 supports the required atomic boundary; document any semantic differences.

### CF03 — Telemetry parity and operations
- Add OTLP/HTTP protobuf if feasible in Workers; evaluate gRPC separately rather than claiming compatibility.
- Add retention enforcement, bounded cardinality, sampling and quotas.
- Add safe webhook delivery and idempotent retries; keep email disabled until a verified sender is configured.
- Add Cloudflare runtime logs/traces and application-level health metrics.

### CF04 — Shared dashboard and dual-runtime acceptance
- Add an explicit Cloudflare runtime entry point without changing the Apps Script wrapper.
- Ensure both runtimes can be opened simultaneously in separate tabs and never share session tokens or storage.
- Verify the complete UI against both runtime contracts and prove workspace A/B isolation.

### CF05 — Release
- Run typecheck/build, Worker syntax/runtime tests, D1 migration tests, API contract tests, security review, clean-browser end-to-end checks, and Cloudflare deployment smoke.
- Publish a separate Cloudflare runtime release; do not move or rewrite the immutable v1.0.0 tag.

## Safety and cost policy

- Keep the Apps Script demo synthetic-only.
- Do not send production/customer data until identity, grants, workspace isolation, retention, backups, and operational limits have passed their gates.
- Do not enable paid billing, automatic upgrades, or usage-based overages without explicit approval.
- R2 is optional for this design and remains off until a concrete archival/export requirement justifies enabling it.
