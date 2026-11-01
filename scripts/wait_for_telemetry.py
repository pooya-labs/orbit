#!/usr/bin/env python3
"""Wait until telemetry services are healthy from inside the private network."""
from __future__ import annotations
import time
import urllib.error
import urllib.request

PROBES = {
    "Prometheus": "http://prometheus:9090/-/ready",
    "Loki": "http://loki:3100/ready",
    "Tempo": "http://tempo:3200/ready",
    "Collector": "http://otel-collector:13133/",
    "demo": "http://telemetry-demo:8081/healthz",
    "ORBIT API": "http://api:8080/health/ready",
}

def ready(url: str) -> bool:
    request = urllib.request.Request(url)
    with urllib.request.urlopen(request, timeout=3) as response:
        return 200 <= response.status < 300

def main() -> int:
    pending = dict(PROBES)
    deadline = time.monotonic() + 90
    while pending and time.monotonic() < deadline:
        for name, url in list(pending.items()):
            try:
                if ready(url):
                    print(f"READY: {name}")
                    del pending[name]
            except (OSError, urllib.error.URLError, TimeoutError):
                pass
        if pending:
            time.sleep(2)
    if pending:
        raise SystemExit("Timed out waiting for: " + ", ".join(pending))
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
