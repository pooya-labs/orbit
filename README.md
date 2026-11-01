# ORBIT

**Distributed Systems Intelligence & Observability**

ORBIT is an engineering-first observability platform for understanding the health, behavior, and reliability of distributed services.

> Status: early development. Metrics and events shown by the initial interface are clearly marked as demo data until connected to real telemetry sources.

## Product direction

- Service topology and dependency health
- Metrics, traces, and structured logs
- Incident timelines and alert policies
- Deployment markers and release health
- OpenTelemetry-native ingestion

## Architecture (planned)

| Layer | Technology |
| --- | --- |
| Web application | Next.js · TypeScript |
| API and ingestion | Go |
| Persistence | PostgreSQL |
| Cache and ephemeral state | Redis |
| Event transport | NATS |
| Telemetry | OpenTelemetry · Prometheus · Loki · Tempo |
| Local infrastructure | Docker Compose |

## Development

The repository is being bootstrapped. The first milestone is the responsive observability workspace and its documented data contracts. Real ingestion and persistence will follow; the UI will never present fabricated telemetry as live production data.
