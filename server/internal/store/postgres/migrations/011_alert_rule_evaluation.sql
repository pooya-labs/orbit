CREATE UNIQUE INDEX alert_rules_id_workspace_unique
    ON alert_rules (id, workspace_id);

CREATE TABLE alert_rule_states (
    rule_id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL,
    state TEXT NOT NULL DEFAULT 'normal'
        CHECK (state IN ('normal', 'pending', 'firing')),
    active_since TIMESTAMPTZ,
    last_value DOUBLE PRECISION CHECK (last_value IS NULL OR last_value >= 0),
    last_evaluated_at TIMESTAMPTZ,
    last_fired_at TIMESTAMPTZ,
    last_resolved_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT alert_rule_state_rule_workspace_fk
        FOREIGN KEY (rule_id, workspace_id)
        REFERENCES alert_rules (id, workspace_id) ON DELETE CASCADE,
    CHECK (
        (state = 'normal' AND active_since IS NULL) OR
        (state IN ('pending', 'firing') AND active_since IS NOT NULL)
    )
);

CREATE INDEX alert_rule_states_workspace_state_idx
    ON alert_rule_states (workspace_id, state, updated_at DESC);

CREATE TABLE alert_rule_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    rule_id UUID NOT NULL,
    rule_name TEXT NOT NULL CHECK (length(rule_name) BETWEEN 1 AND 120),
    event_type TEXT NOT NULL CHECK (event_type IN (
        'alert.pending',
        'alert.pending_cleared',
        'alert.firing',
        'alert.resolved',
        'alert.disabled',
        'alert.deleted'
    )),
    severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    metric TEXT NOT NULL CHECK (metric = 'request_rate'),
    operator TEXT NOT NULL CHECK (operator IN ('gt', 'gte', 'lt', 'lte')),
    threshold DOUBLE PRECISION NOT NULL CHECK (threshold > 0 AND threshold <= 1000000000),
    observed_value DOUBLE PRECISION CHECK (observed_value IS NULL OR observed_value >= 0),
    details JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(details) = 'object' AND octet_length(details::text) <= 16384),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX alert_rule_events_workspace_rule_time_idx
    ON alert_rule_events (workspace_id, rule_id, occurred_at DESC, id DESC);

CREATE FUNCTION prevent_alert_rule_event_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'alert rule events are append-only';
END;
$$;

CREATE TRIGGER alert_rule_events_append_only
    BEFORE UPDATE OR DELETE ON alert_rule_events
    FOR EACH ROW EXECUTE FUNCTION prevent_alert_rule_event_mutation();

CREATE FUNCTION prevent_alert_rule_event_truncate() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'alert rule events are append-only';
END;
$$;

CREATE TRIGGER alert_rule_events_no_truncate
    BEFORE TRUNCATE ON alert_rule_events
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_alert_rule_event_truncate();
