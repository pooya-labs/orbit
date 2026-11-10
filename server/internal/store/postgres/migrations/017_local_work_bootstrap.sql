-- Bootstrap a fully operational, single-user local workspace.
-- The local principal is accepted only by the opt-in Local Work token verifier;
-- this migration does not enable local authentication in staging or production.
DO $$
DECLARE
    local_workspace_id UUID;
    local_workload_id UUID;
    telemetry_service_id UUID;
BEGIN
    INSERT INTO workspaces (id, name, slug, created_by_issuer, created_by_subject)
    VALUES (
        'f2000000-0000-4000-8000-000000000001',
        'ORBIT Local Work',
        'orbit-local-work',
        'urn:orbit:local-work',
        'local-owner'
    )
    ON CONFLICT (slug) DO NOTHING;

    SELECT id INTO local_workspace_id
    FROM workspaces
    WHERE slug = 'orbit-local-work';

    IF local_workspace_id IS NULL THEN
        RAISE EXCEPTION 'could not create ORBIT Local Work workspace';
    END IF;

    INSERT INTO workspace_memberships (workspace_id, user_issuer, user_subject, role)
    VALUES (local_workspace_id, 'urn:orbit:local-work', 'local-owner', 'owner')
    ON CONFLICT (workspace_id, user_issuer, user_subject) DO NOTHING;

    INSERT INTO services
        (id, workspace_id, name, description, environment, region, status,
         latency_p95_ms, error_rate, last_seen_at, source)
    VALUES
        ('f2000000-0000-4000-8000-000000000101', local_workspace_id, 'api-gateway',
         'Edge request router · seeded scenario', 'production', 'eu-central-1',
         'operational', 42, 0.0003, NOW(), 'demo_seed'),
        ('f2000000-0000-4000-8000-000000000102', local_workspace_id, 'identity-core',
         'Authentication service · seeded scenario', 'production', 'eu-central-1',
         'operational', 68, 0.0008, NOW(), 'demo_seed'),
        ('f2000000-0000-4000-8000-000000000103', local_workspace_id, 'billing-engine',
         'Payments and invoicing · seeded scenario', 'production', 'eu-west-1',
         'degraded', 184, 0.0142, NOW(), 'demo_seed'),
        ('f2000000-0000-4000-8000-000000000104', local_workspace_id, 'event-stream',
         'Asynchronous event processing · seeded scenario', 'production', 'eu-central-1',
         'operational', 31, 0.0001, NOW(), 'demo_seed'),
        ('f2000000-0000-4000-8000-000000000105', local_workspace_id, 'search-indexer',
         'Search infrastructure · seeded scenario', 'production', 'us-east-1',
         'incident', NULL, NULL, NOW() - INTERVAL '15 minutes', 'demo_seed'),
        ('f2000000-0000-4000-8000-000000000106', local_workspace_id, 'orbit-telemetry-demo',
         'Live local OpenTelemetry workload (metrics, logs, and traces)',
         'development', 'local', 'operational', NULL, NULL, NOW(), 'local_work')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO incidents
        (id, workspace_id, title, description, service_id, severity, status,
         started_at, updated_at, source)
    VALUES
        ('f2000000-0000-4000-8000-000000000201', local_workspace_id,
         'Search indexing stalled',
         'The seeded search workload stopped advancing its checkpoint. Synthetic scenario.',
         'f2000000-0000-4000-8000-000000000105', 'critical', 'investigating',
         NOW() - INTERVAL '14 minutes', NOW(), 'demo_seed'),
        ('f2000000-0000-4000-8000-000000000202', local_workspace_id,
         'Elevated payment latency',
         'The seeded payment path is above its normal latency band. Synthetic scenario.',
         'f2000000-0000-4000-8000-000000000103', 'medium', 'monitoring',
         NOW() - INTERVAL '38 minutes', NOW(), 'demo_seed')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO incident_timeline_events
        (workspace_id, incident_id, actor_issuer, actor_subject, event_type,
         to_status, comment, details)
    VALUES
        (local_workspace_id, 'f2000000-0000-4000-8000-000000000201',
         'urn:orbit:local-work', 'local-owner', 'incident.created',
         'investigating', 'Seeded local scenario.', '{}'::jsonb),
        (local_workspace_id, 'f2000000-0000-4000-8000-000000000202',
         'urn:orbit:local-work', 'local-owner', 'incident.created',
         'monitoring', 'Seeded local scenario.', '{}'::jsonb);

    INSERT INTO workspace_audit_events
        (workspace_id, actor_issuer, actor_subject, action, resource_type, resource_id, details)
    VALUES
        (local_workspace_id, 'urn:orbit:local-work', 'local-owner',
         'workspace.created', 'workspace', local_workspace_id::text,
         jsonb_build_object('source', 'local_work_bootstrap')),
        (local_workspace_id, 'urn:orbit:local-work', 'local-owner',
         'incident.created', 'incident', 'f2000000-0000-4000-8000-000000000201',
         jsonb_build_object('source', 'demo_seed')),
        (local_workspace_id, 'urn:orbit:local-work', 'local-owner',
         'incident.created', 'incident', 'f2000000-0000-4000-8000-000000000202',
         jsonb_build_object('source', 'demo_seed'));

    INSERT INTO workload_identities
        (id, issuer, subject, display_name, created_by_issuer, created_by_subject)
    VALUES
        ('f2000000-0000-4000-8000-000000000301',
         'urn:orbit:local-workload', 'orbit-telemetry-demo', 'ORBIT Local Telemetry Demo',
         'urn:orbit:local-work', 'local-owner')
    ON CONFLICT (issuer, subject) DO NOTHING;

    SELECT id INTO local_workload_id
    FROM workload_identities
    WHERE issuer = 'urn:orbit:local-workload'
      AND subject = 'orbit-telemetry-demo';

    INSERT INTO workload_workspace_grants
        (workload_identity_id, workspace_id, signal, granted_by_issuer, granted_by_subject)
    VALUES
        (local_workload_id, local_workspace_id, 'metrics', 'urn:orbit:local-work', 'local-owner'),
        (local_workload_id, local_workspace_id, 'logs', 'urn:orbit:local-work', 'local-owner'),
        (local_workload_id, local_workspace_id, 'traces', 'urn:orbit:local-work', 'local-owner')
    ON CONFLICT (workload_identity_id, workspace_id, signal)
    DO UPDATE SET revoked_at = NULL,
                  granted_by_issuer = EXCLUDED.granted_by_issuer,
                  granted_by_subject = EXCLUDED.granted_by_subject,
                  granted_at = NOW();

    INSERT INTO workload_identity_audit_events
        (workload_identity_id, workspace_id, actor_issuer, actor_subject, action, details)
    VALUES
        (local_workload_id, local_workspace_id, 'urn:orbit:local-work', 'local-owner',
         'workload.grant.created', jsonb_build_object('signal', 'metrics', 'source', 'local_work_bootstrap')),
        (local_workload_id, local_workspace_id, 'urn:orbit:local-work', 'local-owner',
         'workload.grant.created', jsonb_build_object('signal', 'logs', 'source', 'local_work_bootstrap')),
        (local_workload_id, local_workspace_id, 'urn:orbit:local-work', 'local-owner',
         'workload.grant.created', jsonb_build_object('signal', 'traces', 'source', 'local_work_bootstrap'));

    INSERT INTO alert_rules
        (id, workspace_id, name, description, service_name, metric, operator,
         threshold, window_seconds, pending_seconds, severity, enabled,
         created_by_issuer, created_by_subject, updated_by_issuer, updated_by_subject)
    VALUES
        ('f2000000-0000-4000-8000-000000000401', local_workspace_id,
         'Local telemetry traffic surge',
         'Example rule for the live local OTLP workload.',
         'orbit-telemetry-demo', 'request_rate', 'gt', 50, 60, 60, 'high', TRUE,
         'urn:orbit:local-work', 'local-owner', 'urn:orbit:local-work', 'local-owner')
    ON CONFLICT (id) DO NOTHING;

    SELECT id INTO telemetry_service_id
    FROM services
    WHERE workspace_id = local_workspace_id
      AND name = 'orbit-telemetry-demo'
    LIMIT 1;

    INSERT INTO service_level_objectives
        (id, workspace_id, service_id, name, route, target, rolling_window_days,
         enabled, created_by_issuer, created_by_subject, updated_by_issuer, updated_by_subject)
    VALUES
        ('f2000000-0000-4000-8000-000000000501', local_workspace_id,
         telemetry_service_id, 'Local /emit availability', '/emit', 0.999, 7,
         TRUE, 'urn:orbit:local-work', 'local-owner', 'urn:orbit:local-work', 'local-owner')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO deployment_annotations
        (id, workspace_id, service_id, service_name, environment, version,
         commit_sha, change_reference, release_notes, deployed_at,
         created_by_issuer, created_by_subject)
    VALUES
        ('f2000000-0000-4000-8000-000000000601', local_workspace_id,
         telemetry_service_id, 'orbit-telemetry-demo', 'development', '0.4.0',
         '1234567890abcdef1234567890abcdef12345678', 'local-seed',
         'Seeded baseline for local deployment comparison.', NOW() - INTERVAL '1 hour',
         'urn:orbit:local-work', 'local-owner'),
        ('f2000000-0000-4000-8000-000000000602', local_workspace_id,
         telemetry_service_id, 'orbit-telemetry-demo', 'development', '0.5.0',
         'abcdef1234567890abcdef1234567890abcdef12', 'local-seed',
         'Local telemetry demo workload release.', NOW() - INTERVAL '5 minutes',
         'urn:orbit:local-work', 'local-owner')
    ON CONFLICT (id) DO NOTHING;
END $$;
