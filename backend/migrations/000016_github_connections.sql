BEGIN;
CREATE TABLE github_authorizations (
 user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 github_user_id BIGINT NOT NULL, login TEXT NOT NULL,
 token_encrypted TEXT NOT NULL, expires_at TIMESTAMPTZ NOT NULL,
 connected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE github_oauth_states (
 state_hash BYTEA PRIMARY KEY, user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 phase TEXT NOT NULL CHECK (phase IN ('ticket','oauth')), expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX github_oauth_expiry ON github_oauth_states(expires_at);
CREATE TABLE application_github_repositories (
 application_id UUID PRIMARY KEY REFERENCES applications(id) ON DELETE CASCADE,
 installation_id BIGINT NOT NULL CHECK(installation_id>0), repository_id BIGINT NOT NULL CHECK(repository_id>0),
 full_name TEXT NOT NULL, default_branch TEXT NOT NULL,
 linked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
COMMIT;
