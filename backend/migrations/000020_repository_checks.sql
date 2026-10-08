BEGIN;
CREATE TABLE repository_checks (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 run_id UUID NOT NULL REFERENCES browser_journey_runs(id) ON DELETE CASCADE,
 application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 release_context JSONB NOT NULL,
 profile TEXT NOT NULL CHECK(profile IN ('node-test','go-test')),
 directory TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','running','passed','failed','error')),
 download_token TEXT,
 lease_token UUID,
 lease_expires_at TIMESTAMPTZ,
 result JSONB,
 error_message TEXT,
 queued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 finished_at TIMESTAMPTZ
);
CREATE INDEX repository_checks_queue ON repository_checks(queued_at) WHERE status='queued';
CREATE INDEX repository_checks_owner ON repository_checks(user_id,run_id,queued_at DESC);
COMMIT;
