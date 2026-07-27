-- +goose Up

CREATE TABLE activity_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    actor_user_id UUID
        REFERENCES users(id)
        ON DELETE SET NULL,

    application_id UUID
        REFERENCES applications(id)
        ON DELETE CASCADE,

    action VARCHAR(100) NOT NULL,
    entity_type VARCHAR(60) NOT NULL,
    entity_id UUID,

    summary TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,

    request_id VARCHAR(100),
    ip_address INET,
    user_agent TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT activity_logs_action_not_blank
        CHECK (char_length(trim(action)) > 0),

    CONSTRAINT activity_logs_entity_type_not_blank
        CHECK (char_length(trim(entity_type)) > 0),

    CONSTRAINT activity_logs_summary_not_blank
        CHECK (char_length(trim(summary)) > 0)
);

CREATE INDEX idx_activity_logs_actor_user
    ON activity_logs(actor_user_id, created_at DESC);

CREATE INDEX idx_activity_logs_application
    ON activity_logs(application_id, created_at DESC);

CREATE INDEX idx_activity_logs_entity
    ON activity_logs(entity_type, entity_id);

CREATE INDEX idx_activity_logs_action
    ON activity_logs(action, created_at DESC);

CREATE INDEX idx_activity_logs_created_at
    ON activity_logs(created_at DESC);

-- +goose Down

DROP TABLE IF EXISTS activity_logs;
