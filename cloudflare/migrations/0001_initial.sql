-- ORBIT Cloudflare runtime schema v1.
-- This database is dedicated to the Cloudflare runtime; Apps Script Sheets remain independent.

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS principals (
  id TEXT PRIMARY KEY,
  issuer TEXT NOT NULL,
  subject TEXT NOT NULL,
  display_name TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (issuer, subject)
);

CREATE TABLE IF NOT EXISTS memberships (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('viewer','operator','administrator','owner')),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (workspace_id, principal_id)
);

CREATE TABLE IF NOT EXISTS workload_grants (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  issuer TEXT NOT NULL,
  subject TEXT NOT NULL,
  signal TEXT NOT NULL CHECK (signal IN ('metrics','logs','traces')),
  active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0,1)),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (issuer, subject, workspace_id, signal)
);

CREATE TABLE IF NOT EXISTS telemetry_services (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  environment TEXT,
  region TEXT,
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  sources_json TEXT NOT NULL DEFAULT '[]',
  attributes_json TEXT NOT NULL DEFAULT '{}',
  PRIMARY KEY (workspace_id, name)
);

CREATE TABLE IF NOT EXISTS metric_samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  service_name TEXT NOT NULL,
  metric_name TEXT NOT NULL,
  metric_type TEXT NOT NULL,
  value REAL NOT NULL,
  timestamp_ms INTEGER NOT NULL,
  labels_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS metric_samples_workspace_time ON metric_samples(workspace_id, timestamp_ms DESC);
CREATE INDEX IF NOT EXISTS metric_samples_workspace_service_metric ON metric_samples(workspace_id, service_name, metric_name, timestamp_ms DESC);

CREATE TABLE IF NOT EXISTS log_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  service_name TEXT NOT NULL,
  timestamp_ms INTEGER NOT NULL,
  severity_text TEXT,
  body TEXT NOT NULL,
  trace_id TEXT,
  span_id TEXT,
  attributes_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS log_records_workspace_time ON log_records(workspace_id, timestamp_ms DESC);
CREATE INDEX IF NOT EXISTS log_records_workspace_trace ON log_records(workspace_id, trace_id);

CREATE TABLE IF NOT EXISTS spans (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  trace_id TEXT NOT NULL,
  span_id TEXT NOT NULL,
  parent_span_id TEXT,
  service_name TEXT NOT NULL,
  operation_name TEXT NOT NULL,
  start_time_ms INTEGER NOT NULL,
  end_time_ms INTEGER NOT NULL,
  status_code TEXT,
  attributes_json TEXT NOT NULL DEFAULT '{}',
  PRIMARY KEY (workspace_id, trace_id, span_id)
);
CREATE INDEX IF NOT EXISTS spans_workspace_start ON spans(workspace_id, start_time_ms DESC);
CREATE INDEX IF NOT EXISTS spans_workspace_trace ON spans(workspace_id, trace_id);

CREATE TABLE IF NOT EXISTS incidents (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  service_name TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT,
  severity TEXT NOT NULL CHECK (severity IN ('critical','high','medium','low')),
  status TEXT NOT NULL CHECK (status IN ('open','investigating','monitoring','resolved')),
  started_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  resolved_at TEXT
);
CREATE INDEX IF NOT EXISTS incidents_workspace_updated ON incidents(workspace_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS incident_events (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  incident_id TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  message TEXT NOT NULL,
  actor_id TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS alert_rules (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
  metric_id TEXT NOT NULL,
  threshold REAL NOT NULL,
  comparator TEXT NOT NULL CHECK (comparator IN ('gt','gte','lt','lte')),
  duration_seconds INTEGER NOT NULL DEFAULT 60,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS alert_instances (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  rule_id TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
  state TEXT NOT NULL CHECK (state IN ('pending','firing','resolved','cancelled')),
  started_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  resolved_at TEXT
);

CREATE TABLE IF NOT EXISTS notification_channels (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  channel_type TEXT NOT NULL CHECK (channel_type IN ('webhook','email')),
  destination TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0,1)),
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS notification_deliveries (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  channel_id TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued','delivered','failed','retrying')),
  attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TEXT,
  created_at TEXT NOT NULL,
  delivered_at TEXT
);

CREATE TABLE IF NOT EXISTS deployments (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  service_name TEXT NOT NULL,
  version TEXT NOT NULL,
  commit_sha TEXT,
  environment TEXT NOT NULL,
  deployed_at TEXT NOT NULL,
  created_by TEXT
);

CREATE TABLE IF NOT EXISTS slo_objectives (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  service_name TEXT NOT NULL,
  target REAL NOT NULL CHECK (target >= 0.9 AND target <= 0.9999),
  window_days INTEGER NOT NULL CHECK (window_days IN (1,3,7)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_events (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  actor_id TEXT,
  event_type TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT,
  details_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS audit_workspace_created ON audit_events(workspace_id, created_at DESC);

CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT OR IGNORE INTO schema_migrations(version) VALUES ('0001_initial_cloudflare_runtime');
