-- Record workload authorization denials without retaining bearer tokens or payload data.
ALTER TABLE workload_identity_audit_events
    DROP CONSTRAINT workload_identity_audit_events_action_check;

ALTER TABLE workload_identity_audit_events
    ADD CONSTRAINT workload_identity_audit_events_action_check
    CHECK (action IN (
        'workload.created', 'workload.enabled', 'workload.disabled', 'workload.revoked',
        'workload.grant.created', 'workload.grant.revoked',
        'workload.authorization.denied'
    ));
