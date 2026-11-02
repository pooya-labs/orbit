#!/usr/bin/env python3
"""Check ORBIT's explicit local telemetry retention and cardinality guardrails.

This is a policy/CI regression check, not a YAML parser. The observability CI job
also starts the pinned Prometheus, Loki, Tempo, and Collector images, so their
native configuration parsers validate the actual YAML structure and field names.
"""
from __future__ import annotations

from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]


def read(relative: str) -> str:
    path = ROOT / relative
    if not path.is_file():
        raise SystemExit(f"missing required policy file: {relative}")
    return path.read_text(encoding="utf-8")


def require(text: str, pattern: str, label: str) -> None:
    if re.search(pattern, text, re.MULTILINE) is None:
        raise SystemExit(f"missing or changed retention/cardinality guardrail: {label}")


compose = read("docker-compose.yml")
prometheus = read("deploy/observability/prometheus.yml")
loki = read("deploy/observability/loki.yaml")
tempo = read("deploy/observability/tempo.yaml")
collector = read("deploy/observability/otel-collector.yaml")
postgres = read("server/internal/store/postgres/postgres.go")
rate_limit_migration = read("server/internal/store/postgres/migrations/004_shared_api_rate_limits.sql")

# Metrics: 7-day TSDB retention and a bounded scrape from the Collector exporter.
require(compose, r'"--storage\.tsdb\.retention\.time=7d"', "Prometheus 7-day TSDB retention")
collector_job = prometheus.split("  - job_name: orbit-otel-collector", 1)
if len(collector_job) != 2:
    raise SystemExit("missing Prometheus Collector scrape job")
collector_job = collector_job[1].split("\n  - job_name:", 1)[0]
for pattern, label in [
    (r"^\s+sample_limit:\s*20000\s*$", "Prometheus Collector sample_limit=20000"),
    (r"^\s+label_limit:\s*32\s*$", "Prometheus Collector label_limit=32"),
    (r"^\s+label_name_length_limit:\s*128\s*$", "Prometheus label-name length=128"),
    (r"^\s+label_value_length_limit:\s*256\s*$", "Prometheus label-value length=256"),
    (r"^\s+body_size_limit:\s*10MB\s*$", "Prometheus scrape body limit=10MB"),
]:
    require(collector_job, pattern, label)

# Logs: retention and global single-tenant stream/label/ingestion budgets.
for pattern, label in [
    (r"^\s+retention_enabled:\s*true\s*$", "Loki compactor retention enabled"),
    (r"^\s+retention_period:\s*168h\s*$", "Loki 168-hour retention"),
    (r"^\s+reject_old_samples:\s*true\s*$", "Loki rejects old samples"),
    (r"^\s+reject_old_samples_max_age:\s*168h\s*$", "Loki old-sample age=168h"),
    (r"^\s+ingestion_rate_strategy:\s*global\s*$", "Loki global ingestion rate strategy"),
    (r"^\s+ingestion_rate_mb:\s*4\s*$", "Loki ingestion rate=4 MB/s"),
    (r"^\s+ingestion_burst_size_mb:\s*8\s*$", "Loki ingestion burst=8 MB"),
    (r"^\s+max_label_name_length:\s*128\s*$", "Loki label-name length=128"),
    (r"^\s+max_label_value_length:\s*256\s*$", "Loki label-value length=256"),
    (r"^\s+max_label_names_per_series:\s*12\s*$", "Loki label count=12"),
    (r"^\s+max_global_streams_per_user:\s*5000\s*$", "Loki active stream cap=5000"),
    (r"^\s+max_line_size:\s*262144\s*$", "Loki log line cap=256 KiB"),
]:
    require(loki, pattern, label)

# Traces: retention is configured in Tempo 3.x's backend scheduler compaction job.
for pattern, label in [
    (r"^\s+interval:\s*1h\s*$", "Tempo retention sweep interval=1h"),
    (r"^\s+block_retention:\s*168h\s*$", "Tempo block retention=168h"),
    (r"^\s+compacted_block_retention:\s*1h\s*$", "Tempo compacted block retention=1h"),
    (r"^\s+max_block_bytes:\s*1073741824\s*$", "Tempo compacted block cap=1 GiB"),
    (r"^\s+max_attribute_bytes:\s*1024\s*$", "Tempo attribute cap=1 KiB"),
    (r"^\s+max_traces_per_user:\s*10000\s*$", "Tempo active trace cap=10000"),
    (r"^\s+max_bytes_per_trace:\s*5000000\s*$", "Tempo trace size cap=5 MB"),
]:
    require(tempo, pattern, label)

# Do not duplicate the existing Collector RAM/batch controls or PostgreSQL rate-window TTL.
for pattern, label in [
    (r"^\s+memory_limiter:\s*$", "Collector memory limiter"),
    (r"^\s+limit_mib:\s*256\s*$", "Collector memory limit=256 MiB"),
    (r"^\s+spike_limit_mib:\s*64\s*$", "Collector memory spike limit=64 MiB"),
    (r"^\s+batch:\s*$", "Collector batch processor"),
]:
    require(collector, pattern, label)

require(postgres, r"bucket_start\s*\+\s*INTERVAL '2 minutes'", "API rate-window expiry=2 minutes")
require(postgres, r"DELETE FROM api_rate_limit_windows WHERE expires_at <= NOW\(\)", "expired API rate-window pruning")
require(postgres, r"Unix\(\)\s*/\s*300", "rate-window cleanup cadence=5 minutes")
require(rate_limit_migration, r"ON api_rate_limit_windows\s*\(expires_at\)", "expiry index for API rate-window pruning")

print("PASS: 7-day metrics/logs/traces retention and explicit telemetry cardinality/size guardrails are present.")
print("PASS: existing Collector memory/batch limits and PostgreSQL ephemeral rate-window TTL/pruning remain intact.")
print("NOTE: this Compose profile is single-tenant; backend limits are not per-workspace quotas.")
print("NOTE: durable audit, incident, alert, notification and deployment histories are intentionally not TTL-pruned.")
