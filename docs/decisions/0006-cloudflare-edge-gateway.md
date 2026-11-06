# ADR 0006 — Cloudflare edge gateway over the canonical ORBIT runtime

- Status: Accepted for the post-v1 Cloudflare runtime
- Date: 2026-11-06
- Depends on: ADR 0001 (OIDC access tokens), ADR 0002 (authenticated OTLP), ADR 0003 (workspace routing), ADR 0004 (PostgreSQL outbox; Redis/NATS deferred), ADR 0005 (resource-attribute allowlist)
- Scope: Cloudflare Worker entry point, Caddy API ingress, and the existing Go/PostgreSQL/OTel deployment

## Context

ORBIT already has a complete backend implementation and an accepted deployment shape:

- Go API is the HTTP control plane and owns authorization, route policy, rate limits, and API response contracts.
- PostgreSQL is the authoritative transactional store for principals/memberships, workload grants, incidents/timelines, alerts/evaluation, the notification outbox/attempt history, deployments, SLOs, audit, and shared rate limits.
- The OpenTelemetry Collector is the only route into the telemetry backends. Prometheus stores metrics, Loki stores logs, and Tempo stores traces.
- The authenticated Go workload gateways accept OTLP/HTTP and OTLP/gRPC, verify a separate workload-token audience, authorize one explicit workspace and signal, reject reserved tenant attributes, inject canonical workspace attribution, and apply bounded admission/rate limits.
- Keycloak is the provider-neutral OIDC issuer for browser Authorization Code + PKCE and separate workload clients.
- Caddy is the TLS ingress. PostgreSQL, Collector, Prometheus, Loki, and Tempo are private and never directly published.
- Redis and NATS are deliberately deferred. PostgreSQL remains the source of truth and durable outbox under ADR 0004.

A first Cloudflare prototype introduced a separate D1 data model, stored raw metrics/logs/spans in D1, and implemented a second OIDC/workload authorization layer. That duplicates the production API, diverges from the accepted relational model, drops the existing Collector/Prometheus/Loki/Tempo data plane, and weakens parity with the Go gateway. It must not become a second source of truth.

## Decision

### 1. Reuse the existing backend; do not fork it

Cloudflare is the public edge entry point for the Cloudflare dashboard mode. The existing Go API remains the API implementation and the only authorization authority. PostgreSQL remains authoritative. No D1 copy of workspace, alert, incident, notification, SLO, audit, or telemetry records is created.

### 2. Cloudflare Worker is a narrow API edge gateway

The `orbit-runtime` Worker:

- accepts only same-origin browser API requests from the GitHub Pages origin;
- routes only `/api/v1/*`, `/health/live`, and `/health/ready`;
- forwards the user's Bearer access token unchanged to the Go API;
- uses a private `ORBIT_GATEWAY_SECRET` to authenticate Worker-to-Caddy traffic;
- generates a request ID and forwards Cloudflare's edge-derived client IP through a dedicated header;
- does not store user/workload tokens, session state, telemetry payloads, or application records;
- does not accept arbitrary target URLs, arbitrary paths, or public webhook destinations;
- does not cache authenticated API responses.

The Worker does not duplicate OIDC/JWKS verification or route-role logic. The Go API remains authoritative for both.

### 3. Protect the API origin from bypass

The API hostname on Caddy is an origin endpoint, not the browser API URL. Caddy serves `/health/live` for liveness, but requires the secret Worker header for `/api/v1/*` and `/health/ready`. Caddy removes the gateway secret before forwarding to Go. The Worker secret and the VM-side secret are generated independently of source control and stored only in secret files/bindings.

The Worker replaces any client-supplied gateway/IP headers. Caddy forwards the verified edge IP as `X-ORBIT-Client-IP`; the Go API uses it for shared IP rate limiting. A direct request to the API origin without the Worker secret is rejected before reaching the Go API.

### 4. Keep the original telemetry ingress and all signal semantics

The Worker is not an OTLP or gRPC proxy. Workloads use the existing dedicated TLS hostnames:

- OTLP/HTTP → Caddy → Go authenticated HTTP workload gateway → private Collector.
- OTLP/gRPC → Caddy HTTP/2 → Go authenticated gRPC workload gateway → private Collector.

The Go gateway continues to enforce the strict workload audience, explicit workspace selector (HTTP path or gRPC metadata), workspace/signal grant, reserved-attribute rejection, canonical tenant attribution, allowlist, shared PostgreSQL quotas, bounded concurrency, payload limits, timeout, and backpressure. Collector receivers and telemetry backend APIs remain on the internal network only.

### 5. Preserve PostgreSQL outbox and defer unneeded services

Do not use Workers Queues, D1, Redis, or NATS to replace the accepted PostgreSQL notification outbox, evaluator lock, shared rate limits, or durable history. ADR 0004's measurement gates remain the only way to revisit Redis/NATS.

### 6. Free-tier and cost boundary

The Worker is a thin streaming proxy and is expected to stay within the Workers Free request/CPU limits at the current small portfolio usage. Do not adopt Cloudflare Containers, Workers Paid, Hyperdrive, D1, R2, or paid add-ons for this architecture. The existing zero-cost OCI Always Free design remains the origin compute/data plane. If an actual free allowance is unavailable, leave the Cloudflare edge in a not-configured/503 state; do not enable billing or weaken security.

## Trust boundaries

| Boundary | Required trust |
| --- | --- |
| Browser → Worker | Exact Pages origin CORS; user access token still required by Go for protected API routes |
| Worker → Caddy API origin | HTTPS plus 256-bit shared gateway secret |
| Caddy → Go API | Private container network; gateway secret stripped; verified edge IP forwarded |
| Browser → Keycloak | Existing HTTPS Authorization Code + PKCE; tokens kept in memory |
| Workload → OTLP gateway | Separate short-lived `at+jwt` workload token and dedicated ingestion audience |
| OTLP gateway → Collector | Private internal network only |
| Collector → Prometheus/Loki/Tempo | Private telemetry network; backend write/query APIs unpublished |

## Required verification

- Worker unit tests cover path allowlisting, exact-origin CORS, upstream origin validation, request-ID replacement, gateway-secret injection, IP-header replacement, no-cookie/no-cache behavior, fail-closed configuration, and safe errors.
- Caddy validation proves API requests without the gateway secret are rejected and that the secret is removed before Go.
- Go tests verify the forwarded client IP parsing and fallback behavior.
- Existing Go/PostgreSQL CI remains authoritative for API auth/roles, workspace isolation, durable outbox concurrency, webhook SSRF protection, SLO semantics, and audit immutability.
- Existing observability E2E remains authoritative for OTLP/HTTP + gRPC, workspace A/B write-to-query isolation, Prometheus scrape, Collector routing, and PostgreSQL recovery.
- Deployment acceptance still requires a real origin, Keycloak PKCE smoke, API source connection through the Worker, full workspace/role tests, HTTP+gRPC ingestion, and a clean A/B tenant-isolation smoke.
