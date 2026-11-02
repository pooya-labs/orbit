CREATE TABLE alert_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    alert_rule_id UUID NOT NULL,
    rule_name TEXT NOT NULL CHECK (length(btrim(rule_name)) BETWEEN 1 AND 120),
    service_name TEXT NOT NULL CHECK (length(service_name) BETWEEN 1 AND 128),
    metric TEXT NOT NULL CHECK (metric = 'request_rate'),
    operator TEXT NOT NULL CHECK (operator IN ('gt', 'gte', 'lt', 'lte')),
    threshold DOUBLE PRECISION NOT NULL CHECK (threshold > 0 AND threshold <= 1000000000),
    window_seconds INTEGER NOT NULL CHECK (window_seconds IN (60, 300, 900)),
    pending_seconds INTEGER NOT NULL CHECK (pending_seconds BETWEEN 0 AND 3600),
    severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'firing', 'resolved', 'cancelled')),
    value DOUBLE PRECISION NOT NULL CHECK (value >= 0 AND value < 'Infinity'::double precision),
    pending_since TIMESTAMPTZ NOT NULL,
    fired_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    last_evaluated_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, workspace_id),
    CHECK ((status = 'firing' AND fired_at IS NOT NULL AND resolved_at IS NULL) OR
           (status = 'pending' AND fired_at IS NULL AND resolved_at IS NULL) OR
           (status IN ('resolved', 'cancelled') AND resolved_at IS NOT NULL))
);

CREATE UNIQUE INDEX alert_instances_one_active_per_rule_service
    ON alert_instances (alert_rule_id, service_name)
    WHERE status IN ('pending', 'firing');

CREATE INDEX alert_instances_workspace_status_idx
    ON alert_instances (workspace_id, status, updated_at DESC, id DESC);

CREATE TABLE alert_instance_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    alert_instance_id UUID NOT NULL,
    alert_rule_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN (
        'alert.pending',
        'alert.firing',
        'alert.resolved',
        'alert.pending_cancelled',
        'alert.rule_updated',
        'alert.rule_disabled',
        'alert.rule_deleted'
    )),
    details JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(details) = 'object' AND octet_length(details::text) <= 16384),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT alert_instance_events_workspace_instance_fk
        FOREIGN KEY (alert_instance_id, workspace_id)
        REFERENCES alert_instances (id, workspace_id) ON DELETE RESTRICT
);

CREATE INDEX alert_instance_events_workspace_timeline_idx
    ON alert_instance_events (workspace_id, alert_instance_id, created_at DESC, id DESC);

CREATE FUNCTION prevent_alert_instance_event_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'alert instance events are append-only';
END;
$$;

CREATE TRIGGER alert_instance_events_append_only
    BEFORE UPDATE OR DELETE ON alert_instance_events
    FOR EACH ROW EXECUTE FUNCTION prevent_alert_instance_event_mutation();

CREATE FUNCTION prevent_alert_instance_event_truncate() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'alert instance events are append-only';
END;
$$;

CREATE TRIGGER alert_instance_events_no_truncate
    BEFORE TRUNCATE ON alert_instance_events
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_alert_instance_event_truncate();
