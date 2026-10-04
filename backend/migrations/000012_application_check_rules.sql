BEGIN;

CREATE TABLE application_check_rules (
    application_id UUID PRIMARY KEY
        REFERENCES applications(id) ON DELETE CASCADE,

    expected_status INTEGER,
    required_text TEXT NOT NULL DEFAULT '',
    json_pointer TEXT,
    json_expected JSONB,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT check_rules_status_range
        CHECK (
            expected_status IS NULL
            OR expected_status BETWEEN 100 AND 599
        ),

    CONSTRAINT check_rules_text_length
        CHECK (char_length(required_text) <= 2048),

    CONSTRAINT check_rules_json_pair
        CHECK (
            (json_pointer IS NULL) = (json_expected IS NULL)
        ),

    CONSTRAINT check_rules_pointer_format
        CHECK (
            json_pointer IS NULL OR (
                char_length(json_pointer) <= 512
                AND json_pointer ~ '^(/([^~]|~[01])*)*$'
            )
        )
);

COMMIT;
