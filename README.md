# ORBIT

**Distributed Systems Intelligence & Observability**

ORBIT is an engineering-first observability platform for understanding the health, behavior, and reliability of distributed services.

> **Current stage:** Milestone 01 — responsive observability workspace. The dashboard uses clearly labelled illustrative demo data. No production systems are connected yet.

## What is being built

- Service inventory and dependency topology
- Metrics, distributed traces, and structured logs
- Incident timelines, alert policies, and SLO tracking
- Deployment markers and release health
- OpenTelemetry-native ingestion and service instrumentation

## Technology direction

| Layer | Technology |
| --- | --- |
| Web application | Next.js · React · TypeScript |
| API and ingestion | Go |
| Persistence | PostgreSQL |
| Cache and ephemeral state | Redis |
| Event transport | NATS |
| Telemetry pipeline | OpenTelemetry · Prometheus · Loki · Tempo |
| Local infrastructure | Docker Compose |

The web workspace is the first deliverable. Backend services will be introduced behind documented API contracts, then connected to real telemetry. **Demo values must never be presented as live production data.**

## Run locally on Windows

Requirements: Node.js 20 or newer and Git.

```powershell
git clone https://github.com/pooya16pg-spec/orbit.git
cd orbit
npm install
Copy-Item .env.example .env.local
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

Useful commands:

```powershell
npm run typecheck
npm run build
npm run start
```

## Repository map

```text
app/                  Next.js app router, dashboard, global styles
server/               Go API, configuration, HTTP handlers, PostgreSQL repository
server/.../migrations  Versioned SQL schema and explicit demo seed rows
docs/                 Architecture decisions and development notes
.github/workflows/    CI and GitHub Pages deployment
```

## Development principles

1. Real telemetry or explicitly labelled demo data — never ambiguous synthetic "live" data.
2. Keep ingestion, storage, querying, and presentation as separate concerns.
3. Prefer observable failure modes, typed contracts, and reproducible local infrastructure.
4. Design for desktop and mobile without sacrificing information density on large screens.

## Roadmap

See [docs/roadmap.md](docs/roadmap.md) for the staged implementation plan and [docs/architecture.md](docs/architecture.md) for the initial system boundaries.

## License

No license has been selected yet. All rights are reserved by default.
