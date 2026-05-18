-- +goose Up
ALTER TABLE orgs
    ADD COLUMN session_invite_policy TEXT NOT NULL DEFAULT 'members_only'
    CHECK (session_invite_policy IN ('members_only', 'open'));

-- +goose Down
ALTER TABLE orgs DROP COLUMN session_invite_policy;
