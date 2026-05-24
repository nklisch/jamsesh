---
id: gate-tests-reserved-slug-conflict-main-exit-1
kind: story
stage: review
tags: [testing, portal, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Reserved-slug-conflict `cmd/portal/main.go` exit-1 path is not tested

## Priority
High

## Spec reference
Item: `feature-epic-ephemeral-playground-reserved-org`

Acceptance criterion: Unit 4 AC: "Pre-existing unprotected org with slug `playground`: returns `ErrReservedSlugConflict`, main exits 1 with a clear error." SELF_HOST.md documents this behavior.

## Gap type
missing test for e2e-seam (function tested in isolation; main wiring untested)

## Suggested test
```go
func TestMain_PlaygroundEnabledWithUnprotectedSlugCollision_Exits1(t *testing.T) {
    // Start portal binary subprocess: PlaygroundEnabled=true,
    // seeded with an unprotected org slug='playground'.
    // Assert exit code 1 + stderr contains "reserved slug" + remediation hint.
}
```
`TestProvisionReservedOrg_UnprotectedSlugConflict` validates the function
return but not the `os.Exit(1)` operators see.

## Test location (suggested)
`cmd/portal/main_test.go` (new file)

## Implementation notes

Created `cmd/portal/main_test.go` with
`TestMain_PlaygroundEnabledWithUnprotectedSlugCollision_Exits1`. The test:

1. Builds the portal binary via `go build jamsesh/cmd/portal` into a
   `t.TempDir()` — no build tags so it runs on every `go test ./cmd/portal/...`.
2. Opens an on-disk SQLite DB at a temp path, runs migrations via `db.Open`,
   then inserts an unprotected org with slug="playground" using `store.CreateOrg`.
3. Spawns the binary with `JAMSESH_PLAYGROUND_ENABLED=true`,
   `JAMSESH_DB_DRIVER=sqlite`, and `JAMSESH_DB_DSN=<temp path>`.
   `JAMSESH_LOG_FORMAT=text` makes the log line easier to substring-match.
4. Asserts exit code 1.
5. Asserts combined output contains `"reserved slug"` (from the slog.Error
   message and `ErrReservedSlugConflict.Error()`).
6. Asserts combined output contains `"JAMSESH_PLAYGROUND_ENABLED=false"`
   (the remediation hint from the main.go slog.Error call).

`go build ./...` and `go test ./cmd/portal/...` both pass. Test runtime ~1.6s
(dominated by the `go build` of the portal binary).
