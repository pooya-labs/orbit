#!/usr/bin/env python3
"""Exercise the Cloudflare-native D1 foundation migration with SQLite."""
from pathlib import Path
import sqlite3

migration = Path("cloudflare-native/migrations/0001_foundation.sql").read_text(encoding="utf-8")
db = sqlite3.connect(":memory:")
db.row_factory = sqlite3.Row
db.execute("PRAGMA foreign_keys = ON")
db.executescript(migration)

def expect_integrity_error(sql: str, params: tuple = ()) -> None:
    try:
        db.execute(sql, params)
    except sqlite3.IntegrityError:
        return
    raise AssertionError(f"Expected an integrity failure for: {sql}")

now = "2026-11-06T12:00:00.000Z"
db.execute(
    "INSERT INTO workspaces VALUES (?, ?, ?, ?, ?, ?, ?)",
    ("workspace-a", "Workspace A", "workspace-a", "https://id.example", "user-a", now, now),
)
db.execute(
    "INSERT INTO workspaces VALUES (?, ?, ?, ?, ?, ?, ?)",
    ("workspace-b", "Workspace B", "workspace-b", "https://id.example", "user-b", now, now),
)
db.execute(
    "INSERT INTO workspace_memberships VALUES (?, ?, ?, ?, ?, ?)",
    ("workspace-a", "https://id.example", "user-a", "owner", now, now),
)
db.execute(
    "INSERT INTO services (id, workspace_id, name, description, environment, region, status, source, created_at, updated_at) "
    "VALUES (?, ?, ?, '', 'production', 'eu-west-1', 'operational', 'live', ?, ?)",
    ("service-a", "workspace-a", "api-gateway", now, now),
)

# Tenant-scoped composite foreign keys prohibit cross-workspace references.
expect_integrity_error(
    "INSERT INTO incidents (id, workspace_id, title, service_id, severity, status, started_at, updated_at, source) "
    "VALUES ('bad-incident', 'workspace-b', 'Cross-tenant', 'service-a', 'high', 'open', ?, ?, 'live')",
    (now, now),
)
expect_integrity_error(
    "INSERT INTO workspace_memberships VALUES (?, ?, ?, ?, ?, ?)",
    ("workspace-b", "https://id.example", "user-b", "superuser", now, now),
)
expect_integrity_error(
    "INSERT INTO workspaces VALUES (?, ?, ?, ?, ?, ?, ?)",
    ("bad-workspace", "Bad slug", "Not-Valid", "https://id.example", "user-c", now, now),
)

# Audit history is append-only.
db.execute(
    "INSERT INTO workspace_audit_events (id, workspace_id, actor_issuer, actor_subject, action, resource_type, resource_id, request_id, details, created_at) "
    "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
    ("audit-a", "workspace-a", "https://id.example", "user-a", "workspace.created", "workspace", "workspace-a", "req-a", "{}", now),
)
expect_integrity_error("UPDATE workspace_audit_events SET action = 'tampered' WHERE id = 'audit-a'")
expect_integrity_error("DELETE FROM workspace_audit_events WHERE id = 'audit-a'")

# Tenant queries return only the current workspace's rows.
rows = db.execute("SELECT id FROM services WHERE workspace_id = ?", ("workspace-b",)).fetchall()
assert rows == [], rows
rows = db.execute("SELECT id FROM services WHERE workspace_id = ?", ("workspace-a",)).fetchall()
assert [row["id"] for row in rows] == ["service-a"]

print("Cloudflare-native SQLite schema smoke: PASS")
