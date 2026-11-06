# ADR 0007 — Cloudflare-native D1 runtime profile

- **Status:** Accepted for implementation
- **Date:** 2026-11-06
- **Scope:** A separate no-hosting-cost Cloudflare deployment profile for ORBIT

## Context

The existing Cloudflare Worker is a thin edge gateway for the canonical Go API and PostgreSQL database. That topology requires a separately hosted origin. The owner has confirmed that no external VPS, laptop-hosted process, or paid service is acceptable for the Cloudflare-hosted demo.

The repository also contains a separate Apps Script demo, which must remain independent and unchanged.

## Decision

Build a second, isolated Cloudflare runtime profile using Workers + D1:

- Keep `cloudflare/worker.mjs` (`orbit-runtime`) as the thin gateway for the existing Go/PostgreSQL reference stack.
- Add `cloudflare-native/worker.mjs` (`orbit-native-runtime`) as a distinct D1-backed application API.
- Keep the existing Go/PostgreSQL implementation as the richer engineering/reference core; do not delete or rewrite its code.
- Keep Apps Script and its synthetic data independent.
- Reuse the frontend's OIDC Authorization Code + PKCE flow. The native Worker validates an access token against a configurable HTTPS issuer and audience; workspace memberships and roles remain authoritative in D1.
- Preserve workspace scoping and append-only audit semantics in the D1 schema.
- Deploy the native Worker separately and do not switch the Pages API URL until feature coverage and acceptance tests are complete.

## Explicit feature boundary

Workers + D1 can host the dashboard's transactional state and API logic without a VPS. It does not supply a drop-in replacement for PostgreSQL or the existing Prometheus, Loki and Tempo services.

The profile therefore migrates workspace, service, incident, audit, alert-definition/state, notification metadata, deployment annotation and SLO-definition records, and exposes member-filtered global service/incident inventory for the existing frontend. Alert evaluation and notification delivery require separate bounded Worker execution and retry design. Webhook secrets must not be stored in plaintext D1 rows.

The existing raw OTLP ingestion path and Prometheus/Loki/Tempo querying are not represented as completed by D1. Any telemetry shown by the native profile must be labelled according to its actual source (for example, Workers runtime observability or explicitly synthetic demo data). It must not be presented as real production telemetry.

## Consequences

- A compatible OIDC provider remains a prerequisite; this decision does not host identity or issue user tokens. Auth0 Free is the documented recommendation for the initial demo, but a tenant has not been created or configured.
- PostgreSQL SQL, pgx repositories and server-side Go code are not reused directly. D1 migrations use SQLite-compatible SQL and the Worker uses the D1 binding API.
- D1's Free-plan daily row read/write and storage limits become operational constraints. Queries need tenant-leading indexes, bounded result sets and explicit handling when quotas are reached.
- The native Worker is a separate deployment and can be rolled back without affecting the existing gateway, Pages site or Apps Script demo.
- The legacy D1 prototype that was previously deleted is not resurrected; this is a newly versioned, separately named runtime with its own schema and acceptance gates.

## Implementation gates

1. Foundation: schema, JWT verification parity, tenant isolation, frontend-compatible metadata and global/workspace service/incident APIs with signed-token and SQLite contract tests.
2. Operations: alert CRUD/evaluation, deployment annotations, SLO CRUD/budget strategy, audit completeness.
3. Notifications: encrypted webhook secrets, bounded retries, delivery attempts and abuse protections.
4. Telemetry: honest source model, data retention and query quotas; no fake Prometheus/Loki/Tempo parity.
5. Frontend: compatibility tests for every client method, safe API-origin switch, visual and role-based acceptance.
6. Cloudflare account: create D1 database, apply migrations, configure OIDC issuer/audience, enable workers.dev only after acceptance, deploy.
