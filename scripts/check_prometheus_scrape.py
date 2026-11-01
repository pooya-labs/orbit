#!/usr/bin/env python3
"""Verify Prometheus scrapes the Collector from the private network."""
from __future__ import annotations
import json
import time
import urllib.parse
import urllib.request

def main() -> int:
    deadline = time.monotonic() + 60
    url = "http://prometheus:9090/api/v1/query?" + urllib.parse.urlencode(
        {"query": 'up{job="orbit-otel-collector"}'}
    )
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(url, timeout=4) as response:
                payload = json.loads(response.read().decode("utf-8"))
            rows = payload.get("data", {}).get("result", [])
            if payload.get("status") == "success" and rows and rows[0]["value"][1] == "1":
                print("PASS: Prometheus scrapes the ORBIT Collector over the private network")
                return 0
        except (OSError, ValueError, KeyError):
            pass
        time.sleep(2)
    raise SystemExit("Prometheus did not report orbit-otel-collector as up")

if __name__ == "__main__":
    raise SystemExit(main())
