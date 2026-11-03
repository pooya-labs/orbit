#!/usr/bin/env python3
"""Guard ORBIT's authenticated OTLP resource-attribute allowlist contract."""
from __future__ import annotations

from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]


def read(relative: str) -> str:
    path = ROOT / relative
    if not path.is_file():
        raise SystemExit(f"missing resource-attribute policy file: {relative}")
    return path.read_text(encoding="utf-8")


def extract_allowlist(source: str, variable: str, relative: str) -> set[str]:
    declaration = re.search(rf"(?m)^var\s+{re.escape(variable)}\s*=", source)
    if declaration is None:
        raise SystemExit(f"{relative}: missing {variable} declaration")

    # The map literal opens on its own line after the Go map type. Locate that
    # brace separately from the braces in map[string]struct{}.
    opening = source.find("{\n", declaration.end())
    closing = source.find("\n}", opening + 2) if opening >= 0 else -1
    if opening < 0 or closing < 0:
        raise SystemExit(f"{relative}: could not locate {variable} map body")

    body = source[opening + 2 : closing]
    entries = re.findall(r'^\s*"([^"]+)"\s*:\s*\{\}\s*,?\s*$', body, re.MULTILINE)
    non_empty_lines = [line for line in body.splitlines() if line.strip()]
    if len(entries) != len(non_empty_lines):
        raise SystemExit(f"{relative}: {variable} contains an unrecognized map entry")
    if len(entries) != len(set(entries)):
        raise SystemExit(f"{relative}: {variable} contains duplicate keys")
    return set(entries)


def extract_positive_int(source: str, name: str, relative: str) -> int:
    match = re.search(rf"(?m)^\s*{re.escape(name)}\s*=\s*(\d+)\s*$", source)
    if match is None:
        raise SystemExit(f"{relative}: missing numeric limit {name}")
    value = int(match.group(1))
    if value <= 0:
        raise SystemExit(f"{relative}: {name} must be positive")
    return value


ingest_path = "server/internal/ingest/resource_attributes.go"
store_path = "server/internal/store/postgres/workload_resource_attributes.go"
ingest_source = read(ingest_path)
store_source = read(store_path)

ingest_keys = extract_allowlist(
    ingest_source, "workloadResourceAttributeAllowlist", ingest_path
)
store_keys = extract_allowlist(
    store_source, "workloadResourceAttributePersistenceAllowlist", store_path
)

if ingest_keys != store_keys:
    missing_at_persistence = sorted(ingest_keys - store_keys)
    missing_at_ingress = sorted(store_keys - ingest_keys)
    raise SystemExit(
        "resource-attribute allowlists have drifted:\n"
        f"  accepted by ingress but rejected by persistence: {missing_at_persistence}\n"
        f"  accepted by persistence but not extracted by ingress: {missing_at_ingress}"
    )

# These identifiers, network details, and potentially secret-bearing values
# require a separate privacy/cardinality review; never add them by accident.
forbidden = {
    "service.instance.id",
    "service.criticality",
    "host.id",
    "host.ip",
    "host.mac",
    "cloud.account.id",
    "cloud.resource_id",
    "container.id",
    "k8s.cluster.uid",
    "k8s.pod.uid",
    "process.pid",
    "process.command_line",
    "process.command_args",
    "process.executable.path",
    "process.owner",
    "user.id",
    "user.name",
    "user.email",
    "orbit.workspace.id",
    "orbit_workspace_id",
}
unexpected = sorted(ingest_keys & forbidden)
if unexpected:
    raise SystemExit(
        "resource-attribute allowlist includes explicitly deferred sensitive/"
        f"high-cardinality keys: {unexpected}"
    )

limit_pairs = [
    ("maxWorkloadResourceServices", "maxPersistedWorkloadResourceObservations"),
    ("maxWorkloadResourceAttributes", "maxPersistedWorkloadResourceAttributes"),
    ("maxWorkloadResourceValues", "maxPersistedWorkloadResourceValues"),
    ("maxWorkloadResourceValueBytes", "maxPersistedWorkloadResourceValueBytes"),
]
for ingress_name, persistence_name in limit_pairs:
    ingress_limit = extract_positive_int(ingest_source, ingress_name, ingest_path)
    persistence_limit = extract_positive_int(store_source, persistence_name, store_path)
    if ingress_limit != persistence_limit:
        raise SystemExit(
            f"resource-attribute bound mismatch: {ingress_name}={ingress_limit}, "
            f"{persistence_name}={persistence_limit}"
        )

json_limit = re.search(
    r"(?m)^\s*maxPersistedWorkloadResourceJSONBytes\s*=\s*(\d+)\s*<<\s*10\s*$",
    store_source,
)
if json_limit is None or int(json_limit.group(1)) <= 0:
    raise SystemExit(
        f"{store_path}: expected a positive KiB-based JSON persistence cap"
    )

print(
    f"PASS: ingress and PostgreSQL persistence allowlists match exactly "
    f"({len(ingest_keys)} attributes)."
)
print(
    f"PASS: {len(forbidden)} explicitly deferred sensitive/high-cardinality "
    "attributes remain excluded."
)
print(
    "PASS: service, attribute, value-count, and string-length bounds match "
    "between extraction and persistence."
)
print(
    f"PASS: persisted resource-attribute JSON has a {int(json_limit.group(1))} KiB cap."
)
print(
    "NOTE: this is a contract regression check, not evidence that every accepted "
    "attribute is appropriate for every deployment."
)
