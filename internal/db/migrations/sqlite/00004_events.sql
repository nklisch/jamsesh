-- +goose Up
-- +goose StatementBegin
CREATE TABLE event_seq (
    session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    next INTEGER NOT NULL DEFAULT 0
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE events (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    UNIQUE(session_id, seq)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX events_session_created_idx ON events(session_id, created_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX events_org_idx ON events(org_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE presence (
    org_id TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    ref TEXT NOT NULL,
    current_sha TEXT NOT NULL,
    last_active_at DATETIME NOT NULL,
    PRIMARY KEY (session_id, account_id, ref)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX presence_org_idx ON presence(org_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS presence;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS events;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS event_seq;
-- +goose StatementEnd
