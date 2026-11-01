CREATE TABLE alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
    service_name TEXT NOT NULL DEFAULT '',
    metric TEXT NOT NULL CHECK (metric = 'request_rate'),
    operator TEXT NOT NULL CHECK (operator IN ('gt', 'gte', 'lt', 'lte')),
    threshold DOUBLE PRECISION NOT NULL CHECK (threshold > 0 AND threshold <= 1000000000),
    window_seconds INTEGER NOT NULL CHECK (window_seconds IN (60, 300, 900)),
    pending_seconds INTEGER NOT NULL CHECK (pending_seconds BETWEEN 0 AND 3600),
    severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_by_issuer TEXT NOT NULL CHECK (length(created_by_issuer) BETWEEN 1 AND 512),
    created_by_subject TEXT NOT NULL CHECK (length(created_by_subject) BETWEEN 1 AND 512),
    updated_by_issuer TEXT NOT NULL CHECK (length(updated_by_issuer) BETWEEN 1 AND 512),
    updated_by_subject TEXT NOT NULL CHECK (length(updated_by_subject) BETWEEN 1 AND 512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX alert_rules_workspace_name_unique
    ON alert_rules (workspace_id, lower(name));

CREATE INDEX alert_rules_workspace_enabled_idx
    ON alert_rules (workspace_id, enabled, updated_at DESC);
