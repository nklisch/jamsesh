---
id: gate-tests-cli-open-failing-opener-contract
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

# Original `--open` browser-launch failure contract is not directly tested

## Priority
Critical

## Spec reference
Item: `feature-cli-jam-open-in-browser-cli-open-flag`
Acceptance criterion: "`--open` with a failing opener still exits 0."

## Gap type
missing test for error case

## Suggested test
```go
// Stub openURL to return an error for new/join --open; assert command returns nil
// and the token-free URL remains available in output.
```

## Test location (suggested)
`cmd/jamsesh/sessioncmd/new_test.go`, `cmd/jamsesh/sessioncmd/join_test.go`

