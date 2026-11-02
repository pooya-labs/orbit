CREATE TABLE service_level_objectives (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    service_id UUID NOT NULL,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    route TEXT NOT NULL CHECK (
        route ~ '^/[A-Za-z0-9][A-Za-z0-9/_-]{0,127}$'
        AND position('//' in route) = 0
    ),
    target DOUBLE PRECISION NOT NULL CHECK (target >= 0.90 AND target <= 0.9999),
    rolling_window_days INTEGER NOT NULL CHECK (rolling_window_days IN (1, 3, 7)),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_by_issuer TEXT NOT NULL CHECK (length(created_by_issuer) BETWEEN 1 AND 512),
    created_by_subject TEXT NOT NULL CHECK (length(created_by_subject) BETWEEN 1 AND 512),
    updated_by_issuer TEXT NOT NULL CHECK (length(updated_by_issuer) BETWEEN 1 AND 512),
    updated_by_subject TEXT NOT NULL CHECK (length(updated_by_subject) BETWEEN 1 AND 512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, workspace_id),
    CONSTRAINT service_level_objectives_service_workspace_fk
        FOREIGN KEY (service_id, workspace_id)
        REFERENCES services (id, workspace_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX service_level_objectives_workspace_service_name_unique
    ON service_level_objectives (workspace_id, service_id, lower(name));

CREATE INDEX service_level_objectives_workspace_service_enabled_idx
    ON service_level_objectives (workspace_id, service_id, enabled, updated_at DESC);
