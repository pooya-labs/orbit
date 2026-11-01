#!/usr/bin/env python3
"""Assert that raw OTLP receivers are not published to the host."""

from __future__ import annotations

import json
import subprocess
import sys


def main() -> int:
    result = subprocess.run(
        ["docker", "compose", "--profile", "observability", "config", "--format", "json"],
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print(result.stderr, file=sys.stderr)
        return result.returncode

    config = json.loads(result.stdout)
    services = config.get("services", {})
    collector = services.get("otel-collector")
    if not isinstance(collector, dict):
        raise RuntimeError("otel-collector service is missing")

    published_targets = {
        int(port["target"])
        for port in collector.get("ports", [])
        if isinstance(port, dict) and "target" in port
    }
    exposed_otlp_ports = published_targets.intersection({4317, 4318})
    if exposed_otlp_ports:
        raise RuntimeError(
            f"raw OTLP receiver ports must not be published to the host: {sorted(exposed_otlp_ports)}"
        )

    networks = config.get("networks", {})
    private_networks = {
        name for name, settings in networks.items()
        if isinstance(settings, dict) and settings.get("internal") is True
    }
    if len(private_networks) != 1:
        raise RuntimeError("exactly one internal telemetry network must be configured")
    private_network = next(iter(private_networks))

    required_services = {"api", "otel-collector", "prometheus", "loki", "tempo", "telemetry-demo", "telemetry-smoke"}
    for name in required_services:
        service = services.get(name)
        if not isinstance(service, dict):
            raise RuntimeError(f"required telemetry service {name!r} is missing")
        attached = service.get("networks", {})
        if private_network not in attached:
            raise RuntimeError(f"service {name!r} is not attached to the private telemetry network")

    print("PASS: OTLP receivers are not host-published and telemetry services share an internal network")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (KeyError, TypeError, ValueError, RuntimeError) as exc:
        print(f"FAIL: {exc}", file=sys.stderr)
        raise SystemExit(1)
