-- +goose Up
ALTER TABLE orgs ADD COLUMN org_protected INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- modernc.org/sqlite bundles SQLite 3.49+ which supports DROP COLUMN (3.35.0+).
ALTER TABLE orgs DROP COLUMN org_protected;
