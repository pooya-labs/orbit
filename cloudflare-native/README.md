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
- Authenticated API read/write rate limits using Cloudflare Workers Rate Limiting bindings; hashed OIDC identities, distinct read/write budgets, and fail-closed behavior when a binding is missing or unavailable.

## What is not ready yet

This is a staged foundation, not a claim of feature parity or a deployable v1. Live alert evaluation, webhook delivery, SLO error-budget calculation and telemetry query routes are not implemented in this profile. In particular, D1 does not replace the current Prometheus/Loki/Tempo telemetry pipeline. The Cloudflare-native profile must either present clearly labelled D1/Workers runtime events and demo samples, or retain a separately hosted telemetry source; it must not label synthetic samples as real telemetry.

The Worker API returns the direct `ApiMetadata` object at `/api/v1/meta` and uses the same `{ data: ... }` envelopes for collection/resource routes as the current typed client. The frontend still uses its existing OIDC Authorization Code + PKCE flow. The Worker validates the resulting Bearer access token against a configured HTTPS OIDC issuer and API audience, including the same `iat`, `exp`, `sub`, `typ`, `kid`, signing-algorithm and configurable role-claim checks as the Go verifier. No identity provider is bundled or hosted by this Worker. A compatible issuer and audience must be selected and configured before the API can be activated.

## Authenticated API rate limits

The Worker uses two native Workers Rate Limiting bindings after successful OIDC authentication and before any D1 operation:

| Binding | Budget | Scope |
|---|---:|---|
| `ORBIT_READ_LIMITER` | 120 requests / 60 seconds | Per authenticated subject |
| `ORBIT_WRITE_LIMITER` | 30 requests / 60 seconds | Per authenticated subject |

The key is a SHA-256 hash of the OIDC issuer and subject tuple. Raw issuer/subject values are not sent to the rate-limit binding. Read requests (`GET`/ `HEAD`) use the read budget; other API methods use the write budget. A rejected request returns `429 rate_limit_exceeded` with `Retry-After: 60` and does not reach D1.

These limits are an abuse/burst guard, **not a strict global quota**. Cloudflare applies Workers Rate Limiting counters approximately per Cloudflare location, and updates are eventually consistent; requests can exceed a configured threshold during propagation. They do not enforce an exact cross-region limit and do not replace the Workers Free daily request cap or D1 daily row-read/row-write limits. Unauthenticated OIDC discovery/JWKS work occurs before the authenticated API limiter and needs separate edge/WAF controls if exposed to untrusted traffic.

The two positive integer `namespace_id` values in `wrangler.jsonc` must be unique within the Cloudflare account. Confirm that they are unused before the first deployment; change them if the account already has a collision. No rate-limit namespace has been provisioned by this source change.

For strict cross-location quotas, Cloudflare recommends a Durable Object-backed limiter. This profile does not add one: it would introduce stateful storage/compute and additional quota considerations. Revisit only if measured traffic requires a strict global cap.

## Recommended free OIDC configuration

The implementation is designed to reuse the frontend's existing Authorization Code + PKCE client. The recommended no-cost issuer for the first hosted demo is an Auth0 Free tenant using its default `*.auth0.com` domain (not a paid/custom domain). Auth0 advertises a Free tier with up to 25,000 monthly active users and no credit card required to sign up. Custom-domain activation can require card verification, so do not configure one for this profile.

1. Create an Auth0 **Single Page Application** and an API resource.
2. Set the API identifier to exactly `https://pooya-labs.github.io/orbit-api` and set its signing algorithm to **RS256**.
3. In the SPA settings, allow this callback URL: `https://pooya-labs.github.io/orbit/auth/callback/`.
4. Set the allowed web origin to `https://pooya-labs.github.io` and the allowed logout URL to `https://pooya-labs.github.io/orbit/`.
5. Keep the tenant's default Auth0 domain. Set the issuer to its exact HTTPS URL with the trailing slash, for example `https://YOUR-TENANT.REGION.auth0.com/`.
6. Use the same values for the frontend build and Worker: `NEXT_PUBLIC_ORBIT_OIDC_ISSUER` / `ORBIT_OIDC_ISSUER` = the exact issuer; `NEXT_PUBLIC_ORBIT_OIDC_CLIENT_ID` = the SPA client ID; `NEXT_PUBLIC_ORBIT_OIDC_AUDIENCE` / `ORBIT_OIDC_AUDIENCE` = `https://pooya-labs.github.io/orbit-api`; and `NEXT_PUBLIC_ORBIT_OIDC_SCOPE` = `openid profile email`.

The SPA client ID is public and belongs in the Pages build configuration. Do **not** put an Auth0 client secret in the browser bundle. The Worker does not need a client secret; it verifies the signed access token against the issuer's public JWKS. Workspace roles continue to come from D1 memberships, not from user-editable profile metadata.

This is a recommendation only. No Auth0 tenant, Cloudflare secret, D1 database, Worker route or Pages environment variable has been created or changed by this branch. For reference, see the [Auth0 Free plan](https://auth0.com/pricing), [Auth0 SPA guidance](https://developer.auth0.com/resources/get-started/spa), and [Auth0 API audience guidance](https://support.auth0.com/center/s/article/value-of-the-audience-attribute-in-access-token-requests-and-its-significance).

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



## Native request Metrics — staged source only

The Worker source contains a request-only Metrics path backed by Workers Analytics Engine:

- Each completed request records a fixed route template, HTTP method, response status, elapsed milliseconds, service version and deployment environment.
- Workspace partitioning uses a SHA-256 digest of the workspace UUID. Raw workspace IDs, issuer/subject values, access tokens, query strings, request bodies and payload contents are not written to Analytics Engine.
- The Metrics API verifies workspace membership before querying a tenant partition. It accepts fixed time windows, three metric IDs and three allow-listed grouping dimensions; results are cached for 60 seconds per Worker isolate.
- Analytics Engine data is adaptively sampled. The SQL query explicitly sums each row's `_sample_interval` for total requests and uses weighted conditional sums for 2xx/3xx and 4xx/5xx responses; it does not treat sampled rows as full event counts. The dashboard identifies the source and discloses adaptive sampling. Results reaching the 5,000-row safety cap fail closed rather than returning a silently truncated chart.
- This profile currently provides request Metrics only. Logs and Traces continue to return explicit 503 responses; live alert evaluation, SLO budgets and notification delivery are not implemented.

The Wrangler configuration declares an Analytics Engine dataset binding for writes and an Analytics SQL binding for reads. The CI workflow validates the Analytics SQL binding with Wrangler 4.158.0 and tests the write/query contract with mocks. This does not provision or remotely test a Cloudflare dataset, D1 database, Worker route or identity provider.

Cloudflare's published Workers Free allowance for Analytics Engine is 100,000 data points written per day and 10,000 read queries per day. Cloudflare currently states Analytics Engine usage is not being billed yet, while publishing future pricing. Treat both the limits and temporary billing statement as subject to change; do not switch to Workers Paid or enable a paid fallback. Re-check the account's plan and current pricing before any deployment. See [Analytics Engine pricing](https://developers.cloudflare.com/analytics/analytics-engine/pricing/).

The Analytics SQL Wrangler binding requires Wrangler 4.145.0 or later. The dataset binding is declared through `analytics_engine_datasets`; Analytics Engine datasets are created on first write after binding configuration, not by this source-only change. See [Analytics SQL binding](https://developers.cloudflare.com/workers/runtime-apis/bindings/analytics-sql/), [Wrangler Analytics Engine datasets](https://developers.cloudflare.com/workers/wrangler/configuration/#analytics-engine-datasets), and [Analytics Engine write API](https://developers.cloudflare.com/workers/examples/analytics-engine/) and [sampling model](https://developers.cloudflare.com/analytics/analytics-engine/sampling/).

**Deployment remains blocked by design:** no remote D1 database, Analytics Engine dataset, OIDC tenant, secret, Worker route or Pages API-origin switch is created by this change. Keep `workers_dev: false`, the Apps Script demo unchanged and the $0/month constraint intact.
