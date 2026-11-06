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


# An incident transition and its history/audit entries commit as one batch.
db.execute(
    "INSERT INTO incidents (id, workspace_id, title, description, service_id, severity, status, started_at, updated_at, source) "
    "VALUES (?, ?, ?, '', ?, 'high', 'open', ?, ?, 'live')",
    ("incident-a", "workspace-a", "Service degradation", "service-a", now, now),
)
result = db.execute(
    "UPDATE incidents SET status = 'investigating', updated_at = ? WHERE id = ? AND workspace_id = ? AND status = 'open'",
    (now, "incident-a", "workspace-a"),
)
assert result.rowcount == 1
db.execute(
    "INSERT INTO incident_timeline_events (id, workspace_id, incident_id, actor_issuer, actor_subject, event_type, from_status, to_status, comment, request_id, details, created_at) "
    "SELECT 'timeline-a', 'workspace-a', 'incident-a', 'https://id.example', 'user-a', 'incident.status_changed', 'open', 'investigating', '', 'req-a', '{}', ? WHERE changes() = 1",
    (now,),
)
db.execute(
    "INSERT INTO workspace_audit_events (id, workspace_id, actor_issuer, actor_subject, action, resource_type, resource_id, request_id, details, created_at) "
    "SELECT 'audit-transition-a', 'workspace-a', 'https://id.example', 'user-a', 'incident.status_changed', 'incident', 'incident-a', 'req-a', '{}', ? WHERE changes() = 1",
    (now,),
)
assert db.execute("SELECT COUNT(*) FROM incident_timeline_events WHERE incident_id = 'incident-a'").fetchone()[0] == 1
assert db.execute("SELECT COUNT(*) FROM workspace_audit_events WHERE resource_id = 'incident-a'").fetchone()[0] == 1

# A stale status update must not append a misleading timeline or audit record.
db.execute(
    "UPDATE incidents SET status = 'resolved' WHERE id = 'incident-a' AND status = 'open'"
)
db.execute(
    "INSERT INTO incident_timeline_events (id, workspace_id, incident_id, actor_issuer, actor_subject, event_type, from_status, to_status, comment, request_id, details, created_at) "
    "SELECT 'timeline-stale', 'workspace-a', 'incident-a', 'https://id.example', 'user-a', 'incident.status_changed', 'open', 'resolved', '', 'req-stale', '{}', ? WHERE changes() = 1",
    (now,),
)
assert db.execute("SELECT COUNT(*) FROM incident_timeline_events WHERE incident_id = 'incident-a'").fetchone()[0] == 1

# Tenant queries return only the current workspace's rows.
rows = db.execute("SELECT id FROM services WHERE workspace_id = ?", ("workspace-b",)).fetchall()
assert rows == [], rows
rows = db.execute("SELECT id FROM services WHERE workspace_id = ?", ("workspace-a",)).fetchall()
assert [row["id"] for row in rows] == ["service-a"]

print("Cloudflare-native SQLite schema smoke: PASS")
