-- +goose Up
CREATE TABLE leases (
    session_id     TEXT    PRIMARY KEY,
    pod_id         TEXT    NOT NULL,
    fencing_token  INTEGER NOT NULL,
    acquired_at    TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    released_at    TEXT,
    heartbeat_at   TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX leases_released_at_idx ON leases(released_at) WHERE released_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS leases_released_at_idx;
DROP TABLE IF EXISTS leases;
