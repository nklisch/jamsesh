---
id: gate-cruft-status-test-stale-setupstatusenv-comment
kind: story
stage: review
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# status_test.go: setupStatusEnv "backward compat" doc comment is misleading

## Confidence
Medium

## Category
stale comment

## Location
`cmd/jamsesh/sessioncmd/status_test.go:586-589`

## Evidence
```go
// setupStatusEnv is kept for backward compat with any tests that still need it
// (e.g. tests in the fork/mode files that call into setupStatusEnv via
// cross-file sharing). It creates both the legacy token file and a per-session
// token so both old-style and new-style status work in tests.
func setupStatusEnv(t *testing.T, srvURL, sessionID, orgID, yourRef string) string {
```

The function IS actively used inside `status_test.go` itself (lines 656 and 707 in `TestStatusAction_jsonOutputLegacy` and a sibling test). The "kept for backward compat with tests in the fork/mode files" rationale is stale — `grep -rn 'setupStatusEnv' --include="*.go" cmd/jamsesh/sessioncmd/` shows callers only in this same file, not the fork/mode files. The comment overstates the function's scope and creates the false impression that the helper is preserved for cross-file consumers that don't actually exist.

## Removal
Replace the four-line comment (lines 586-589) with a concise one-liner reflecting reality:

```go
// setupStatusEnv writes both the legacy token file and a per-session token
// into a temp CLAUDE_PLUGIN_DATA dir so status tests cover both lookup paths.
```

No code changes — just the comment. Run `go vet ./cmd/jamsesh/sessioncmd/...` to confirm.

## Implementation notes
Replaced the four-line stale "backward compat" comment on `setupStatusEnv` in `cmd/jamsesh/sessioncmd/status_test.go` with the accurate two-line description. No code changes. `go test ./cmd/jamsesh/sessioncmd/...` passes.
