---
id: gate-tests-migrate-17-18-up-down-up-isolated
kind: story
stage: drafting
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
