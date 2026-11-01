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
