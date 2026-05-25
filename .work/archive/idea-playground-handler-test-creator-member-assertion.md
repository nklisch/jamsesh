---
id: idea-playground-handler-test-creator-member-assertion
kind: story
stage: done
tags: [portal, playground, testing]
parent: feature-playground-hardening
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-25
---

# Assert creator member row persists in CreatePlaygroundSession_RepoCreateFails

## Origin

Surfaced during review of
`story-playground-server-hardening-handler-test-coverage` (review verdict:
approve with comments).

## Problem

`TestCreatePlaygroundSession_RepoCreateFails_ReturnsError` (in
`internal/portal/playground/handler_test.go:945-975`) asserts the orphaned
**session** row persists after `CreateRepo` fails, but does not assert the
**creator member row** also persists. The design (Unit 3 in
`feature-playground-server-hardening`, around line 358-361) explicitly called
for both:

> "the destruction sweep cleans by session_id. Test asserts both rows persist
>  via env.s.GetSession and env.s.GetSessionMember."

The current code path:
1. Insert session row (TX)
2. Issue bearer (TX)
3. Add session_member row (TX)
4. CreateRepo (disk op, post-TX)

When step 4 fails, the test should confirm steps 1-3's rows all remain so the
destruction sweep can find them by session_id. Today it only confirms step 1.

## Fix

Add a `s.GetSessionMember` lookup after the existing `ListExpiredPlaygroundSessions`
check that asserts the creator member row remains. Roughly 8 lines added.

## Acceptance

- The test asserts both the session row AND the creator member row remain in
  the store after `CreateRepo` returns an error.
- Test still passes against both SQLite and (when env set) Postgres.

## Implementation notes

- Extended `TestCreatePlaygroundSession_RepoCreateFails_ReturnsError` with a
  second assertion: after the existing `ListExpiredPlaygroundSessions` check
  confirms the orphan session row remains, call
  `ListAnonymousSessionMemberIDs(ctx, playground.ReservedOrgID, orphanSessID)`
  and assert at least one member ID is returned. This pins that step 3
  (AddSessionMember as "creator") ran successfully before step 4 (CreateRepo)
  failed.
- Promoted the prior `t.Error` on empty sessions to `t.Fatal` so the test
  short-circuits cleanly if the session row is missing — otherwise the
  subsequent member-list call would panic on `sessions[0]`.

Verified: `go test ./internal/portal/playground/... -count 1 -run RepoCreateFails_ReturnsError` passes.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Targeted test extension that pins step-3 persistence. Promoting `t.Error` → `t.Fatal` before the subsequent member-list call is correct — prevents a panic on `sessions[0]`. The assertion uses `ListAnonymousSessionMemberIDs` (the same call destruction makes for anon-account cleanup), tying the test to the actual recovery path.
