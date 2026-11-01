CREATE UNIQUE INDEX incidents_id_workspace_unique
    ON incidents (id, workspace_id);

CREATE TABLE incident_timeline_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    incident_id UUID NOT NULL,
    actor_issuer TEXT NOT NULL CHECK (length(actor_issuer) BETWEEN 1 AND 512),
    actor_subject TEXT NOT NULL CHECK (length(actor_subject) BETWEEN 1 AND 512),
    event_type TEXT NOT NULL CHECK (event_type IN (
        'incident.created',
        'incident.status_changed',
        'incident.reopened'
    )),
    from_status TEXT CHECK (from_status IS NULL OR from_status IN ('open', 'investigating', 'monitoring', 'resolved')),
    to_status TEXT CHECK (to_status IS NULL OR to_status IN ('open', 'investigating', 'monitoring', 'resolved')),
    comment TEXT NOT NULL DEFAULT '' CHECK (length(comment) <= 4000),
    request_id TEXT NOT NULL DEFAULT '' CHECK (length(request_id) <= 128),
    details JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(details) = 'object' AND octet_length(details::text) <= 16384),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT incident_timeline_workspace_incident_fk
        FOREIGN KEY (incident_id, workspace_id)
        REFERENCES incidents (id, workspace_id) ON DELETE RESTRICT
);

CREATE INDEX incident_timeline_events_idx
    ON incident_timeline_events (workspace_id, incident_id, created_at DESC, id DESC);

CREATE FUNCTION prevent_incident_timeline_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'incident timeline events are append-only';
END;
$$;

CREATE TRIGGER incident_timeline_events_append_only
    BEFORE UPDATE OR DELETE ON incident_timeline_events
    FOR EACH ROW EXECUTE FUNCTION prevent_incident_timeline_mutation();

CREATE FUNCTION prevent_incident_timeline_truncate() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'incident timeline events are append-only';
END;
$$;

CREATE TRIGGER incident_timeline_events_no_truncate
    BEFORE TRUNCATE ON incident_timeline_events
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_incident_timeline_truncate();
