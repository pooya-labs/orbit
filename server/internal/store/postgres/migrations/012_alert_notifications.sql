ALTER TABLE alert_instance_events
    ADD CONSTRAINT alert_instance_events_id_workspace_unique UNIQUE (id, workspace_id);

CREATE TABLE alert_notification_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    adapter TEXT NOT NULL DEFAULT 'webhook' CHECK (adapter = 'webhook'),
    secret_ref TEXT NOT NULL CHECK (secret_ref ~ '^[A-Z][A-Z0-9_]{0,63}$'),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_by_issuer TEXT NOT NULL CHECK (length(created_by_issuer) BETWEEN 1 AND 512),
    created_by_subject TEXT NOT NULL CHECK (length(created_by_subject) BETWEEN 1 AND 512),
    updated_by_issuer TEXT NOT NULL CHECK (length(updated_by_issuer) BETWEEN 1 AND 512),
    updated_by_subject TEXT NOT NULL CHECK (length(updated_by_subject) BETWEEN 1 AND 512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, workspace_id)
);

CREATE UNIQUE INDEX alert_notification_channels_workspace_name_unique
    ON alert_notification_channels (workspace_id, lower(name));

CREATE INDEX alert_notification_channels_workspace_enabled_idx
    ON alert_notification_channels (workspace_id, enabled, created_at DESC);

CREATE TABLE alert_notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    alert_instance_id UUID NOT NULL,
    alert_instance_event_id UUID NOT NULL,
    channel_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('alert.firing', 'alert.resolved')),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'delivered', 'dead')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 8),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '' CHECK (length(last_error) <= 80),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, workspace_id),
    UNIQUE (alert_instance_event_id, channel_id),
    CONSTRAINT alert_notification_deliveries_workspace_instance_fk
        FOREIGN KEY (alert_instance_id, workspace_id)
        REFERENCES alert_instances (id, workspace_id) ON DELETE RESTRICT,
    CONSTRAINT alert_notification_deliveries_workspace_event_fk
        FOREIGN KEY (alert_instance_event_id, workspace_id)
        REFERENCES alert_instance_events (id, workspace_id) ON DELETE RESTRICT,
    CONSTRAINT alert_notification_deliveries_workspace_channel_fk
        FOREIGN KEY (channel_id, workspace_id)
        REFERENCES alert_notification_channels (id, workspace_id) ON DELETE RESTRICT,
    CHECK ((status = 'processing' AND lease_until IS NOT NULL) OR status <> 'processing'),
    CHECK ((status = 'delivered' AND delivered_at IS NOT NULL) OR status <> 'delivered')
);

CREATE INDEX alert_notification_deliveries_claim_idx
    ON alert_notification_deliveries (status, next_attempt_at, lease_until, created_at)
    WHERE status IN ('pending', 'processing');

CREATE INDEX alert_notification_deliveries_workspace_timeline_idx
    ON alert_notification_deliveries (workspace_id, created_at DESC, id DESC);

CREATE TABLE alert_notification_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    delivery_id UUID NOT NULL,
    attempt_number INTEGER NOT NULL CHECK (attempt_number BETWEEN 1 AND 8),
    outcome TEXT NOT NULL CHECK (outcome IN ('delivered', 'retry_scheduled', 'dead')),
    http_status INTEGER CHECK (http_status BETWEEN 100 AND 599),
    error_code TEXT NOT NULL DEFAULT '' CHECK (length(error_code) <= 80),
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (delivery_id, attempt_number),
    CONSTRAINT alert_notification_attempts_workspace_delivery_fk
        FOREIGN KEY (delivery_id, workspace_id)
        REFERENCES alert_notification_deliveries (id, workspace_id) ON DELETE RESTRICT
);

CREATE INDEX alert_notification_attempts_delivery_timeline_idx
    ON alert_notification_attempts (workspace_id, delivery_id, attempted_at DESC, id DESC);

CREATE FUNCTION prevent_alert_notification_attempt_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'alert notification attempts are append-only';
END;
$$;

CREATE TRIGGER alert_notification_attempts_append_only
    BEFORE UPDATE OR DELETE ON alert_notification_attempts
    FOR EACH ROW EXECUTE FUNCTION prevent_alert_notification_attempt_mutation();

CREATE FUNCTION prevent_alert_notification_attempt_truncate() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'alert notification attempts are append-only';
END;
$$;

CREATE TRIGGER alert_notification_attempts_no_truncate
    BEFORE TRUNCATE ON alert_notification_attempts
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_alert_notification_attempt_truncate();
