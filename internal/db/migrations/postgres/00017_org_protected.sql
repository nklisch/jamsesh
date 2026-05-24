-- +goose Up
ALTER TABLE orgs ADD COLUMN org_protected BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE orgs DROP COLUMN org_protected;
