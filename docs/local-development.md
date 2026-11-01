# Local Development

## Requirements

- Windows 10/11, macOS, or Linux
- Git
- Node.js 20+ and npm
- Docker Desktop (for the backend stack)
- Go 1.24+ (API); Docker builds the Go 1.25 telemetry demo

## Clone the repository

```powershell
git clone https://github.com/pooya16pg-spec/orbit.git
cd orbit
```

## Run the web workspace

```powershell
npm install
npm run dev
```

Open http://localhost:3000. The local development server uses the root path; GitHub Pages uses the repository base path automatically during its deployment workflow. In the dashboard, choose **Connect a source**, enter `http://localhost:8080`, and select **Test and connect** to read local API data.

## Run the backend and local infrastructure

Before starting the API, choose an explicit runtime mode. The supplied Compose file sets `ORBIT_ENVIRONMENT=development`; running the API directly requires setting it yourself. Development can omit OIDC, but if either OIDC variable is set, both must be set and a successful discovery/JWKS load is required.

With Docker Desktop running:

```powershell
docker compose up --build
```

The first API startup runs the versioned SQL migrations and adds sample service and incident rows. Migration `002_telemetry_service_catalog` creates the normalized observed-service and source tables. The records are marked `demo_seed`.

Useful endpoints (service and telemetry data routes are public only in explicit local development mode):

- http://localhost:8080/health/live
- http://localhost:8080/health/ready
- http://localhost:8080/api/v1/meta
- http://localhost:8080/api/v1/services
- http://localhost:8080/api/v1/incidents
- http://localhost:8080/api/v1/incidents?status=investigating

PostgreSQL is exposed on localhost:5432, Redis on localhost:6379, and NATS on localhost:4222 (monitoring endpoint localhost:8222). These are local development ports, not production-secured services.

Stop the containers:

```powershell
docker compose down
```

Delete container data too (destructive):

```powershell
docker compose down -v
```

## Start the optional observability backends

The Collector, Prometheus, Loki, and Tempo are opt-in so a normal development session stays lightweight:

```powershell
docker compose --profile observability up -d postgres api prometheus loki tempo otel-collector telemetry-demo
```

| Service | Local endpoint |
| --- | --- |
| OTLP/HTTP receiver | http://localhost:4318 |
| OTLP/gRPC receiver | localhost:4317 |
| Collector health | http://localhost:13133 |
| Prometheus | http://localhost:9090 |
| Loki readiness | http://localhost:3100/ready |
| Tempo readiness | http://localhost:3200/ready |

The `telemetry-demo` workload is instrumented using the OpenTelemetry Go SDK. Send a sample request with `curl.exe http://localhost:8081/emit`; it returns a trace ID and emits a metric, trace, and structured log. Query metrics in Prometheus at http://localhost:9090, logs in Loki at http://localhost:3100, and traces in Tempo at http://localhost:3200. The Collector routes OTLP metrics to Prometheus, traces to Tempo, and logs to Loki. The ORBIT API container is configured to reach those backends over the Compose network. All ports bind to loopback and the backends have no authentication; do not expose them outside your development machine. Prometheus and Loki retain seven days of data in named volumes. Stop these services with `docker compose stop postgres api prometheus loki tempo otel-collector telemetry-demo`.

The dedicated demo workload is instrumented and its three OTLP signal paths are covered by CI. The ORBIT API provides read-only, bounded queries for all three stores and discovers service names from Prometheus, Loki, and Tempo. The Metrics, Logs, and Traces views show those discoveries in a shared service selector. The production Go API is not instrumented yet. The catalog currently stores discovered names and backend provenance; its JSONB resource-attribute field is ready for richer instrumentation metadata.

### Query telemetry through the ORBIT API

After starting the observability profile and generating a sample with `/emit`, query the same signals through ORBIT rather than calling each backend directly:

```powershell
Invoke-RestMethod "http://localhost:8080/api/v1/telemetry/services?window=1h"
Invoke-RestMethod "http://localhost:8080/api/v1/telemetry/metrics?window=1h&service=orbit-telemetry-demo"
Invoke-RestMethod "http://localhost:8080/api/v1/telemetry/logs?service=orbit-telemetry-demo&window=1h&limit=50"
Invoke-RestMethod "http://localhost:8080/api/v1/telemetry/traces?service=orbit-telemetry-demo&window=1h&limit=20"
```

The service discovery endpoint merges names from all configured stores, upserts observations into PostgreSQL, and returns the persisted active catalog with first-seen, last-seen, and last-discovered timestamps. If one store is temporarily unavailable, the API returns persisted services and a warning. If all stores are unavailable, the API falls back to the last known PostgreSQL catalog when it contains services. Allowed windows are `15m`, `1h`, `6h`, and `24h`; service names are validated and query results are capped at 100. The request-rate query currently uses the instrumented demo counter and does not claim to be a generalized PromQL proxy.

## Connect the local dashboard to the API

With `docker compose up --build` running, open the dashboard at `http://localhost:3000` and choose **Connect a source**. Enter `http://localhost:8080` and click **Test and connect**. ORBIT verifies `/api/v1/meta`, `/api/v1/services`, and `/api/v1/incidents` before switching the dashboard to API mode. The URL is saved in browser local storage and can be changed from the same source panel.

The Go API allows the GitHub Pages origin and local development origins through `ORBIT_ALLOWED_ORIGINS`. If you use a different dashboard origin, add that exact origin to the allowlist and restart the API. After connecting the local dashboard to the local API, select **Metrics**, **Logs**, or **Traces** in the navigation. A shared service selector is populated from the persisted service catalog and shows source provenance. Each view queries its corresponding backend through the ORBIT API, supports 15m/1h/6h/24h windows, refreshes every 30 seconds, and shows the backend source and last query time. The Overview page remains focused on service and incident inventory. If you open the GitHub Pages website instead of the local frontend, you need a separately hosted HTTPS API that permits the Pages origin; `localhost:8080` on your computer is not reachable by the hosted site.


With an OIDC-enabled API, send a verified access token explicitly when calling protected endpoints:

```powershell
$headers = @{ Authorization = "Bearer $env:ORBIT_ACCESS_TOKEN" }
Invoke-RestMethod -Headers $headers "http://localhost:8080/api/v1/services"
Invoke-RestMethod -Headers $headers "http://localhost:8080/api/v1/telemetry/services?window=1h"
```

## Run Go tests directly

```powershell
cd server
go mod tidy
go test -race ./...
go vet ./...
go run ./cmd/orbit-api
```

Set `ORBIT_ENVIRONMENT=development` explicitly and configure `ORBIT_DATABASE_URL` before running the API outside Docker. Example:

```powershell
$env:ORBIT_ENVIRONMENT = "development"
$env:ORBIT_DATABASE_URL = "postgres://orbit:orbit_local_only@localhost:5432/orbit?sslmode=disable"
go run ./cmd/orbit-api
```

## Environment, authentication and secrets

- `.env.example` contains local-only defaults and documents the optional issuer/audience settings.
- Set `ORBIT_ENVIRONMENT=development` explicitly for a local API without authentication. Do not rely on an implicit default.
- To enable OIDC in development, set both `ORBIT_OIDC_ISSUER` and `ORBIT_OIDC_AUDIENCE`. The issuer must publish a matching `/.well-known/openid-configuration` document and a reachable `jwks_uri`. `ORBIT_OIDC_ROLES_CLAIM` defaults to `roles`; supported values are `viewer`, `operator`, and `administrator`.
- Staging and production require both OIDC settings and fail startup if discovery/JWKS validation fails. API data routes then require `Authorization: Bearer <access-token>`; the token must target the configured API audience. The public `/health/live` and `/api/v1/meta` endpoints remain unauthenticated.
- The dashboard does not yet implement OIDC PKCE, so do not connect the static frontend to an auth-protected API until the browser-authentication milestone is implemented.
- `ORBIT_RATE_LIMIT_PER_MINUTE` defaults to 120 per IP and authenticated subject, per API process. This is a development safety limit, not a distributed production quota.
- Compose currently has a deliberately local development password. Change all credentials before deploying any service outside localhost.
- Never commit `.env.local`, production secrets, API keys, or real telemetry payloads.
- The public GitHub Pages dashboard is frontend-only; it does not tunnel to a developer's local API.

## Troubleshooting

### Port 3000 is in use

```powershell
npm run dev -- --port 3001
```

### Rebuild the backend after changing Go code

```powershell
docker compose up --build api
```

### Reset local database data

This deletes all local Postgres and Redis volume data:

```powershell
docker compose down -v
docker compose up --build
```
