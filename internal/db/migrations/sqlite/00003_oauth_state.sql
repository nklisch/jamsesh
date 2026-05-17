-- +goose Up
-- +goose StatementBegin
CREATE TABLE oauth_state (
    nonce TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX oauth_state_expires_idx ON oauth_state(expires_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_state;
-- +goose StatementEnd
