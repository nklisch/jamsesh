---
id: gate-tests-migrate-17-18-up-down-up-isolated
kind: story
stage: review
tags: [testing, portal, migrations]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Migrations 17 (`org_protected`) and 18 (`playground_sessions`) lack dedicated up/down/up tests

## Priority
Medium

## Spec reference
Item: `story-anon-bearer-test-integrity-migration-updownup`

Acceptance criterion: Story decisions: "calling `migrateDown(t, ctx, db, "sqlite", 15)` ... cleanly rolls back 17 and 18 ... along with 16 — intentional because 00018's `oauth_tokens.session_id` FK depends on the column 00016 introduces."

## Gap type
tautological-rework / adversarial-spec-silent — 17/18 only exercised via the cascade in the 00016 test

## Suggested test
```go
func TestMigrate00017_OrgProtected_UpDownUp(t *testing.T) { ... }
func TestMigrate00018_PlaygroundSessions_UpDownUp(t *testing.T) { ... }
```

## Test location (suggested)
`internal/db/migrate_test.go`

## Implementation notes

**Test functions added** (in `internal/db/migrate_test.go`):

- `TestMigrate00017_OrgProtected_UpDownUp` — seeds the DB to version 16 (full MigrateUp then migrateDown to 16), then exercises: verify `org_protected` absent → Up to 17 → verify column present + DEFAULT 0 works → Down to 16 → verify column gone → Re-Up → verify column back.

- `TestMigrate00018_PlaygroundSessions_UpDownUp` — seeds the DB to version 17 (full MigrateUp then migrateDown to 17), then exercises: verify playground columns absent + `tombstones` table absent → Up to 18 → verify `last_substantive_activity_at` back-filled, `hard_cap_at`/`idle_timeout_at` columns usable, `tombstones` table exists → Down to 17 → verify table gone + columns gone + pre-migration session row survived the table-rebuild in the Down script → Re-Up → verify all schema elements back.

**Spec note on cascading rollback**: The story spec notes that `migrateDown(t, ctx, db, "sqlite", 15)` rolls back 17 and 18 along with 16 due to the FK dependency. This is correct for rolling back past 16, but migrations 17 and 18 can be isolated from each other at the 16→17 and 17→18 boundaries respectively. The `oauth_tokens.session_id` FK column was introduced by migration 16 (not 17 or 18), so 17 and 18 can each be rolled back independently without cascading. The tests confirm this — each rolls back only its own migration cleanly.

**Verification**:
```
go test ./internal/db/... -run 'TestMigrate00017|TestMigrate00018' -v
# --- PASS: TestMigrate00017_OrgProtected_UpDownUp (0.02s)
# --- PASS: TestMigrate00018_PlaygroundSessions_UpDownUp (0.01s)

go test ./internal/db/...
# ok  jamsesh/internal/db  0.105s
```
