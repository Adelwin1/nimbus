BEGIN;
CREATE TABLE hosted_worker_dispatches (
 job_id UUID PRIMARY KEY,
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK(kind IN ('browser','repository')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX hosted_worker_dispatches_created ON hosted_worker_dispatches(created_at);
CREATE INDEX hosted_worker_dispatches_user ON hosted_worker_dispatches(user_id,created_at);
COMMIT;
