BEGIN;

CREATE TABLE application_repair_reviews (
    id UUID PRIMARY KEY,
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    rule VARCHAR(80) NOT NULL,
    base_commit VARCHAR(64) NOT NULL
        CHECK (base_commit ~ '^[0-9a-f]{40,64}$'),
    patch TEXT NOT NULL CHECK (octet_length(patch) BETWEEN 1 AND 32768),
    patch_sha256 VARCHAR(64) NOT NULL
        CHECK (patch_sha256 ~ '^[0-9a-f]{64}$'),
    validation JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(validation) = 'object'),
    review_status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (review_status IN ('pending', 'approved', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at TIMESTAMPTZ,
    CONSTRAINT repair_review_timestamp CHECK (
        (review_status='pending' AND reviewed_at IS NULL)
        OR (review_status IN ('approved','rejected') AND reviewed_at IS NOT NULL)
    )
);

CREATE INDEX application_repair_reviews_recent_idx
    ON application_repair_reviews(application_id, created_at DESC);

COMMIT;
