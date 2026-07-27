-- +goose Up

CREATE TABLE incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    application_id UUID NOT NULL
        REFERENCES applications(id)
        ON DELETE CASCADE,

    incident_type VARCHAR(40) NOT NULL,
    title VARCHAR(200) NOT NULL,
    summary TEXT NOT NULL DEFAULT '',

    severity VARCHAR(20) NOT NULL DEFAULT 'warning',
    status VARCHAR(20) NOT NULL DEFAULT 'open',

    -- Used to prevent multiple active incidents for the same problem.
    dedup_key VARCHAR(200) NOT NULL,

    source_health_check_id UUID
        REFERENCES health_checks(id)
        ON DELETE SET NULL,

    source_deployment_id UUID
        REFERENCES deployments(id)
        ON DELETE SET NULL,

    rollback_deployment_id UUID
        REFERENCES deployments(id)
        ON DELETE SET NULL,

    acknowledged_at TIMESTAMPTZ,
    acknowledged_by UUID
        REFERENCES users(id)
        ON DELETE SET NULL,

    resolved_at TIMESTAMPTZ,
    resolved_by UUID
        REFERENCES users(id)
        ON DELETE SET NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT incidents_type_valid
        CHECK (
            incident_type IN (
                'health_failure',
                'excessive_latency',
                'deployment_failure'
            )
        ),

    CONSTRAINT incidents_severity_valid
        CHECK (
            severity IN (
                'warning',
                'critical'
            )
        ),

    CONSTRAINT incidents_status_valid
        CHECK (
            status IN (
                'open',
                'acknowledged',
                'resolved'
            )
        ),

    CONSTRAINT incidents_title_not_blank
        CHECK (char_length(trim(title)) > 0),

    CONSTRAINT incidents_dedup_key_not_blank
        CHECK (char_length(trim(dedup_key)) > 0),

    CONSTRAINT incidents_acknowledgement_consistent
        CHECK (
            (
                acknowledged_at IS NULL
                AND acknowledged_by IS NULL
            )
            OR acknowledged_at IS NOT NULL
        ),

    CONSTRAINT incidents_resolution_consistent
        CHECK (
            (
                resolved_at IS NULL
                AND resolved_by IS NULL
            )
            OR resolved_at IS NOT NULL
        )
);

CREATE TABLE incident_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    incident_id UUID NOT NULL
        REFERENCES incidents(id)
        ON DELETE CASCADE,

    event_type VARCHAR(60) NOT NULL,
    message TEXT NOT NULL,

    actor_user_id UUID
        REFERENCES users(id)
        ON DELETE SET NULL,

    health_check_id UUID
        REFERENCES health_checks(id)
        ON DELETE SET NULL,

    deployment_id UUID
        REFERENCES deployments(id)
        ON DELETE SET NULL,

    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT incident_events_type_not_blank
        CHECK (char_length(trim(event_type)) > 0),

    CONSTRAINT incident_events_message_not_blank
        CHECK (char_length(trim(message)) > 0)
);

CREATE INDEX idx_incidents_application_id
    ON incidents(application_id);

CREATE INDEX idx_incidents_application_created_at
    ON incidents(application_id, created_at DESC);

CREATE INDEX idx_incidents_status
    ON incidents(status);

CREATE INDEX idx_incidents_type
    ON incidents(incident_type);

CREATE INDEX idx_incidents_source_health_check
    ON incidents(source_health_check_id);

CREATE INDEX idx_incidents_source_deployment
    ON incidents(source_deployment_id);

CREATE INDEX idx_incident_events_incident_id
    ON incident_events(incident_id);

CREATE INDEX idx_incident_events_incident_created_at
    ON incident_events(incident_id, created_at ASC);

-- Only one active incident of a particular problem may exist
-- for an application.
CREATE UNIQUE INDEX idx_incidents_active_deduplication
    ON incidents(application_id, dedup_key)
    WHERE status IN (
        'open',
        'acknowledged'
    );

-- A failed deployment verification may create only one incident.
CREATE UNIQUE INDEX idx_incidents_unique_source_deployment
    ON incidents(source_deployment_id)
    WHERE source_deployment_id IS NOT NULL
      AND incident_type = 'deployment_failure';

-- A rollback deployment may belong to only one incident.
CREATE UNIQUE INDEX idx_incidents_unique_rollback_deployment
    ON incidents(rollback_deployment_id)
    WHERE rollback_deployment_id IS NOT NULL;

-- +goose Down

DROP TABLE IF EXISTS incident_events;
DROP TABLE IF EXISTS incidents;
