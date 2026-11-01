CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    slug TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9](?:[a-z0-9-]{0,46}[a-z0-9])$'),
    created_by_issuer TEXT NOT NULL CHECK (length(created_by_issuer) BETWEEN 1 AND 512),
    created_by_subject TEXT NOT NULL CHECK (length(created_by_subject) BETWEEN 1 AND 512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE workspace_memberships (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    user_issuer TEXT NOT NULL CHECK (length(user_issuer) BETWEEN 1 AND 512),
    user_subject TEXT NOT NULL CHECK (length(user_subject) BETWEEN 1 AND 512),
    role TEXT NOT NULL CHECK (role IN ('owner', 'administrator', 'operator', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workspace_id, user_issuer, user_subject)
);

CREATE UNIQUE INDEX workspace_memberships_single_owner_idx
    ON workspace_memberships (workspace_id)
    WHERE role = 'owner';

CREATE INDEX workspace_memberships_principal_idx
    ON workspace_memberships (user_issuer, user_subject, workspace_id);

CREATE TABLE workspace_audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    actor_issuer TEXT NOT NULL CHECK (length(actor_issuer) BETWEEN 1 AND 512),
    actor_subject TEXT NOT NULL CHECK (length(actor_subject) BETWEEN 1 AND 512),
    action TEXT NOT NULL CHECK (action ~ '^[a-z][a-z0-9_.:-]{0,95}$'),
    resource_type TEXT NOT NULL CHECK (resource_type ~ '^[a-z][a-z0-9_.:-]{0,63}$'),
    resource_id TEXT NOT NULL DEFAULT '' CHECK (length(resource_id) <= 256),
    request_id TEXT NOT NULL DEFAULT '' CHECK (length(request_id) <= 128),
    details JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(details) = 'object' AND octet_length(details::text) <= 16384),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX workspace_audit_events_timeline_idx
    ON workspace_audit_events (workspace_id, created_at DESC, id DESC);

CREATE FUNCTION prevent_workspace_audit_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'workspace audit events are append-only';
END;
$$;

CREATE TRIGGER workspace_audit_events_append_only
    BEFORE UPDATE OR DELETE ON workspace_audit_events
    FOR EACH ROW EXECUTE FUNCTION prevent_workspace_audit_mutation();

CREATE FUNCTION prevent_workspace_audit_truncate() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'workspace audit events are append-only';
END;
$$;

CREATE TRIGGER workspace_audit_events_no_truncate
    BEFORE TRUNCATE ON workspace_audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_workspace_audit_truncate();
