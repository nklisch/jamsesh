---
id: gate-tests-receivepack-stdio-failure-status
kind: story
stage: implementing
tags: [testing]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: tests
created: 2026-05-31
updated: 2026-05-31
---

# Receive-pack stdin/stdout failure contract is only covered as "not 200"

## Priority
Critical

## Spec reference
Item: `epic-bug-squash-handler-error-classification`
Acceptance criterion: "A simulated stdout-read truncation / our-side stdin read error -> 500 (not 200)."

## Gap type
missing test for error case

## Suggested test
```go
// Inject receive-pack stdout-copy or stdin-copy failure and assert HTTP 500,
// while keeping git-level report-status rejection at HTTP 200.
```

## Test location (suggested)
`internal/portal/githttp/receive_pack_test.go`

