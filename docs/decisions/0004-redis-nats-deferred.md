# ADR 0004 — Keep PostgreSQL authoritative; defer Redis cache and NATS event publishing

- Status: Accepted
- Date: 2026-11-03
- Scope: API caching, alert notification delivery, and application event distribution

## Context

The local Compose stack provisions Redis and NATS JetStream, but the Go API does not currently depend on either service. The architecture roadmap says to wire them only where justified. Adding services because they already exist in Compose is not, by itself, a product or operational requirement.

Current application behavior:

- PostgreSQL is the authoritative store for workspace membership, workload grants, alert rules and instances, incident timelines, audit history, deployment annotations, SLOs, notification delivery rows, and immutable notification attempt history.
- The alert notification path uses a transactional PostgreSQL outbox. Alert lifecycle changes and notification delivery rows are committed in the same database transaction.
- Each API replica runs a notification worker. It polls every five seconds, claims bounded batches with `FOR UPDATE SKIP LOCKED`, and writes a time-limited lease before sending. Expired leases are recorded as attempts before redelivery.
- Delivery retries are bounded to eight attempts with capped exponential backoff. The HTTPS webhook sender uses an explicit host allowlist, public-unicast DNS validation, redirects disabled, bounded timeouts, and the durable delivery ID as an idempotency key.
- A separate alert evaluator uses a PostgreSQL advisory lock so multiple API replicas do not evaluate the same rules concurrently.
- The shared API/workload rate limiter is PostgreSQL-backed and provides atomic cross-replica state. It is not a disposable cache.

## Decision

**Do not add Redis caching or NATS publishing to the API in this milestone.** Keep PostgreSQL as the source of truth and as the durable notification queue. Keep Redis and NATS as optional local Compose services only.

### Redis

No current API read path has an observed cache bottleneck, a defined freshness target, or a cache invalidation contract. The service catalog already uses bounded indexed PostgreSQL queries and cursor pagination. Caching workspace membership, grants, notification channel state, rate limits, or operator history would introduce stale authorization or consistency risk. Do not cache authorization or durable history.

Revisit Redis only after a measured database read bottleneck and a specific cacheable workload are demonstrated. A future proposal must name:
- the exact cached values and owners;
- freshness and invalidation semantics across replicas;
- behavior when Redis is unavailable or stale;
- memory bounds, eviction, tenant isolation, and operational ownership;
- a benchmark proving meaningful improvement over PostgreSQL.

### NATS JetStream

The current notification outbox has one durable consumer role: the HTTPS delivery worker. PostgreSQL polling provides at-least-once delivery with transactionally created queue rows, leases, retry history, and idempotency keys. There is no second independent consumer requiring fan-out, replay, or stream retention.

Publishing directly to NATS after a database commit would create a dual-write gap: the database transaction can commit while the publish fails, or the publish can succeed while the caller observes a database error. If a future requirement needs NATS, first introduce a transactional event outbox in PostgreSQL, then publish with stable event IDs and idempotent JetStream consumers. Define replay, retention, dead-letter handling, ordering, backpressure, and recovery before making NATS part of the production path.

Revisit NATS only when measured polling latency is insufficient or at least one additional independently operated consumer needs durable fan-out/replay. Do not replace the PostgreSQL outbox or immutable attempt history with ephemeral messages.

## Consequences

- No extra cache invalidation, broker, credentials, monitoring, or failure mode is added to the current API.
- The five-second worker polling interval is an explicit current latency trade-off; it is not claimed to be sub-second event delivery.
- PostgreSQL remains a required durable dependency for queued notification attempts and shared authorization/rate-limit state.
- The M03 roadmap item stays open and deferred rather than being marked complete without an implementation. Revisit it when a measured trigger above is met.

## Verification to keep in place

The PostgreSQL integration suite must prove that separate repository pools can claim the same outbox concurrently without duplicate ownership. Retry/lease-expiry and append-only attempt history tests remain required. The existing CI Postgres service is suitable for repository-level concurrency regression, but is not a substitute for production-scale broker/cache benchmarks.
