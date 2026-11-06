# ORBIT Cloudflare edge runtime

The Cloudflare Worker is the public edge entry point for the existing ORBIT backend. It is deliberately **not a second application backend**.

## Canonical architecture

```text
Browser — GitHub Pages dashboard
        |
        | HTTPS + user OIDC access token
        v
Cloudflare Worker: orbit-runtime
  - exact Pages-origin CORS
  - explicit API path allowlist
  - request ID and trusted Cloudflare client IP
  - private Worker-to-origin gateway secret
  - no authenticated response cache
        |
        | HTTPS + private gateway secret
        v
Caddy — existing API hostname / origin
  - rejects API traffic without Worker secret
  - strips gateway secret before Go API
  - forwards verified edge client IP
        |
        v
Existing Go API — canonical auth, role and workspace policy
        |
        +---- PostgreSQL — authoritative workspace and operations data
        |
        +---- authenticated OTLP/HTTP + OTLP/gRPC gateways
                  |
                  v
             private OTel Collector
                  |
             +----+----+
             |    |    |
          Prom. Loki Tempo
          metrics logs traces
```

## Resources

- Worker: `orbit-runtime`
- Public edge URL: https://orbit-runtime.pooyagame141.workers.dev
- Browser origin: `https://pooya-labs.github.io`
- API origin: the existing `ORBIT_API_DOMAIN` DuckDNS hostname behind Caddy; never point the browser directly at it.
- Identity provider: existing self-hosted Keycloak configuration in the original zero-cost deployment runbook.
- Source of truth: existing Go API + PostgreSQL + Collector + Prometheus/Loki/Tempo.

There is no D1 binding, no duplicated workspace/application data, and no Worker-owned identity or notification system in the accepted architecture.

## Request paths

Browser dashboard requests go to the Worker at `/api/v1/*`, `/health/live`, or `/health/ready`. The Worker accepts only the configured GitHub Pages origin for browser CORS, forwards the user's Authorization Bearer token unchanged, and adds a private gateway secret and Cloudflare-derived client IP. It never selects an arbitrary upstream host.

Caddy requires the Worker gateway secret for `/api/v1/*` and `/health/ready`, strips the secret, and forwards requests to the existing Go API. The Go API remains responsible for OIDC/JWKS validation, roles, workspace membership, response contracts, shared PostgreSQL rate limits, and all business operations.

The Worker does not proxy OTLP or gRPC. Workloads continue to use the separate authenticated OTLP/HTTP and OTLP/gRPC ingress hostnames in the original Caddy setup. The Go gateway enforces the dedicated workload-token audience, an explicit workspace/signal grant, canonical tenant attribution, reserved-attribute rejection, the reviewed attribute allowlist, shared rate limits, payload limits, concurrency admission, timeout and backpressure before forwarding to the private Collector.

## Local configuration and deployment

Read [the step-by-step Workers Free deployment runbook](../docs/deployment/cloudflare-free-worker.md) first. It begins with the billing/usage-model gate and then walks through origin setup, Worker deployment, Pages configuration and acceptance checks.

The API hostname remains a public HTTPS transport endpoint, but Caddy protects all API and readiness routes with the Worker-only gateway secret; it is not a browser-facing API URL:

1. Configure the original zero-cost origin stack from [the free deployment runbook](../docs/deployment/free-only-orbit.md). This is the same Go/PostgreSQL/Keycloak/Collector/Prometheus/Loki/Tempo deployment, not a replacement.
2. The existing `deploy/production/free-stack.sh prepare` generates the private `.env.production` file, including a random `ORBIT_WORKER_GATEWAY_SECRET`. Keep that file mode 600 and out of Git.
3. Start the origin stack, confirm DNS/TLS and Keycloak/API health.
4. The script uses Wrangler 4.158.0 pinned in the repository and the account selected by `account_id` in `cloudflare/wrangler.jsonc`; run it from the existing deployment environment:
   `bash deploy/production/configure-cloudflare-worker.sh`
5. The script reads the origin and secret from `.env.production`, writes a temporary mode-600 secrets file, deploys `cloudflare/wrangler.jsonc`, then deletes the temporary file. It does not print the secret.
6. Build GitHub Pages with `NEXT_PUBLIC_ORBIT_API_URL=https://orbit-runtime.pooyagame141.workers.dev` and the same existing Keycloak issuer, public client ID and API audience used by the origin deployment. Open the dashboard's Connect a source panel and test the Worker URL.

Never store the gateway secret in GitHub Pages variables, browser code, URL query parameters, or public documentation. The browser only receives its own short-lived user access token.

## Free-tier and service choices

The Worker is a thin edge proxy with no resource bindings. Its Wrangler configuration explicitly caps CPU time at **10 ms per invocation**, matching the Workers Free limit. Workers Free currently allows **100,000 requests per day**; exceeding that daily request allowance causes requests to fail until the UTC reset. Free also limits an invocation to 50 subrequests.

Workers Logs are included on Free. Until November 30, 2026, the documented Free allowance is 200,000 log events per day with three-day retention. Beginning December 1, 2026, Workers Logs and Workers Traces move to the shared Cloudflare Observability allowance: **0.5 GB ingestion per day with seven-day retention included**. On Free, Cloudflare stops ingesting additional telemetry when the daily allowance is reached; it does not continue metered ingestion. The Worker keeps 10% head sampling for logs and traces to conserve that allowance. Sampling reduces volume but is not itself a billing-plan switch.

No D1, R2, Hyperdrive, Durable Objects, Queues, Cloudflare Tunnel, Workers AI, or paid add-on is required. The Go/PostgreSQL/Keycloak/Collector origin remains separate and must stay within its own $0/month hosting budget.

The currently deployed Worker reports `usage_model: standard`. Cloudflare documents Standard as a Workers Paid-plan usage model, so the account/Worker must be switched to the intended Free plan in the dashboard before real traffic is enabled. The connected API token cannot read the account subscription/billing endpoint. Do not try to force Free by setting the legacy `bundled` usage model; that is not the Free plan. No billing or usage-model change is performed by this repository.

Keep the existing $0/month hard budget. If the Free request or Observability allowance is exhausted, allow Cloudflare's Free-plan limit behavior to stop requests/telemetry rather than enabling paid billing or weakening the security boundary.

## Verification

The repository CI includes a Worker contract test for liveness, fail-closed origin configuration, path allowlisting, exact-origin preflight, Bearer-token forwarding, gateway-secret/IP-header replacement, streamed mutation bodies, query preservation, and safe handling of upstream cookies/redirects. CI also checks the Caddy ingress configuration and Go request-IP behavior. The existing Go/PostgreSQL and observability jobs remain authoritative for real API auth/roles, workload identity, OTLP/HTTP+gRPC, workspace A/B isolation, durable outbox, webhook safety, retention/cardinality, Prometheus/Loki/Tempo and recovery.

A successful Worker deployment is not proof that the remote origin, DNS/TLS, Keycloak, or end-to-end flows are ready. These still require owner-origin smoke tests described in the deployment runbook.


## Billing verification gate

The Cloudflare API reports `usage_model: standard` for the deployed Worker. Cloudflare documents the Standard usage model as a Workers Paid-plan capability. The connected API credential cannot read the account's subscription/billing usage endpoint (HTTP 403), so the account's current billing plan is **not verified** by this integration. No billing plan, subscription, or usage model has been changed by this work. Before routing real traffic, verify in Cloudflare Dashboard → Workers & Pages → Usage Model / Billing that this Worker is on the intended no-cost plan. Do not assume 10% sampling by itself guarantees zero cost, and do not change to a paid plan to unblock deployment.
