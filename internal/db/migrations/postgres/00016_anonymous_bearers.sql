-- +goose Up
ALTER TABLE accounts
  ADD COLUMN is_anonymous BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE oauth_tokens
  ADD COLUMN session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE;

ALTER TABLE oauth_tokens
  DROP CONSTRAINT oauth_tokens_kind_check,
  ADD CONSTRAINT oauth_tokens_kind_check
    CHECK (kind IN ('access', 'refresh', 'anonymous_session_bearer'));

CREATE INDEX oauth_tokens_session_idx ON oauth_tokens(session_id)
  WHERE session_id IS NOT NULL;

-- +goose Down
-- Rows with the new kind must be removed before restoring the old CHECK.
DELETE FROM oauth_tokens WHERE kind = 'anonymous_session_bearer';

DROP INDEX IF EXISTS oauth_tokens_session_idx;

ALTER TABLE oauth_tokens
  DROP CONSTRAINT oauth_tokens_kind_check,
  ADD CONSTRAINT oauth_tokens_kind_check
    CHECK (kind IN ('access', 'refresh'));

ALTER TABLE oauth_tokens
  DROP COLUMN session_id;

ALTER TABLE accounts
  DROP COLUMN is_anonymous;
