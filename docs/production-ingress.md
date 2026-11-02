# Production OTLP ingress reference

This is an **optional edge-ingress scaffold**, not a declaration that ORBIT is production-ready. It terminates public TLS at Caddy and forwards OTLP to the authenticated ORBIT gateways over a private Docker bridge network.

## Topology and exposure

- Caddy is the only service in the `ingress` network that publishes host ports: TCP 80/443 and UDP 443 (HTTP/3).
- ORBIT API joins `ingress`, the existing default application network, and `telemetry-internal`. Its OTLP listeners bind inside the container on 4319/4320 but are not published on the host.
- Caddy joins only `ingress`; it cannot directly address PostgreSQL, Redis, NATS, Prometheus, Loki, Tempo, or the raw Collector receivers.
- Raw Collector OTLP ports 4317/4318 remain unpublished and attached only to `telemetry-internal`.
- Use separate DNS names for OTLP/HTTP and OTLP/gRPC. Caddy obtains and renews certificates automatically through ACME; public DNS must resolve to this host and inbound TCP 80/443 must be reachable. UDP 443 is optional for HTTP/3.

## Configure and validate

Supply these values through a protected deployment environment or secret manager; do not commit a populated `.env` file:

- `ORBIT_OTLP_HTTP_DOMAIN` and `ORBIT_OTLP_GRPC_DOMAIN`: real public DNS names.
- `ORBIT_OIDC_ISSUER` / `ORBIT_OIDC_AUDIENCE`: human API token issuer and audience.
- `ORBIT_WORKLOAD_OIDC_ISSUER` / `ORBIT_WORKLOAD_OIDC_AUDIENCE`: workload-token issuer and a distinct audience.
- `ORBIT_DATABASE_URL`: production database URL with a strong credential and an appropriate TLS mode for the selected database.
- `POSTGRES_PASSWORD`: strong password if using the Compose PostgreSQL service. Prefer a managed PostgreSQL service with TLS and managed backups for production.

Validate the merged Compose model before starting anything:

```sh
docker compose -f docker-compose.yml -f deploy/production/compose.ingress.yaml --profile observability --profile production-ingress config --quiet
```

Start only after DNS, firewall, identity-provider, database, backup, and secret-management prerequisites are complete:

```sh
docker compose -f docker-compose.yml -f deploy/production/compose.ingress.yaml --profile observability --profile production-ingress up -d postgres api prometheus loki tempo otel-collector caddy
```

The Compose overlay sets `ORBIT_ENVIRONMENT=production`; the API therefore refuses to start unless both human and workload OIDC, distinct audiences, and PostgreSQL are configured. The API still uses its local development Compose dependencies unless deliberately replaced/overridden; do not use the repository's local-only database credentials in a public deployment.

## Important limitations / go-live gates

- This edge proxy provides TLS termination; the API-to-Caddy hop is private-network plaintext. Use a trusted host, restrict Docker socket access, and assess whether your threat model requires mTLS on the internal hop.
- Caddy is not an identity provider. Workload JWT verification, workspace grants, signal permissions, and shared rate limits remain enforced by ORBIT.
- The dashboard/API's human OIDC provider integration and browser PKCE rollout must be tested against the real provider. Do not assume this OTLP ingress overlay configures browser login.
- Configure host firewall rules to expose only TCP 80/443 and optional UDP 443. Never publish API OTLP ports 4319/4320, raw Collector ports 4317/4318, or backend ports.
- Add deployment-specific monitoring, certificate renewal alerts, restore drills, secret/key rotation, managed retention/cardinality limits with per-workspace budgets, and incident runbooks before enabling tenant traffic. The local Compose budgets in [the retention policy](observability-retention.md) are single-tenant development defaults and must not be treated as production quotas.
- This scaffold does not implement Kubernetes NetworkPolicy, cloud load-balancer policy, mTLS, or production secret delivery. Those remain deployment-specific controls.

Production tenant ingestion must remain disabled until every go-live gate in [the M06 roadmap](../roadmap.md) is verified.
