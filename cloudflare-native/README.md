# ORBIT Cloudflare-native runtime (separate deployment profile)

This is a new, independent Cloudflare runtime profile. It does not replace the existing `orbit-runtime` edge gateway, the Go/PostgreSQL reference implementation, the GitHub Pages build, or the Apps Script demo.

## What is included in the first increment

- A separate Worker named `orbit-native-runtime`; it is deliberately not exposed on `workers.dev` yet.
- Workspace creation bootstraps five curated services, two incidents and two example alert rules. Service/incident records are tagged `demo_seed`, and sample descriptions explicitly say the data are synthetic.
- D1 schema for workspace data, memberships, immutable audit events, services, incidents and timelines, alerting, notification records, deployment annotations and SLO definitions.
- OIDC access-token verification using the same issuer/audience model as the existing browser PKCE client and Go API.
- Workspace role checks and tenant-scoped reads/writes for workspace, membership, audit, service and incident endpoints.
- Alert-rule CRUD, deployment annotation list/create/compare, SLO-definition CRUD and notification-channel metadata endpoints.
- Frontend-compatible global service/incident inventory filtered to workspaces where the caller is a member.
- Exact Pages-origin CORS, request IDs, bounded JSON bodies, fail-closed database/auth configuration, and tests for core policy helpers.

## What is not ready yet

This is a staged foundation, not a claim of feature parity or a deployable v1. Live alert evaluation, webhook delivery, SLO error-budget calculation, API rate limiting and telemetry query routes are not implemented in this profile. In particular, D1 does not replace the current Prometheus/Loki/Tempo telemetry pipeline. The Cloudflare-native profile must either present clearly labelled D1/Workers runtime events and demo samples, or retain a separately hosted telemetry source; it must not label synthetic samples as real telemetry.

The Worker API returns the direct `ApiMetadata` object at `/api/v1/meta` and uses the same `{ data: ... }` envelopes for collection/resource routes as the current typed client. The frontend still uses its existing OIDC Authorization Code + PKCE flow. The Worker validates the resulting Bearer access token against a configured HTTPS OIDC issuer and API audience, including the same `iat`, `exp`, `sub`, `typ`, `kid`, signing-algorithm and configurable role-claim checks as the Go verifier. No identity provider is bundled or hosted by this Worker. A compatible issuer and audience must be selected and configured before the API can be activated.

## Deployment safety

- `workers_dev` is false until the account's D1 binding and OIDC configuration have been verified.
- The D1 database ID is an explicit placeholder and must be replaced only after the intended database has been created.
- Do not run remote migrations or deploy this profile until the database and OIDC settings have been reviewed.
- No database, binding, secret, route, or billing setting has been changed in the Cloudflare account by this source commit.
- The existing Apps Script runtime remains independent and unchanged.

## Local checks

From the repository root:

```sh
node --test cloudflare-native/worker.test.mjs
```

Apply migrations only after creating the matching D1 database:

```sh
npx wrangler d1 migrations apply orbit-native --remote --config cloudflare-native/wrangler.jsonc
```

The current migration is the initial schema. Do not configure the GitHub Pages API URL to this Worker until the remaining evaluator/telemetry gaps, hosted OIDC decision and end-to-end acceptance matrix are complete.
