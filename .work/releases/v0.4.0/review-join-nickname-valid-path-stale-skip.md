---
id: review-join-nickname-valid-path-stale-skip
kind: story
stage: done
tags: [testing, portal, playground, tech-debt]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Unskip and fix valid-path subtests in `TestJoinPlaygroundSession_NicknameValidation`

## Source
Spawned from review of `gate-tests-join-nickname-server-side-validation`.

## Context
The 5 valid-path subtests (valid_2char, valid_24char, valid_all_digits,
valid_with_dashes, empty_server_mints) in
`internal/portal/playground/handler_test.go` are skipped pointing at a
backlog id `bug-playground-join-with-nickname-returns-410-on-fresh-session`
that does not exist. The underlying clock-injection bug WAS fixed in commit
7bfdabe (`story-fix-playground-join-handler-unit-test-clock-injection-debt`,
now archived). Sibling tests `TestJoinPlaygroundSession_Success` and
`TestJoinPlaygroundSession_WithNickname_UsesIt` pass cleanly.

## What needs to change
1. Remove the `t.Skip(...)` guard at handler_test.go:1440-1442.
2. Branch the assertions: for `wantCode == 200`, decode
   `openapi.PlaygroundSessionJoined` and assert nickname round-trips
   verbatim (or matches the wordlist pattern for the empty-server-mints
   case). For non-200, keep the existing ErrorEnvelope decode.
3. Confirm all 11 subtests pass.

## Why this is non-blocking for the parent story
The parent story's primary acceptance criterion — "server-side validation
rejects invalid nicknames" — is satisfied by the 6 invalid-path subtests
(too short, too long, has-space, has-at, has-slash, has-unicode), all of
which pass. Valid-path subtests are bonus coverage that was correctly
skipped at write-time when the join handler was broken. Now that the
handler works, the skips are stale and the test bodies need branching to
match the success shape.

## Implementation notes
Removed the `t.Skip(...)` guard (handler_test.go:1440-1442) and branched
the assertion block: for `wantCode == 200`, the test now decodes
`openapi.PlaygroundJoinResult` and asserts that a supplied nickname
round-trips verbatim or, for the server-mints case, is non-empty. For
non-200 paths the original `ErrorEnvelope` decode is retained. All 11
subtests (6 invalid + 5 valid) pass cleanly with no new bugs surfaced —
no parking required.
