---
id: gate-tests-resume-token-raw-value-log-redaction
kind: story
stage: implementing
tags: [testing, security]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: tests
created: 2026-05-31
updated: 2026-05-31
---

# Resume-token raw-value logging contract is not asserted

## Priority
Critical

## Spec reference
Item: `epic-cli-browser-session-resume-portal-contract`
Acceptance criterion: "token never logged" / "assert the token is absent from logs"

## Gap type
missing test for security error case

## Suggested test
```go
// Install a capture logger, mint and exchange a resume token, force failure
// paths, then assert logs contain neither the raw resume token nor "#rt=".
```

## Test location (suggested)
`internal/portal/sessionresume/mint_test.go`, `internal/portal/sessionresume/exchange_test.go`

