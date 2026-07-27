-- +goose Up

CREATE TABLE deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL
        REFERENCES applications(id)
        ON DELETE CASCADE,

    version VARCHAR(100) NOT NULL,
    commit_sha VARCHAR(100),
    release_notes TEXT NOT NULL DEFAULT '',

    deployment_type VARCHAR(30) NOT NULL DEFAULT 'deployment',
    status VARCHAR(30) NOT NULL DEFAULT 'pending',

    previous_version VARCHAR(100),

    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    triggered_by UUID NOT NULL
        REFERENCES users(id)
        ON DELETE RESTRICT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT deployments_version_not_blank
        CHECK (char_length(trim(version)) > 0),

    CONSTRAINT deployments_type_valid
        CHECK (
            deployment_type IN (
                'deployment',
                'rollback'
            )
        ),

    CONSTRAINT deployments_status_valid
        CHECK (
            status IN (
                'pending',
                'triggering',
                'verifying',
                'successful',
                'failed',
                'cancelled'
            )
        )
);

CREATE TABLE deployment_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id UUID NOT NULL
        REFERENCES deployments(id)
        ON DELETE CASCADE,

    event_type VARCHAR(60) NOT NULL,
    message TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT deployment_events_type_not_blank
        CHECK (char_length(trim(event_type)) > 0),

    CONSTRAINT deployment_events_message_not_blank
        CHECK (char_length(trim(message)) > 0)
);

CREATE INDEX idx_deployments_application_id
    ON deployments(application_id);

CREATE INDEX idx_deployments_application_created_at
    ON deployments(application_id, created_at DESC);

CREATE INDEX idx_deployments_status
    ON deployments(status);

CREATE INDEX idx_deployments_triggered_by
    ON deployments(triggered_by);

CREATE INDEX idx_deployment_events_deployment_id
    ON deployment_events(deployment_id);

CREATE INDEX idx_deployment_events_deployment_created_at
    ON deployment_events(deployment_id, created_at ASC);

-- Prevent two active releases for the same application.
CREATE UNIQUE INDEX idx_deployments_one_active_per_application
    ON deployments(application_id)
    WHERE status IN (
        'pending',
        'triggering',
        'verifying'
    );

-- +goose Down

DROP TABLE IF EXISTS deployment_events;
DROP TABLE IF EXISTS deployments;
