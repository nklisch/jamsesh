---
id: gate-cruft-worker-test-createplaygroundsession-helper-unused
kind: story
stage: implementing
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# worker_test.go: createPlaygroundSession helper has no callers

## Confidence
High

## Category
dead function

## Location
`internal/portal/playground/worker_test.go:48-75`

## Evidence
```go
// createPlaygroundSession is a helper that creates a playground session
// directly in the store with configurable hard_cap_at and idle_timeout_at.
func createPlaygroundSession(t *testing.T, ctx context.Context, s store.Store, svc tokens.Service, now time.Time, hardCap, idleTimeout time.Duration) store.Session {
	t.Helper()
	...
}
```

`deadcode -test ./internal/portal/playground/...` reports the helper unreachable. The only same-named function in the repo is in `tests/e2e/golden/playground_abandonment_destruction_sweep_test.go` — a different package with a different signature (uses HTTP, not direct store writes). The unit-test helper is genuinely orphaned, likely abandoned during the worker test rework.

Also dead: `randHexTest` (lines 77-86), which is only called by `createPlaygroundSession` and has no other callers. Confirm with `grep -n "randHexTest" internal/portal/playground/worker_test.go` — only the declaration and the call inside `createPlaygroundSession` itself.

## Removal
Delete `createPlaygroundSession` (lines 48-75) and `randHexTest` (lines 77-86) along with their docstrings. Run `go vet ./internal/portal/playground/... && go test ./internal/portal/playground/...` to confirm no fallout. Remove any imports that become unused (likely `context`, `time`, `store`, `tokens` — check by running `goimports -l` or letting the compiler complain).
