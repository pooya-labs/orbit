PRAGMA foreign_keys = ON;

CREATE TABLE workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
  slug TEXT NOT NULL UNIQUE CHECK (length(slug) BETWEEN 1 AND 48 AND substr(slug, 1, 1) GLOB '[a-z0-9]' AND substr(slug, -1, 1) GLOB '[a-z0-9]' AND slug NOT GLOB '*[^a-z0-9-]*'),
  created_by_issuer TEXT NOT NULL CHECK (length(created_by_issuer) BETWEEN 1 AND 512),
  created_by_subject TEXT NOT NULL CHECK (length(created_by_subject) BETWEEN 1 AND 512),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE workspace_memberships (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  user_issuer TEXT NOT NULL CHECK (length(user_issuer) BETWEEN 1 AND 512),
  user_subject TEXT NOT NULL CHECK (length(user_subject) BETWEEN 1 AND 512),
  role TEXT NOT NULL CHECK (role IN ('owner', 'administrator', 'operator', 'viewer')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (workspace_id, user_issuer, user_subject)
);
CREATE INDEX workspace_memberships_principal_idx
  ON workspace_memberships (user_issuer, user_subject, workspace_id);

CREATE TABLE workspace_audit_actors (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  actor_issuer TEXT NOT NULL,
  actor_subject TEXT NOT NULL,
  actor_ref TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (workspace_id, actor_issuer, actor_subject),
  UNIQUE (workspace_id, actor_ref)
);

CREATE TABLE workspace_audit_events (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  actor_issuer TEXT NOT NULL,
  actor_subject TEXT NOT NULL,
  action TEXT NOT NULL CHECK (length(action) BETWEEN 1 AND 96 AND substr(action, 1, 1) GLOB '[a-z]' AND action NOT GLOB '*[^a-z0-9_.:-]*'),
  resource_type TEXT NOT NULL CHECK (length(resource_type) BETWEEN 1 AND 64 AND substr(resource_type, 1, 1) GLOB '[a-z]' AND resource_type NOT GLOB '*[^a-z0-9_.:-]*'),
  resource_id TEXT NOT NULL DEFAULT '' CHECK (length(resource_id) <= 256),
  request_id TEXT NOT NULL DEFAULT '' CHECK (length(request_id) <= 128),
  details TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(details) AND json_type(details) = 'object' AND length(details) <= 16384),
  created_at TEXT NOT NULL
);
CREATE INDEX workspace_audit_events_timeline_idx
  ON workspace_audit_events (workspace_id, created_at DESC, id DESC);
CREATE TRIGGER workspace_audit_events_no_update
  BEFORE UPDATE ON workspace_audit_events
  BEGIN SELECT RAISE(ABORT, 'workspace audit events are append-only'); END;
CREATE TRIGGER workspace_audit_events_no_delete
  BEFORE DELETE ON workspace_audit_events
  BEGIN SELECT RAISE(ABORT, 'workspace audit events are append-only'); END;

CREATE TABLE services (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 128),
  description TEXT NOT NULL DEFAULT '',
  environment TEXT NOT NULL CHECK (length(trim(environment)) BETWEEN 1 AND 64),
  region TEXT NOT NULL CHECK (length(trim(region)) BETWEEN 1 AND 64),
  status TEXT NOT NULL CHECK (status IN ('operational', 'degraded', 'incident', 'unknown')),
  latency_p95_ms REAL,
  error_rate REAL,
  last_seen_at TEXT,
  source TEXT NOT NULL DEFAULT 'live' CHECK (source IN ('live', 'demo_seed')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (id, workspace_id),
  UNIQUE (workspace_id, name, environment)
);
CREATE INDEX services_workspace_status_idx ON services (workspace_id, status);
CREATE INDEX services_workspace_name_idx ON services (workspace_id, name, environment);

CREATE TABLE incidents (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  title TEXT NOT NULL CHECK (length(trim(title)) BETWEEN 1 AND 160),
  description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 8000),
  service_id TEXT NOT NULL,
  severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
  status TEXT NOT NULL CHECK (status IN ('open', 'investigating', 'monitoring', 'resolved')),
  started_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  resolved_at TEXT,
  source TEXT NOT NULL DEFAULT 'live' CHECK (source IN ('live', 'demo_seed')),
  UNIQUE (id, workspace_id),
  FOREIGN KEY (service_id, workspace_id) REFERENCES services(id, workspace_id) ON DELETE RESTRICT
);
CREATE INDEX incidents_workspace_status_idx ON incidents (workspace_id, status, started_at DESC);
CREATE INDEX incidents_workspace_service_idx ON incidents (workspace_id, service_id, started_at DESC);

CREATE TABLE incident_timeline_events (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  incident_id TEXT NOT NULL,
  actor_issuer TEXT NOT NULL,
  actor_subject TEXT NOT NULL,
  event_type TEXT NOT NULL CHECK (event_type IN ('incident.created', 'incident.status_changed', 'incident.reopened')),
  from_status TEXT CHECK (from_status IS NULL OR from_status IN ('open', 'investigating', 'monitoring', 'resolved')),
  to_status TEXT CHECK (to_status IS NULL OR to_status IN ('open', 'investigating', 'monitoring', 'resolved')),
  comment TEXT NOT NULL DEFAULT '' CHECK (length(comment) <= 4000),
  request_id TEXT NOT NULL DEFAULT '' CHECK (length(request_id) <= 128),
  details TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(details) AND json_type(details) = 'object' AND length(details) <= 16384),
  created_at TEXT NOT NULL,
  FOREIGN KEY (incident_id, workspace_id) REFERENCES incidents(id, workspace_id) ON DELETE RESTRICT
);
CREATE INDEX incident_timeline_events_idx
  ON incident_timeline_events (workspace_id, incident_id, created_at DESC, id DESC);

CREATE TABLE alert_rules (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
  description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
  service_name TEXT NOT NULL DEFAULT '',
  metric TEXT NOT NULL CHECK (metric = 'request_rate'),
  operator TEXT NOT NULL CHECK (operator IN ('gt', 'gte', 'lt', 'lte')),
  threshold REAL NOT NULL CHECK (threshold > 0 AND threshold <= 1000000000),
  window_seconds INTEGER NOT NULL CHECK (window_seconds IN (60, 300, 900)),
  pending_seconds INTEGER NOT NULL CHECK (pending_seconds BETWEEN 0 AND 3600),
  severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  created_by_issuer TEXT NOT NULL,
  created_by_subject TEXT NOT NULL,
  updated_by_issuer TEXT NOT NULL,
  updated_by_subject TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (id, workspace_id)
);
CREATE UNIQUE INDEX alert_rules_workspace_name_unique
  ON alert_rules (workspace_id, lower(name));
CREATE INDEX alert_rules_workspace_enabled_idx
  ON alert_rules (workspace_id, enabled, updated_at DESC);

CREATE TABLE alert_instances (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  alert_rule_id TEXT NOT NULL,
  rule_name TEXT NOT NULL,
  service_name TEXT NOT NULL,
  metric TEXT NOT NULL CHECK (metric = 'request_rate'),
  operator TEXT NOT NULL CHECK (operator IN ('gt', 'gte', 'lt', 'lte')),
  threshold REAL NOT NULL CHECK (threshold > 0 AND threshold <= 1000000000),
  window_seconds INTEGER NOT NULL CHECK (window_seconds IN (60, 300, 900)),
  pending_seconds INTEGER NOT NULL CHECK (pending_seconds BETWEEN 0 AND 3600),
  severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
  status TEXT NOT NULL CHECK (status IN ('pending', 'firing', 'resolved', 'cancelled')),
  value REAL NOT NULL CHECK (value >= 0),
  pending_since TEXT NOT NULL,
  fired_at TEXT,
  resolved_at TEXT,
  last_evaluated_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (id, workspace_id),
  FOREIGN KEY (alert_rule_id, workspace_id) REFERENCES alert_rules(id, workspace_id) ON DELETE RESTRICT
);
CREATE INDEX alert_instances_workspace_status_idx ON alert_instances (workspace_id, status, updated_at DESC);

CREATE TABLE alert_instance_events (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  alert_instance_id TEXT NOT NULL,
  alert_rule_id TEXT NOT NULL,
  event_type TEXT NOT NULL CHECK (event_type IN ('alert.pending', 'alert.firing', 'alert.resolved', 'alert.pending_cancelled', 'alert.rule_updated', 'alert.rule_disabled', 'alert.rule_deleted')),
  details TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(details) AND json_type(details) = 'object' AND length(details) <= 16384),
  created_at TEXT NOT NULL,
  UNIQUE (id, workspace_id),
  FOREIGN KEY (alert_instance_id, workspace_id) REFERENCES alert_instances(id, workspace_id) ON DELETE RESTRICT,
  FOREIGN KEY (alert_rule_id, workspace_id) REFERENCES alert_rules(id, workspace_id) ON DELETE RESTRICT
);

CREATE TABLE alert_notification_channels (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 80),
  adapter TEXT NOT NULL DEFAULT 'webhook' CHECK (adapter = 'webhook'),
  secret_ref TEXT NOT NULL CHECK (length(secret_ref) BETWEEN 1 AND 64 AND substr(secret_ref, 1, 1) GLOB '[A-Z]' AND secret_ref NOT GLOB '*[^A-Z0-9_]*'),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  created_by_issuer TEXT NOT NULL,
  created_by_subject TEXT NOT NULL,
  updated_by_issuer TEXT NOT NULL,
  updated_by_subject TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (id, workspace_id)
);

CREATE TABLE alert_notification_deliveries (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  alert_instance_id TEXT NOT NULL,
  alert_instance_event_id TEXT NOT NULL,
  channel_id TEXT NOT NULL,
  event_type TEXT NOT NULL CHECK (event_type IN ('alert.firing', 'alert.resolved')),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'delivered', 'dead')),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 8),
  next_attempt_at TEXT NOT NULL,
  lease_until TEXT,
  last_attempt_at TEXT,
  delivered_at TEXT,
  last_error TEXT NOT NULL DEFAULT '' CHECK (length(last_error) <= 80),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (id, workspace_id),
  UNIQUE (alert_instance_event_id, channel_id),
  FOREIGN KEY (alert_instance_id, workspace_id) REFERENCES alert_instances(id, workspace_id) ON DELETE RESTRICT,
  FOREIGN KEY (alert_instance_event_id, workspace_id) REFERENCES alert_instance_events(id, workspace_id) ON DELETE RESTRICT,
  FOREIGN KEY (channel_id, workspace_id) REFERENCES alert_notification_channels(id, workspace_id) ON DELETE RESTRICT
);
CREATE INDEX alert_notification_deliveries_due_idx
  ON alert_notification_deliveries (status, next_attempt_at);

CREATE TABLE alert_notification_attempts (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  delivery_id TEXT NOT NULL,
  attempt_number INTEGER NOT NULL CHECK (attempt_number BETWEEN 1 AND 8),
  outcome TEXT NOT NULL CHECK (outcome IN ('delivered', 'retry_scheduled', 'dead')),
  http_status INTEGER CHECK (http_status BETWEEN 100 AND 599),
  error_code TEXT NOT NULL DEFAULT '' CHECK (length(error_code) <= 80),
  attempted_at TEXT NOT NULL,
  UNIQUE (delivery_id, attempt_number),
  FOREIGN KEY (delivery_id, workspace_id) REFERENCES alert_notification_deliveries(id, workspace_id) ON DELETE RESTRICT
);

CREATE TABLE deployment_annotations (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  service_id TEXT NOT NULL,
  service_name TEXT NOT NULL CHECK (length(trim(service_name)) BETWEEN 1 AND 128),
  environment TEXT NOT NULL CHECK (length(trim(environment)) BETWEEN 1 AND 64),
  version TEXT NOT NULL CHECK (length(trim(version)) BETWEEN 1 AND 128),
  commit_sha TEXT NOT NULL DEFAULT '' CHECK (commit_sha = '' OR (length(commit_sha) BETWEEN 7 AND 64 AND commit_sha NOT GLOB '*[^0-9a-fA-F]*')),
  change_reference TEXT NOT NULL DEFAULT '' CHECK (length(change_reference) <= 256),
  release_notes TEXT NOT NULL DEFAULT '' CHECK (length(release_notes) <= 4000),
  deployed_at TEXT NOT NULL,
  created_by_issuer TEXT NOT NULL,
  created_by_subject TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (id, workspace_id),
  FOREIGN KEY (service_id, workspace_id) REFERENCES services(id, workspace_id) ON DELETE RESTRICT
);
CREATE INDEX deployment_annotations_service_timeline_idx
  ON deployment_annotations (workspace_id, service_id, deployed_at DESC);

CREATE TABLE service_level_objectives (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  service_id TEXT NOT NULL,
  name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
  route TEXT NOT NULL CHECK (substr(route, 1, 1) = '/' AND length(route) <= 129 AND instr(route, '//') = 0),
  target REAL NOT NULL CHECK (target >= 0.90 AND target <= 0.9999),
  rolling_window_days INTEGER NOT NULL CHECK (rolling_window_days IN (1, 3, 7)),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  created_by_issuer TEXT NOT NULL,
  created_by_subject TEXT NOT NULL,
  updated_by_issuer TEXT NOT NULL,
  updated_by_subject TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (id, workspace_id),
  FOREIGN KEY (service_id, workspace_id) REFERENCES services(id, workspace_id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX service_level_objectives_workspace_service_name_unique
  ON service_level_objectives (workspace_id, service_id, lower(name));
CREATE INDEX service_level_objectives_workspace_service_enabled_idx
  ON service_level_objectives (workspace_id, service_id, enabled, updated_at DESC);
