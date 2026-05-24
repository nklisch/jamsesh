---
id: gate-tests-join-nickname-server-side-validation
kind: story
stage: implementing
tags: [testing, portal, playground, validation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Nickname server-side validation for `JoinSession` (2-24 chars, letters/digits/dashes) untested

## Priority
High

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-rest-endpoints`

Acceptance criterion: Epic strategic decision: "Joiners may keep the suggestion or pick a custom handle (2-24 chars, letters / digits / dashes) at the join screen."

## Gap type
missing test for boundary + invalid input

## Suggested test
```go
func TestJoinPlaygroundSession_NicknameValidation(t *testing.T) {
    // Table-driven:
    //   too short (1), too long (25), invalid chars (space, @, /, unicode),
    //   all-numbers, empty (server-mints), valid edge (2, 24 chars).
    // Assert 400 for invalid, 200 with nickname for valid.
}
```
Existing `TestJoinPlaygroundSession_WithNickname_UsesIt` only covers happy
path.

## Test location (suggested)
`internal/portal/playground/handler_test.go`
