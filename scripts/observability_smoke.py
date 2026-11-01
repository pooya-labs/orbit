#!/usr/bin/env python3
"""End-to-end smoke test for ORBIT's local OTLP pipeline."""

from __future__ import annotations

import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

DEMO_URL = "http://telemetry-demo:8081"
PROMETHEUS_URL = "http://prometheus:9090"
LOKI_URL = "http://loki:3100"
TEMPO_URL = "http://tempo:3200"
ORBIT_API_URL = "http://api:8080"
TIMEOUT_SECONDS = 120
POLL_INTERVAL_SECONDS = 2


def get_json(url: str, timeout: float = 4.0) -> dict[str, Any]:
    request = urllib.request.Request(url, headers={"Accept": "application/json"})
    with urllib.request.urlopen(request, timeout=timeout) as response:
        if response.status < 200 or response.status >= 300:
            raise RuntimeError(f"unexpected HTTP status {response.status} from {url}")
        result = json.loads(response.read().decode("utf-8"))
        if not isinstance(result, dict):
            raise RuntimeError(f"unexpected JSON payload from {url}")
        return result


def wait_for(label: str, probe: Any) -> None:
    deadline = time.monotonic() + TIMEOUT_SECONDS
    last_error = "data not visible yet"
    while time.monotonic() < deadline:
        try:
            if probe():
                print(f"PASS: {label}")
                return
        except (OSError, urllib.error.URLError, TimeoutError, ValueError, RuntimeError) as exc:
            last_error = str(exc)
        time.sleep(POLL_INTERVAL_SECONDS)
    raise RuntimeError(f"timed out waiting for {label}: {last_error}")


def main() -> int:
    try:
        emitted = {}
        for _ in range(3):
            emitted = get_json(f"{DEMO_URL}/emit")
            time.sleep(0.5)
        trace_id = emitted.get("trace_id")
        if emitted.get("status") != "accepted" or not isinstance(trace_id, str) or len(trace_id) != 32:
            raise RuntimeError(f"demo service returned an invalid trace identifier: {emitted}")
        if any(char not in "0123456789abcdef" for char in trace_id):
            raise RuntimeError(f"demo service returned a malformed trace identifier: {trace_id}")
        print(f"Submitted OTLP smoke payload; trace_id={trace_id}")

        def metric_exists() -> bool:
            query = urllib.parse.urlencode({"query": "orbit_demo_requests_total"})
            payload = get_json(f"{PROMETHEUS_URL}/api/v1/query?{query}")
            return payload.get("status") == "success" and bool(payload.get("data", {}).get("result"))

        def log_exists() -> bool:
            params = urllib.parse.urlencode(
                {
                    "query": '{service_name="orbit-telemetry-demo"} |= "orbit-telemetry-smoke"',
                    "limit": "5",
                    "direction": "backward",
                    "start": str(int((time.time() - 120) * 1_000_000_000)),
                    "end": str(int((time.time() + 10) * 1_000_000_000)),
                }
            )
            payload = get_json(f"{LOKI_URL}/loki/api/v1/query_range?{params}")
            return payload.get("status") == "success" and bool(payload.get("data", {}).get("result"))

        def trace_exists() -> bool:
            payload = get_json(f"{TEMPO_URL}/api/traces/{trace_id}")
            return bool(payload)

        wait_for("OTLP metric is queryable in Prometheus", metric_exists)
        wait_for("OTLP log is queryable in Loki", log_exists)
        wait_for("OTLP trace is queryable in Tempo", trace_exists)
        print("PASS: all three OTLP signals reached their local backends")

        def api_services_exist() -> bool:
            payload = get_json(f"{ORBIT_API_URL}/api/v1/telemetry/services?window=1h")
            return (
                payload.get("source") == "telemetry_backends"
                and any(item.get("name") == "orbit-telemetry-demo" for item in payload.get("data", []))
                and {"prometheus", "loki", "tempo"}.issubset(set(
                    next((item.get("sources", []) for item in payload.get("data", [])
                          if item.get("name") == "orbit-telemetry-demo"), [])
                ))
            )

        def api_metrics_exist() -> bool:
            payload = get_json(f"{ORBIT_API_URL}/api/v1/telemetry/metrics?window=1h&service=orbit-telemetry-demo")
            series = payload.get("data", [])
            return (
                payload.get("source") == "prometheus"
                and payload.get("window") == "1h"
                and bool(payload.get("queried_at"))
                and any(item.get("values") for item in series)
            )

        def api_logs_exist() -> bool:
            query = urllib.parse.urlencode(
                {"service": "orbit-telemetry-demo", "window": "1h", "limit": "50"}
            )
            payload = get_json(f"{ORBIT_API_URL}/api/v1/telemetry/logs?{query}")
            return (
                payload.get("source") == "loki"
                and payload.get("service") == "orbit-telemetry-demo"
                and bool(payload.get("data"))
            )

        def api_traces_exist() -> bool:
            query = urllib.parse.urlencode(
                {"service": "orbit-telemetry-demo", "window": "1h", "limit": "20"}
            )
            payload = get_json(f"{ORBIT_API_URL}/api/v1/telemetry/traces?{query}")
            traces = payload.get("data", [])
            if (
                payload.get("source") == "tempo"
                and payload.get("service") == "orbit-telemetry-demo"
                and any(item.get("traceID") == trace_id for item in traces)
            ):
                return True
            visible_ids = [
                item.get("traceID")
                for item in traces[:5]
                if isinstance(item, dict) and isinstance(item.get("traceID"), str)
            ]
            raise RuntimeError(
                "Tempo API search has not returned the emitted trace yet "
                f"(returned={len(traces)}, expected={trace_id}, sample_ids={visible_ids})"
            )

        wait_for("ORBIT API discovers services across all three backends", api_services_exist)
        wait_for("ORBIT API returns the Prometheus metric", api_metrics_exist)
        wait_for("ORBIT API returns Loki logs", api_logs_exist)
        wait_for("ORBIT API returns the matching Tempo trace", api_traces_exist)
        print("PASS: all three signals are queryable through the ORBIT API")

        return 0
    except (OSError, urllib.error.URLError, TimeoutError, ValueError, RuntimeError) as exc:
        print(f"FAIL: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
