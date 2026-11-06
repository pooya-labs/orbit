# ORBIT Cloudflare runtime — edge expansion

This is the post-v1 Cloudflare entry point. It preserves the Apps Script portfolio demo and follows the original ORBIT backend and deployment architecture.

## Architectural correction — 2026-11-06

The first D1-backed prototype was not aligned with ORBIT's accepted architecture. ORBIT already has the real Go API, PostgreSQL domain model, OIDC/role checks, workspace authorization, authenticated OTLP gateways, Collector and three telemetry stores. Duplicating those components inside a Worker/D1 implementation would have produced two different identity systems, two sets of operations records, and a second telemetry store.

[ADR 0006](decisions/0006-cloudflare-edge-gateway.md) therefore defines the correct Cloudflare role: a narrow edge API gateway over the canonical origin.

## Runtime split

| Entry point | Role | Data authority |
| --- | --- | --- |
| Apps Script Web App | Existing zero-cost, low-volume synthetic portfolio demo; unchanged | Owner-private Google Spreadsheet and Apps Script session boundary |
| Cloudflare Worker | Edge entry point for the full Go API deployment | No application database; no user/workload session storage |
| Go API + PostgreSQL | Canonical control plane, identity/roles, workspaces, incidents, alerts, outbox, deployments, SLOs and audit | PostgreSQL |
| OTel Collector + Prometheus/Loki/Tempo | Authenticated telemetry ingestion and specialist signal storage | Prometheus metrics, Loki logs, Tempo traces |
| GitHub Pages | Shared static Next.js dashboard | No server-side state |

Apps Script remains synthetic-only. It is not expanded to accept production telemetry, send external webhooks, or imitate the full Go runtime.

## Cloudflare Worker contract

- Worker: `orbit-runtime`
- Public entry URL: https://orbit-runtime.pooyagame141.workers.dev
- GitHub Pages origin: `https://pooya-labs.github.io`
- API browser URL: Worker URL, not the origin API hostname.
- Origin API hostname: existing `ORBIT_API_DOMAIN` DuckDNS endpoint routed through Caddy.
- Only `/api/v1/*`, `/health/live` and `/health/ready` are accepted.
- Browser Bearer access token is forwarded to Go for validation; the Worker does not duplicate OIDC or roles.
- Worker-to-origin uses the private `ORBIT_GATEWAY_SECRET`; Caddy rejects direct API requests without it and strips it before Go.
- Cloudflare-derived client IP is forwarded through a separate header for the existing shared IP rate limit.
- Authenticated responses are never cached. Cookies and upstream CORS grants are not passed through.

The Worker is an HTTP API edge only. It does not accept raw telemetry, and it does not proxy OTLP/gRPC. Workloads retain the original dedicated TLS hostnames and routes:

- OTLP/HTTP → Caddy → Go authenticated workload HTTP gateway → private Collector.
- OTLP/gRPC → Caddy HTTP/2 → Go authenticated workload gRPC gateway → private Collector.

The existing Go gateways remain responsible for the separate workload-token audience, workspace selector and grant, reserved tenant attributes, canonical workspace injection, reviewed attribute allowlist, shared rate limiting, concurrency/backpressure, request size and timeout.

## Existing infrastructure reused without replacement

- Keycloak 26.8.1 remains the provider-neutral OIDC issuer for the browser Authorization Code + PKCE client and separate workload clients.
- PostgreSQL remains authoritative for human workspace membership, workload identities/grants, alert evaluator coordination, durable notification outbox and attempt history, rate limits, deployments, SLOs, incidents and audit.
- Collector remains private; Prometheus stores metrics, Loki stores logs, Tempo stores traces.
- Caddy remains the only origin ingress and TLS termination.
- Existing encrypted PostgreSQL backup and retention runbooks remain applicable.
- Redis and NATS remain deferred under ADR 0004; Cloudflare Queues are not introduced.
- No D1, R2, Hyperdrive, Durable Objects, Cloudflare Tunnel, Workers Paid, Containers or paid add-on is required.

## Actual Cloudflare account state

The connected Cloudflare account has the workers.dev subdomain `pooyagame141`, but no zones, Cloudflare Tunnels or Hyperdrive configs were present during inspection. R2 was not enabled. The design therefore uses the existing DuckDNS/Caddy API hostname as the origin and does not assume a Cloudflare-managed zone or private network connector.

The owner-provided Billing → Subscriptions view confirms **Workers Free — Active**, with an option to upgrade to Workers Paid and no payment method on file. No billing changes were made. The deployed Worker is version `0.2.3` (version 8), with Logs and Traces enabled at 10% sampling. Its workers.dev route is currently disabled even though the branch's Wrangler config sets `workers_dev: true`; the next guarded deployment is expected to re-enable it. If it remains disabled, enable it under **Workers & Pages → orbit-runtime → Settings → Domains & Routes → workers.dev**.

Remote settings confirm no resource bindings and no Worker secrets are attached yet; the secret list is empty. The private API origin and `ORBIT_GATEWAY_SECRET` are intentionally not configured until the original Go/Caddy origin is ready. API routes therefore remain fail-closed. The per-Worker API metadata returns `usage_model: standard`, which is left unchanged; the dashboard subscription view is the evidence for the active account plan. Cloudflare API read access covers Worker configuration, versions, subdomain state and secret names, but the OAuth integration receives HTTP 403 when reading account subscription records.

## Configuration

The original deployment script generates `ORBIT_WORKER_GATEWAY_SECRET` inside the private mode-600 `.env.production`. After the origin and Keycloak are live, run:

```bash
bash deploy/production/configure-cloudflare-worker.sh
```

That script reads the private origin and secret from `.env.production`, uses Wrangler, passes the secret through a temporary protected secrets file, and removes that file on exit. Before deployment it also requires an explicit confirmation that the Cloudflare dashboard shows Workers Free; otherwise it exits without publishing. Follow [the step-by-step Free deployment runbook](deployment/cloudflare-free-worker.md). Keep the gateway secret out of source control, Pages variables, client code, browser storage and URLs.

For the Cloudflare Pages build, set `NEXT_PUBLIC_ORBIT_API_URL` to `https://orbit-runtime.pooyagame141.workers.dev`; retain the exact same Keycloak issuer, public client ID and API audience as the Go origin. No new identity provider or separate Cloudflare role schema is required.

## Roadmap — no duplicate application logic

### CF01 — Edge and origin trust
- Confirm the existing Go/Keycloak/Caddy origin is deployed and healthy.
- Configure the private Worker gateway secret and origin.
- Verify Caddy rejects direct API calls without the Worker secret.
- Verify the Worker forwards the original user token and trusted client IP.

### CF02 — Browser connection and full API parity
- Build Pages with the Worker URL and existing Keycloak PKCE settings.
- Run every existing typed API-client route through the Worker.
- Preserve the Go API's response, role, workspace and audit contracts; add no duplicate route implementations to the Worker.
- Test viewer/operator/administrator behavior and cross-workspace denial.

### CF03 — Telemetry and operations
- Verify OTLP/HTTP protobuf and OTLP/gRPC through their existing Caddy endpoints.
- Verify workload-specific audience, explicit workspace/signal grants, canonical tenant attribution, reserved-attribute rejection, quotas and revocation.
- Verify Collector routing and Metrics/Logs/Traces readback, plus workspace A/B isolation.
- Keep webhook SSRF protections, PostgreSQL outbox, append-only history and SLO semantics in the canonical Go runtime.

### CF04 — Dual-runtime acceptance
- Test Apps Script and Cloudflare in separate tabs and browser profiles.
- Confirm the Apps Script synthetic workspace and Cloudflare OIDC identity/session never share tokens or storage.
- Keep the Apps Script Web App deployment and private spreadsheet unchanged.
- Record live origin/API/telemetry smoke evidence.

### CF05 — Separate runtime release
- Run Worker contract tests, Caddy validation, Go race tests, PostgreSQL migrations/backup tests, full telemetry E2E, browser PKCE, security and live origin tests.
- Publish a separate Cloudflare edge release only after evidence is complete. Do not rewrite the immutable ORBIT v1.0.0 tag.

## Cost and safety boundary

Keep the original $0/month infrastructure policy. Workers Free automatically enforces a 10 ms CPU cap and 50 external subrequests per invocation; the repository does not set `limits.cpu_ms`, because Cloudflare documents custom Wrangler `limits` as supported only for the Standard usage model. Workers Free allows 100,000 requests/day. Requests above the daily allowance fail until the UTC reset.

Workers Logs are included on Free. Until November 30, 2026, the documented Free allowance is 200,000 log events/day with three-day retention. Beginning December 1, 2026, Workers Logs and Workers Traces share the Cloudflare Observability Free allowance of 0.5 GB ingestion/day with seven-day retention included; Free stops ingesting new data after the daily allowance is exhausted. The Worker keeps 10% head sampling for both signals to reduce volume.

The owner-provided Cloudflare Billing → Subscriptions view confirms **Workers Free — Active**, offers an upgrade to Workers Paid, and shows no payment method on file. This is the confirmed account-plan evidence. The Worker settings API separately reports `usage_model: standard`; do not infer a paid subscription from that field alone and do not force the legacy `bundled` value. Leave the Worker usage-model field unchanged. The connected OAuth integration cannot read `/accounts/{account_id}/subscriptions` (HTTP 403; Billing Read/Write API-token permission is required), so it cannot independently retrieve subscription records. No billing, subscription, or usage-model change has been made by this repository. No paid billing, automatic overages, paid DNS, managed databases, or paid Cloudflare products are authorized. If the Worker Free allowance is insufficient, leave the edge disabled rather than weakening the security boundary or adding an unapproved paid service.

No real customer telemetry should be sent until the original Keycloak, workload grants, origin ingress, retention, backup/restore, capacity and workspace isolation gates have passed.


## Billing-plan verification gate

The deployed Worker's Cloudflare settings currently report `usage_model: standard`. Cloudflare's current pricing documentation associates Standard usage with Workers Paid. The connected API credential does not have permission to read billable subscription information (the read-only endpoint returned HTTP 403), so the account billing plan could not be independently confirmed. No paid plan, subscription, or usage-model change was made. Before any origin traffic is enabled, verify the Worker plan/usage model in the Cloudflare dashboard. Keep the runtime disabled rather than enabling paid billing to clear a limit.
