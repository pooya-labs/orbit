-- Assign pre-workspace records to an explicitly named legacy workspace.
-- Existing API routes remain backward-compatible; new scoped routes must always
-- filter by workspace_id and verify membership before querying.
DO $$
DECLARE legacy_id UUID;
BEGIN
    SELECT id INTO legacy_id FROM workspaces WHERE slug = 'legacy-default';
    IF legacy_id IS NULL THEN
        INSERT INTO workspaces (name, slug, created_by_issuer, created_by_subject)
        VALUES ('Legacy default workspace', 'legacy-default', 'urn:orbit:system', 'legacy-migration')
        RETURNING id INTO legacy_id;
        INSERT INTO workspace_memberships (workspace_id, user_issuer, user_subject, role)
        VALUES (legacy_id, 'urn:orbit:system', 'legacy-migration', 'owner');
    END IF;

    ALTER TABLE services ADD COLUMN IF NOT EXISTS workspace_id UUID;
    UPDATE services SET workspace_id = legacy_id WHERE workspace_id IS NULL;
    ALTER TABLE services ALTER COLUMN workspace_id SET NOT NULL;
    ALTER TABLE services ADD CONSTRAINT services_workspace_fk
        FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT;

    ALTER TABLE incidents ADD COLUMN IF NOT EXISTS workspace_id UUID;
    UPDATE incidents i SET workspace_id = s.workspace_id
    FROM services s WHERE i.service_id = s.id AND i.workspace_id IS NULL;
    ALTER TABLE incidents ALTER COLUMN workspace_id SET NOT NULL;
    ALTER TABLE incidents ADD CONSTRAINT incidents_workspace_fk
        FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT;

    ALTER TABLE telemetry_services ADD COLUMN IF NOT EXISTS workspace_id UUID;
    UPDATE telemetry_services SET workspace_id = legacy_id WHERE workspace_id IS NULL;
    ALTER TABLE telemetry_services ALTER COLUMN workspace_id SET NOT NULL;
    ALTER TABLE telemetry_services ADD CONSTRAINT telemetry_services_workspace_fk
        FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE RESTRICT;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS services_id_workspace_unique
    ON services (id, workspace_id);
CREATE INDEX IF NOT EXISTS services_workspace_name_idx
    ON services (workspace_id, name, environment);
CREATE INDEX IF NOT EXISTS incidents_workspace_status_started_idx
    ON incidents (workspace_id, status, started_at DESC);
CREATE INDEX IF NOT EXISTS telemetry_services_workspace_last_seen_idx
    ON telemetry_services (workspace_id, last_seen_at DESC);

ALTER TABLE incidents ADD CONSTRAINT incidents_service_workspace_fk
    FOREIGN KEY (service_id, workspace_id) REFERENCES services(id, workspace_id) ON DELETE RESTRICT;
