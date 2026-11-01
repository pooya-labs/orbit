# ORBIT

**Distributed Systems Intelligence & Observability**

ORBIT is an engineering-first observability platform for understanding the health, behavior, and reliability of distributed services.

## Live preview

**[Open the ORBIT dashboard](https://pooya16pg-spec.github.io/orbit/)**

The public preview is a static, responsive workspace. Its metrics, service catalogue, and incident timeline are explicitly labelled illustrative demo data; no production systems are connected.

## Current implementation

- Responsive Next.js + TypeScript observability workspace.
- Go HTTP API with liveness, readiness, metadata, service inventory, and incident endpoints.
- PostgreSQL repository with a versioned initial migration and clearly marked seed data.
- Docker Compose stack for PostgreSQL, Redis, NATS, and the Go API.
- GitHub Actions for tests, static build, and GitHub Pages deployment.

## Technology

| Layer | Technology |
| --- | --- |
| Web application | Next.js · React · TypeScript |
| API | Go 1.24+ |
| Persistence | PostgreSQL 17 |
| Cache | Redis 7 (stack provisioned; application integration pending) |
| Event transport | NATS JetStream (stack provisioned; application integration pending) |
| Telemetry direction | OpenTelemetry · Prometheus · Loki · Tempo |
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

GitHub Actions runs the web checks and Go tests against a real PostgreSQL service container.

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

The v0.1.0 public preview is the first dashboard release. The v0.2.0 backend foundation adds PostgreSQL-backed services/incidents, an OpenAPI contract, a typed TypeScript client, and a local Docker Compose stack. Live telemetry ingestion, authentication, and hosted backend deployment are not yet implemented.

## License

No open-source license has been selected. All rights are reserved by default.
