# Local Development

## Requirements

- Windows 10/11, macOS, or Linux
- Git
- Node.js 20+ and npm
- Docker Desktop (for the backend stack)
- Go 1.24+ (only if running the API directly)

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

With Docker Desktop running:

```powershell
docker compose up --build
```

The first API startup runs the initial versioned SQL migration and adds sample service and incident rows. The records are marked `demo_seed`.

Useful endpoints:

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

## Connect the local dashboard to the API

With `docker compose up --build` running, open the dashboard at `http://localhost:3000` and choose **Connect a source**. Enter `http://localhost:8080` and click **Test and connect**. ORBIT verifies `/api/v1/meta`, `/api/v1/services`, and `/api/v1/incidents` before switching the dashboard to API mode. The URL is saved in browser local storage and can be changed from the same source panel.

The Go API allows the GitHub Pages origin and local development origins through `ORBIT_ALLOWED_ORIGINS`. If you use a different dashboard origin, add that exact origin to the allowlist and restart the API. The current API has no time-series metrics endpoint, so the dashboard deliberately hides the sample metric cards while API mode is active.

## Run Go tests directly

```powershell
cd server
go mod tidy
go test -race ./...
go vet ./...
go run ./cmd/orbit-api
```

Set `ORBIT_DATABASE_URL` before running the API outside Docker. Example:

```powershell
$env:ORBIT_DATABASE_URL = "postgres://orbit:orbit_local_only@localhost:5432/orbit?sslmode=disable"
go run ./cmd/orbit-api
```

## Environment and secrets

- `.env.example` contains local-only defaults.
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
