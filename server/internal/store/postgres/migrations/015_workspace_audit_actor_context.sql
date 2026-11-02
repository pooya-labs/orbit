CREATE TABLE workspace_audit_actors (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    actor_issuer TEXT NOT NULL CHECK (length(actor_issuer) BETWEEN 1 AND 512),
    actor_subject TEXT NOT NULL CHECK (length(actor_subject) BETWEEN 1 AND 512),
    actor_ref UUID NOT NULL DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workspace_id, actor_issuer, actor_subject),
    UNIQUE (workspace_id, actor_ref)
);

INSERT INTO workspace_audit_actors (workspace_id, actor_issuer, actor_subject)
SELECT DISTINCT workspace_id, actor_issuer, actor_subject
FROM workspace_audit_events
ON CONFLICT (workspace_id, actor_issuer, actor_subject) DO NOTHING;

ALTER TABLE workspace_audit_events
    ADD COLUMN actor_ref UUID,
    ADD COLUMN actor_role TEXT NOT NULL DEFAULT 'unknown'
        CHECK (actor_role IN ('owner', 'administrator', 'operator', 'viewer', 'system', 'unknown'));

UPDATE workspace_audit_events AS events
SET actor_ref = actors.actor_ref
FROM workspace_audit_actors AS actors
WHERE actors.workspace_id = events.workspace_id
  AND actors.actor_issuer = events.actor_issuer
  AND actors.actor_subject = events.actor_subject;

ALTER TABLE workspace_audit_events
    ALTER COLUMN actor_ref SET NOT NULL,
    ADD CONSTRAINT workspace_audit_events_actor_ref_fk
        FOREIGN KEY (workspace_id, actor_ref)
        REFERENCES workspace_audit_actors (workspace_id, actor_ref)
        ON DELETE RESTRICT;

CREATE FUNCTION assign_workspace_audit_actor() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    actor_reference UUID;
    membership_role TEXT;
BEGIN
    INSERT INTO workspace_audit_actors (workspace_id, actor_issuer, actor_subject)
    VALUES (NEW.workspace_id, NEW.actor_issuer, NEW.actor_subject)
    ON CONFLICT (workspace_id, actor_issuer, actor_subject) DO NOTHING;

    SELECT actor_ref INTO actor_reference
    FROM workspace_audit_actors
    WHERE workspace_id = NEW.workspace_id
      AND actor_issuer = NEW.actor_issuer
      AND actor_subject = NEW.actor_subject;
    NEW.actor_ref := actor_reference;

    SELECT role INTO membership_role
    FROM workspace_memberships
    WHERE workspace_id = NEW.workspace_id
      AND user_issuer = NEW.actor_issuer
      AND user_subject = NEW.actor_subject;

    IF FOUND THEN
        NEW.actor_role := membership_role;
    ELSE
        NEW.actor_role := 'system';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER workspace_audit_events_assign_actor
    BEFORE INSERT ON workspace_audit_events
    FOR EACH ROW EXECUTE FUNCTION assign_workspace_audit_actor();

CREATE FUNCTION prevent_workspace_audit_actor_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'workspace audit actor references are immutable';
END;
$$;

CREATE TRIGGER workspace_audit_actors_immutable
    BEFORE UPDATE OR DELETE ON workspace_audit_actors
    FOR EACH ROW EXECUTE FUNCTION prevent_workspace_audit_actor_mutation();

CREATE FUNCTION prevent_workspace_audit_actor_truncate() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'workspace audit actor references are immutable';
END;
$$;

CREATE TRIGGER workspace_audit_actors_no_truncate
    BEFORE TRUNCATE ON workspace_audit_actors
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_workspace_audit_actor_truncate();
