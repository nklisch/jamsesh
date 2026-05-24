---
id: gate-cruft-userpromptsubmit-test-stale-dir-captures
kind: story
stage: drafting
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# Stale `_ = dir` placeholders after refactored test setup

## Confidence
Medium

## Category
passthrough wrapper

## Location
`cmd/jamsesh/hooks/userpromptsubmit_test.go:157, 208`

## Evidence
```go
dir := setupHookEnv(t, "http://placeholder", sessionID, orgID, ref, accountID)
...
srv := httptest.NewServer(mux)
defer srv.Close()
t.Setenv("JAMSESH_PORTAL_URL", srv.URL)
_ = dir
```

## Removal
`dir` is captured from `setupHookEnv(...)` but never read in either test. Replace `dir := setupHookEnv(...)` with `setupHookEnv(...)` (or `_ = setupHookEnv(...)` if go-vet objects) and drop the `_ = dir` line. If `setupHookEnv` is meant to return a value that callers must reference, this signals a refactor opportunity in the helper itself.
