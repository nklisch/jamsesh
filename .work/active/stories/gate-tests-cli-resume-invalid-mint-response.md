---
id: gate-tests-cli-resume-invalid-mint-response
kind: story
stage: implementing
tags: [testing, cli]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: tests
created: 2026-05-31
updated: 2026-05-31
---

# CLI mint/open helper invalid responses are not tested

## Priority
Critical

## Spec reference
Item: `epic-cli-browser-session-resume-cli-handoff-mint-open-adopt`
Acceptance criterion: "Empty `ResumeUrl` / `SessionId` mismatch -> error before opening anything."

## Gap type
missing test for error case

## Suggested test
```go
// Return 200 from /api/session-resumes with empty ResumeUrl or mismatched
// SessionId; assert openSilent/openURL are not called and no token is printed.
```

## Test location (suggested)
`cmd/jamsesh/sessioncmd/resume_test.go`

