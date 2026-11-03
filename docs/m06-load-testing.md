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
