# M06 OTLP gateway load-testing baseline

This guide provides a repeatable in-process baseline for the authenticated OTLP gateway paths. It is a development and regression aid, **not a production capacity certification**.

## What is measured

The HTTP and gRPC cases exercise the gateway handler path:

- workload bearer parsing and token-verifier call;
- workspace/signal grant authorization call;
- shared rate-limit interface call;
- OTLP payload validation and canonical workspace attribution;
- bounded forwarding and response handling.

The sustained tests execute 512 concurrent metrics exports per transport (16 workers × 32 exports), assert that every request is forwarded successfully, and verify that concurrent forwarding remains inside the configured gateway bound.

The benchmarks report throughput, bytes processed, allocations, and time per operation. Run them on the same machine, Go version, and GOMAXPROCS value when comparing revisions.

## Run the regression tests

From the repository root:

```sh
cd server
go test ./internal/ingest -run 'Test(HTTP|GRPC)GatewaySustainedConcurrentMetricsExports$' -count=5
go test -race ./internal/ingest
```

## Run the benchmarks

From the repository root:

```sh
cd server
go test ./internal/ingest -run '^$' -bench 'Benchmark(HTTP|GRPC)GatewayMetricsHandlerPath$' -benchmem -count=5
```

For a longer local sample, add -benchtime=5s. Keep the raw output with the Go version, OS/architecture, CPU model, GOMAXPROCS, and commit SHA. Compare repeated runs rather than treating one result as a pass/fail capacity number.

## Test boundaries

The tests use an in-memory token verifier, an in-memory grant repository, a deterministic shared-rate-limit stub, and a local forwarding stub. A short bounded delay is used only by the sustained tests to create overlapping exports. Benchmarks omit that delay so the measured time is dominated by gateway processing.

These tests do **not** exercise PostgreSQL connection-pool contention, real shared rate-limit SQL, a networked Collector, TLS termination, multiple gateway processes, the host kernel, or production telemetry backends. They therefore cannot establish production throughput or p95/p99 latency.

## Remaining production-load gate

Before enabling tenant-facing production ingestion, run a separate environment-level workload against the intended PostgreSQL and Collector topology. Record:

- sustained exports/second and p50/p95/p99 end-to-end latency by transport and signal;
- HTTP 429/503/504 and gRPC ResourceExhausted/Unavailable/DeadlineExceeded rates;
- gateway CPU, memory, goroutines, GC pauses, and admission-capacity saturation;
- PostgreSQL pool wait time, active/idle connections, and shared rate-limit query latency;
- Collector queue occupancy, refused/failed exports, and recovery after dependency interruption;
- behavior below, at, and above the configured admission and per-workload rate limits, including more than one gateway replica.

Do not turn noisy absolute benchmark numbers into a CI threshold until the target hardware and service-level objectives are explicitly defined. Production ingestion remains disabled until the deployment and operational gates in the [M06 roadmap](roadmap.md) are complete.


## Real PostgreSQL + Collector backpressure regression

The observability Compose profile also runs `TestWorkspaceOTLPSustainedBackpressureAndRecovery` against the CI PostgreSQL service and the live local OTLP Collector. A deterministic in-process HTTP gate briefly holds already-authorized exports at the Collector network boundary; it does not replace or mock the Collector. The test fills all 64 HTTP gateway admission slots, verifies eight additional requests fail fast with HTTP 503 without reaching the Collector, releases the gate, and verifies all admitted exports succeed. It then sends three further concurrent recovery waves (16 exports each) through the same PostgreSQL-backed authorization/rate-limit repository and the same real Collector forwarder.

This is an integration regression for bounded admission and recovery after transient downstream backpressure. It is not a production capacity result: the test uses one gateway process, CI-sized PostgreSQL and Collector services, a controlled local delay proxy, and a short workload. It does not measure target-environment p95/p99 latency, sustained production throughput, or multi-replica behavior.

The steady phase deliberately stays well below the configured shared 10,000-requests/minute test limit. Each producer has its own service series and emits fresh monotonically increasing points, avoiding out-of-order samples across concurrent workers.


## Observed CI result — PR #110

PR #110 was squash-merged into `main` as `b42b4b56a82a2f977b373731ae6e85a1ceb0760e`. Exact PR head `e00b0308bf0bf3d61e095975ee69546e349d0c43` passed CI run [41580189083](https://github.com/pooya16pg-spec/orbit/actions/runs/41580189083) and Security run [41580189031](https://github.com/pooya16pg-spec/orbit/actions/runs/41580189031).

The new test logged **639 successful exports over its 20-second sustained phase** (about 32 successful exports/second in this CI topology). The complete real-dependency workspace smoke command, including the pre-existing workspace A/B write-to-query test, passed in 66.18 seconds. The 64-slot admission saturation, eight HTTP 503 overflows, 64 admitted forwards, and three 16-export recovery waves all passed before the steady phase.

These are observed results from a single gateway process and CI-sized PostgreSQL/Collector services. The ~32 exports/second figure is not a target capacity or SLO claim. Do not infer p50/p95/p99, production throughput, multi-replica behavior, or host/network recovery from this CI run. Keep the target-environment load and production rollout gates open until evidence is collected on the intended deployment topology.


## Real PostgreSQL container restart and same-pool recovery

The CI observability stack now includes a coordinated restart regression. A Go integration test opens the repository against the actual Compose PostgreSQL service, warms and releases a pooled connection, and signals the runner through a shared temporary control directory. The runner stops the PostgreSQL container and signals completion. The same test must observe a failed pool ping; only then does the runner start the same Compose service again. The test waits for a successful query on the original repository/pool and verifies `pg_postmaster_start_time()` advanced across the restart.

This verifies a real PostgreSQL process/container stop and start, not only a TCP proxy disconnect. It is still a single-node, clean-stop/restart CI test: it does not establish crash recovery after SIGKILL/power loss, disk failure, failover/replication, multi-zone network partition, or a production RPO/RTO.


## Observed PostgreSQL container restart result — PR #111

PR #111 was squash-merged as `5b3c0fc344f043640b01c670a84deb6c78d09a5c`. Exact PR head `ac70f81103ced4e9c5ef5e4968f35ccd3115cce1` passed CI run [41581322810](https://github.com/pooya16pg-spec/orbit/actions/runs/41581322810) and Security run [41581322826](https://github.com/pooya16pg-spec/orbit/actions/runs/41581322826).

The CI runner stopped the actual Compose PostgreSQL container and started it again. The test confirmed the original Repository/pgxpool saw the outage and recovered without being replaced. PostgreSQL `pg_postmaster_start_time()` advanced from `2026-11-03T05:07:11.802744Z` to `2026-11-03T05:10:29.836093Z`; the test passed in 1.094 seconds.

This is a clean single-node container restart test, not crash or disk recovery, HA failover, replication, PITR, or production RPO/RTO evidence.


## PostgreSQL crash recovery — merged PR #112

The existing restart smoke now runs two sequential failure phases through the same Repository/pgxpool instance:

1. A normal Compose `stop` followed by `start`, preserving the already-verified clean shutdown recovery case.
2. An ungraceful `SIGKILL` of the actual PostgreSQL container followed by `start` of the same container with the same persistent data volume. The test requires the original pool to observe the outage, reconnect, and query PostgreSQL after crash/WAL recovery.

Each phase checks that `pg_postmaster_start_time()` advances, so a pre-existing healthy connection cannot make the assertion pass. The test also commits a unique probe row before either restart and reads the same payload back after both phases, validating survival of committed data through PostgreSQL WAL crash recovery. The runner cleanup still attempts to start PostgreSQL even when the integration test fails.

A process SIGKILL with a persistent Docker volume is a useful crash-recovery regression, but it is not equivalent to host power loss, torn/corrupt storage, disk failure, replication failover, network partition, or a measured production RPO/RTO.


## Observed CI result — PR #112 PostgreSQL crash/WAL recovery

PR #112 was squash-merged into `main` as `15fc476f422afbc632542e292d1c17d9a0af2796`. Exact PR head `7fb08afb6b312d3c3878fb4011021e39d5b14f51` passed CI run [41596787239](https://github.com/pooya16pg-spec/orbit/actions/runs/41596787239) and Security run [41596787190](https://github.com/pooya16pg-spec/orbit/actions/runs/41596787190).

Observed postmaster starts:
- Before/after graceful stop-start: `2026-11-03T07:35:45.535327Z` → `2026-11-03T07:38:44.698761Z`.
- After graceful restart / after SIGKILL restart: `2026-11-03T07:38:44.698761Z` → `2026-11-03T07:38:45.596894Z`.

The original Repository/pgxpool recovered in both phases. A committed probe record was read back unchanged after each restart. Both subtests passed in 2.768 seconds total. This is an actual single-node process SIGKILL plus persistent-volume startup/WAL recovery test; it is not host power-loss, storage corruption/disk loss, HA failover, PITR, or production RPO/RTO evidence.
