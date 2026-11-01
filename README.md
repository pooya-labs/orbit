# ORBIT

**Distributed Systems Intelligence & Observability**

ORBIT is an engineering-first observability platform for understanding the health, behavior, and reliability of distributed services.

## Live preview

**[Open the ORBIT dashboard](https://pooya16pg-spec.github.io/orbit/)**

The public preview is a static, responsive workspace. Demo mode uses clearly labelled illustrative data; API mode can read service and incident inventories from an explicitly configured ORBIT API. No production systems are connected by default.

## Current implementation

- Responsive Next.js + TypeScript observability workspace with an explicit demo/API source switch.
- The dashboard can read service and incident inventories from a configured ORBIT API URL; unsupported aggregate metrics are not presented as live data.
- Go HTTP API with liveness, readiness, metadata, service inventory, and incident endpoints.
- PostgreSQL repository with a versioned initial migration and clearly marked seed data.
- Docker Compose stack for PostgreSQL, Redis, NATS, and the Go API.
- Optional local OpenTelemetry Collector → Prometheus, Tempo, and Loki pipeline; dashboard/API integration is still a later milestone.
- GitHub Actions for tests, static build, and GitHub Pages deployment.

## Technology

| Layer | Technology |
| --- | --- |
| Web application | Next.js · React · TypeScript |
| API | Go 1.24+ |
| Persistence | PostgreSQL 17 |
| Cache | Redis 7 (stack provisioned; application integration pending) |
| Event transport | NATS JetStream (stack provisioned; application integration pending) |
| Local telemetry pipeline | OpenTelemetry Collector · Prometheus · Loki · Tempo (opt-in Compose profile) |
| CI/CD | GitHub Actions · GitHub Pages · Docker Compose |

## Run locally on Windows

Requirements: Git, Node.js 20+, and npm.

```powershell
git clone https://github.com/pooya16pg-spec/orbit.git
cd orbit
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

## Connect the dashboard to the local API

For a local API, run the dashboard locally with `npm run dev` and run the backend with Docker Compose. Open `http://localhost:3000`, choose **Connect a source**, enter `http://localhost:8080`, then select **Test and connect**. The dashboard verifies metadata and reads services and incidents. The URL is stored only in this browser. The public GitHub Pages site does not host the Go API; connecting that hosted site requires a separately hosted HTTPS API whose CORS allowlist includes the Pages origin.

When connected, the dashboard hides illustrative metric cards and charts because the current API does not expose time-series telemetry yet. Demo mode remains available at any time.

## Run the backend stack

Install Docker Desktop, then from the repository root:

```powershell
docker compose up --build
```

This starts PostgreSQL, Redis, NATS, and the Go API. On its first start the API applies the initial SQL migration and inserts five sample services and two sample incidents, each marked with source `demo_seed`.

| Endpoint | Purpose |
| --- | --- |
| [http://localhost:8080/health/live](http://localhost:8080/health/live) | Process liveness |
| [http://localhost:8080/health/ready](http://localhost:8080/health/ready) | PostgreSQL readiness |
| [http://localhost:8080/api/v1/meta](http://localhost:8080/api/v1/meta) | API metadata |
| [http://localhost:8080/api/v1/services](http://localhost:8080/api/v1/services) | Service inventory |
| [http://localhost:8080/api/v1/incidents](http://localhost:8080/api/v1/incidents) | Incident inventory |
| [http://localhost:8080/api/v1/incidents?status=investigating](http://localhost:8080/api/v1/incidents?status=investigating) | Filter incidents |

Stop containers with `docker compose down`. To delete local database/cache volumes too, use `docker compose down -v` (this permanently deletes their local data).

## Start the local observability stack

The telemetry backends are optional and do not start during a normal `docker compose up`:

```powershell
docker compose --profile observability up -d prometheus loki tempo otel-collector
```

Send OTLP from an instrumented workload to `http://localhost:4318` (HTTP) or `localhost:4317` (gRPC). The Collector routes metrics to Prometheus, traces to Tempo, and logs to Loki. Local query interfaces: [Prometheus](http://localhost:9090), [Loki](http://localhost:3100/ready), and [Tempo](http://localhost:3200/ready). Collector health: [http://localhost:13133](http://localhost:13133).

These development services bind to loopback and have no authentication. Do not expose their ports to a public network. Prometheus keeps seven days of metrics and Loki is configured for seven-day log retention; data volumes persist until removed. Stop only these services with `docker compose stop prometheus loki tempo otel-collector`.

The collector and storage backends are now provisioned, but the Go API does not yet export telemetry and the ORBIT dashboard does not yet query these backends. The next milestones add an instrumented sample workload and connect observed telemetry to the product.

## Quality checks

```powershell
npm run typecheck
npm run build
cd server
go mod tidy
go test -race ./...
go vet ./...
go build ./cmd/orbit-api
```

GitHub Actions runs web typecheck/build, Go tests against a real PostgreSQL service container, and a Docker Compose smoke test for the optional local telemetry backends.

## Repository map

```text
app/                         Next.js dashboard and design system
server/cmd/orbit-api/        Go API entry point
server/internal/httpapi/     HTTP routes and middleware
server/internal/store/       API read models and repository contract
server/internal/store/postgres/ PostgreSQL implementation and migrations
docs/                        Architecture, roadmap, local development
.github/workflows/            CI and Pages deployment
```

## Roadmap

See [docs/roadmap.md](docs/roadmap.md), [docs/architecture.md](docs/architecture.md), and the [OpenAPI 3.1 contract](docs/openapi.yaml). The TypeScript API client and contracts live in `lib/orbit/`.

## Release status

The v0.1.0 public preview introduced the dashboard. The v0.2.0 backend foundation added PostgreSQL-backed service/incident APIs, OpenAPI, a typed client, and Docker Compose. The v0.3.0 preview added a verified browser source switch between demo fixtures and a configured ORBIT API. The v0.4.0 preview adds an optional local OTLP pipeline backed by Prometheus, Loki, and Tempo. The Go API is not instrumented yet, the dashboard does not query the telemetry stores, and authentication/hosted backend deployment remain future milestones.

## License

No open-source license has been selected. All rights are reserved by default.
