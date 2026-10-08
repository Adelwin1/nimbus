BEGIN;
ALTER TABLE github_oauth_states ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE github_oauth_states ADD COLUMN login_challenge BYTEA;
ALTER TABLE github_oauth_states ADD CONSTRAINT github_login_state_shape CHECK (
 (user_id IS NOT NULL AND login_challenge IS NULL) OR
 (user_id IS NULL AND phase='oauth' AND login_challenge IS NOT NULL AND octet_length(login_challenge)=32)
);
CREATE TABLE github_login_identities (
 github_user_id BIGINT PRIMARY KEY CHECK(github_user_id>0),
 user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE
);
-- Preserve unambiguous existing workspaces; never choose between duplicate identities.
INSERT INTO github_login_identities(github_user_id,user_id)
SELECT a.github_user_id,a.user_id FROM github_authorizations a
WHERE (SELECT count(*) FROM github_authorizations b WHERE b.github_user_id=a.github_user_id)=1;
CREATE TABLE github_login_exchanges (
 code_hash BYTEA PRIMARY KEY CHECK(octet_length(code_hash)=32),
 challenge BYTEA NOT NULL CHECK(octet_length(challenge)=32),
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX github_login_exchange_expiry ON github_login_exchanges(expires_at);
COMMIT;
