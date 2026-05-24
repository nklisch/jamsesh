---
id: story-cli-invite-dedupe-parseinviteemails-test
kind: story
stage: done
tags: [cleanup, test-debt]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
---

# Dedupe `TestParseInviteEmails` across `new_test.go` and `invite_test.go`

## Origin

Review of `story-epic-ephemeral-playground-cli-first-creation-invite` flagged
that the helper move (`parseInviteEmails` + `sendInvitesIfRequested` from
`new.go` → `invite.go`) left the original `TestParseInviteEmails` in
`cmd/jamsesh/sessioncmd/new_test.go:849` and added a near-identical
`TestParseInviteEmails_inviteFile` in
`cmd/jamsesh/sessioncmd/invite_test.go:235`. The implementer flagged this in
an inline comment as deferred cleanup ("can be removed in a follow-up
cleanup, but it doesn't harm correctness to have both").

## Scope

- Delete `TestParseInviteEmails` from
  `cmd/jamsesh/sessioncmd/new_test.go` (lines ~848-875)
- Rename `TestParseInviteEmails_inviteFile` →
  `TestParseInviteEmails` in `cmd/jamsesh/sessioncmd/invite_test.go`
- Remove the explanatory comment block above the renamed function
  (lines ~229-234 in `invite_test.go`)
- Verify `go test ./cmd/jamsesh/sessioncmd/ -run TestParseInviteEmails`
  still passes with one occurrence

## Acceptance

- One `TestParseInviteEmails` function in the package, living in
  `invite_test.go` (alongside the function it tests)
- `go test ./cmd/jamsesh/sessioncmd/...` clean
- `go vet ./cmd/jamsesh/...` clean

## Implementation notes

Canonical test lives in `cmd/jamsesh/sessioncmd/invite_test.go` alongside
`parseInviteEmails` in `invite.go`.

- Removed `TestParseInviteEmails` (6 cases) from `new_test.go` (was left
  behind after the helper moved from `new.go` → `invite.go`).
- Renamed `TestParseInviteEmails_inviteFile` → `TestParseInviteEmails` in
  `invite_test.go` and stripped the now-stale "intentionally duplicated"
  comment block.
- The `invite_test.go` version had one extra case
  (`"a@x.com,b@y.com,c@z.com"`) that documents the comma-joined input
  inviteAction produces; it was retained as-is.
- `go build ./...`, `go test ./...`, and `go vet ./cmd/jamsesh/...` all
  pass cleanly.

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Implementation matches the design; verification passes (Go: `go build` + `go test ./...` clean; frontend: `npm run check` 0 errors, `npm run test` 635/635, `npm run build` clean). Implementation notes accurately document what landed, including any agent decisions or land-mode confirmations.
