-- Machine identities are distinct from human OIDC principals and workspace memberships.
CREATE TABLE workload_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issuer TEXT NOT NULL CHECK (length(issuer) BETWEEN 1 AND 512),
    subject TEXT NOT NULL CHECK (length(subject) BETWEEN 1 AND 512),
    display_name TEXT NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 120),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked', 'disabled')),
    expires_at TIMESTAMPTZ,
    created_by_issuer TEXT NOT NULL CHECK (length(created_by_issuer) BETWEEN 1 AND 512),
    created_by_subject TEXT NOT NULL CHECK (length(created_by_subject) BETWEEN 1 AND 512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    UNIQUE (issuer, subject),
    CHECK ((status = 'revoked') = (revoked_at IS NOT NULL))
);

CREATE TABLE workload_workspace_grants (
    workload_identity_id UUID NOT NULL REFERENCES workload_identities(id) ON DELETE RESTRICT,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    signal TEXT NOT NULL CHECK (signal IN ('metrics', 'logs', 'traces')),
    granted_by_issuer TEXT NOT NULL CHECK (length(granted_by_issuer) BETWEEN 1 AND 512),
    granted_by_subject TEXT NOT NULL CHECK (length(granted_by_subject) BETWEEN 1 AND 512),
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    PRIMARY KEY (workload_identity_id, workspace_id, signal)
);

CREATE INDEX workload_workspace_grants_workspace_idx
    ON workload_workspace_grants (workspace_id, signal, workload_identity_id)
    WHERE revoked_at IS NULL;

CREATE TABLE workload_identity_audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workload_identity_id UUID REFERENCES workload_identities(id) ON DELETE RESTRICT,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE RESTRICT,
    actor_issuer TEXT NOT NULL CHECK (length(actor_issuer) BETWEEN 1 AND 512),
    actor_subject TEXT NOT NULL CHECK (length(actor_subject) BETWEEN 1 AND 512),
    action TEXT NOT NULL CHECK (action IN (
        'workload.created', 'workload.enabled', 'workload.disabled', 'workload.revoked',
        'workload.grant.created', 'workload.grant.revoked'
    )),
    details JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(details) = 'object' AND octet_length(details::text) <= 8192),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX workload_identity_audit_timeline_idx
    ON workload_identity_audit_events (workload_identity_id, created_at DESC, id DESC);

CREATE FUNCTION prevent_workload_identity_audit_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'workload identity audit events are append-only';
END;
$$;

CREATE TRIGGER workload_identity_audit_events_append_only
    BEFORE UPDATE OR DELETE ON workload_identity_audit_events
    FOR EACH ROW EXECUTE FUNCTION prevent_workload_identity_audit_mutation();

CREATE FUNCTION prevent_workload_identity_audit_truncate() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'workload identity audit events are append-only';
END;
$$;

CREATE TRIGGER workload_identity_audit_events_no_truncate
    BEFORE TRUNCATE ON workload_identity_audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_workload_identity_audit_truncate();
