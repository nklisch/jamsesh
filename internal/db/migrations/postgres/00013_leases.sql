-- +goose Up
CREATE SEQUENCE jamsesh_lease_fencing_tokens AS bigint;

CREATE TABLE leases (
    session_id     TEXT        PRIMARY KEY,
    pod_id         TEXT        NOT NULL,
    fencing_token  BIGINT      NOT NULL,
    acquired_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    released_at    TIMESTAMPTZ,
    heartbeat_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX leases_released_at_idx ON leases(released_at) WHERE released_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS leases_released_at_idx;
DROP TABLE IF EXISTS leases;
DROP SEQUENCE IF EXISTS jamsesh_lease_fencing_tokens;
