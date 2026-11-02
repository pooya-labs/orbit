CREATE TABLE deployment_annotations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    service_id UUID NOT NULL,
    service_name TEXT NOT NULL CHECK (length(btrim(service_name)) BETWEEN 1 AND 128),
    environment TEXT NOT NULL CHECK (length(btrim(environment)) BETWEEN 1 AND 64),
    version TEXT NOT NULL CHECK (length(btrim(version)) BETWEEN 1 AND 128),
    commit_sha TEXT NOT NULL DEFAULT ''
        CHECK (commit_sha = '' OR commit_sha ~ '^[0-9A-Fa-f]{7,64}$'),
    change_reference TEXT NOT NULL DEFAULT '' CHECK (length(change_reference) <= 256),
    release_notes TEXT NOT NULL DEFAULT '' CHECK (length(release_notes) <= 4000),
    deployed_at TIMESTAMPTZ NOT NULL,
    created_by_issuer TEXT NOT NULL CHECK (length(created_by_issuer) BETWEEN 1 AND 512),
    created_by_subject TEXT NOT NULL CHECK (length(created_by_subject) BETWEEN 1 AND 512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, workspace_id),
    CONSTRAINT deployment_annotations_service_workspace_fk
        FOREIGN KEY (service_id, workspace_id)
        REFERENCES services (id, workspace_id) ON DELETE RESTRICT
);

CREATE INDEX deployment_annotations_workspace_service_time_idx
    ON deployment_annotations (workspace_id, service_id, deployed_at DESC, id DESC);

CREATE INDEX deployment_annotations_workspace_time_idx
    ON deployment_annotations (workspace_id, deployed_at DESC, id DESC);

CREATE FUNCTION prevent_deployment_annotation_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'deployment annotations are append-only';
END;
$$;

CREATE TRIGGER deployment_annotations_append_only
    BEFORE UPDATE OR DELETE ON deployment_annotations
    FOR EACH ROW EXECUTE FUNCTION prevent_deployment_annotation_mutation();

CREATE FUNCTION prevent_deployment_annotation_truncate() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'deployment annotations are append-only';
END;
$$;

CREATE TRIGGER deployment_annotations_no_truncate
    BEFORE TRUNCATE ON deployment_annotations
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_deployment_annotation_truncate();
