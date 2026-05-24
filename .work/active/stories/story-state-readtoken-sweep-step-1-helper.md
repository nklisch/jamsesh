---
id: story-state-readtoken-sweep-step-1-helper
kind: story
stage: review
tags: [plugin, refactor]
parent: feature-state-readtoken-per-session-sweep
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-24
updated: 2026-05-24
---

# Add state.ReadCurrentBearer() per-session-first / legacy-fallback helper

## Brief

Step 1 of the parent feature: add a helper that encapsulates the
per-session-first / legacy-fallback bearer read pattern that
`mcp-headers` already uses. Each remaining `state.ReadToken()`
callsite (5 of them, covered by step 2) will become a one-line
swap to `state.ReadCurrentBearer()`.

## Current state

`cmd/jamsesh/state/state.go` exposes:
- `ReadToken() (string, error)` — reads `${CLAUDE_PLUGIN_DATA}/token`
  (legacy path)
- `ReadSessionToken(sessionID string) (string, error)` — reads
  `sessions/<id>/token` (per-session path; added by bearer-storage story)

`mcp-headers` (per its post-migration behavior, see
`docs/ARCHITECTURE.md`) already implements per-session-first reads:
when `CLAUDE_SESSION_ID` matches an `instance_id` file, read the
per-session token; else fall back to the legacy `token` file.

That fallback logic lives inline in `mcp-headers` and needs to be
extracted into a reusable helper.

## Target state

```go
// ReadCurrentBearer returns the most appropriate bearer token for the
// current binary invocation. Resolution order:
//
//  1. If sessionID is non-empty AND the per-session bearer file exists
//     AND is readable, return that token.
//  2. Otherwise, fall back to ReadToken() (legacy account-wide path).
//
// Post-migration the legacy token is a `MIGRATED_TO_PER_SESSION` stub;
// callers that receive the stub should treat it as an absent token
// (the portal will reject it as a malformed bearer). Pre-migration or
// for unbound invocations (no sessionID) the legacy token is the
// canonical bearer.
//
// sessionID is typically resolved by the caller via the
// CLAUDE_SESSION_ID -> instance_id binding lookup; callers without a
// binding context pass "".
func ReadCurrentBearer(sessionID string) (string, error) {
    if sessionID != "" {
        if tok, err := ReadSessionToken(sessionID); err == nil {
            return tok, nil
        }
        // Per-session miss is non-fatal; fall through to legacy.
    }
    return ReadToken()
}
```

## Tests

`cmd/jamsesh/state/state_test.go` extended with:

- `TestReadCurrentBearer_PerSessionHit` — sessionID set, per-session file
  exists with a token: returns that token.
- `TestReadCurrentBearer_PerSessionMiss_FallsBackToLegacy` — sessionID set,
  per-session file absent: returns legacy token.
- `TestReadCurrentBearer_EmptySessionID_UsesLegacy` — no sessionID provided:
  returns legacy token.
- `TestReadCurrentBearer_PostMigrationStub_PassesThrough` — sessionID empty,
  legacy is the MIGRATED stub: helper returns the stub string (caller's
  job to interpret).

## Implementation notes

- The helper's signature mirrors `mcp-headers`' inline logic so the
  call-site swap in step 2 is mechanical.
- Per-session miss falls through silently rather than returning an error —
  this is the documented behavior of the existing mcp-headers pattern.
- The auth-write callsites (`auth/browser.go`, `auth/device.go`) are NOT
  consumers — they call WriteToken (legacy path), not ReadToken. They
  stay on the legacy path; this helper is purely for readers.

## Acceptance criteria

- [ ] `state.ReadCurrentBearer(sessionID string) (string, error)` exists
      in `cmd/jamsesh/state/state.go`.
- [ ] All 4 tests above pass.
- [ ] `go build ./...` clean.
- [ ] `go test ./cmd/jamsesh/state/...` clean.
- [ ] Existing `state.ReadToken`, `state.ReadSessionToken` signatures
      unchanged.

## Risk

**Low.** Pure addition — no existing callers touched in this story.

## Rollback

`git revert` the implementation commit.

## Implementation notes

- Added `ReadCurrentBearer(sessionID string) (string, error)` to
  `cmd/jamsesh/state/state.go` immediately after `ReadSessionToken`.
- `ReadSessionToken` returns `([]byte, error)`; the helper trims whitespace
  via `strings.TrimSpace(string(tok))` before returning, matching the
  inline trim already in `mcpheaders`.
- Four tests added to `cmd/jamsesh/state/state_test.go`:
  - `TestReadCurrentBearer_PerSessionHit` — per-session file present; returns trimmed token.
  - `TestReadCurrentBearer_PerSessionMiss_FallsBackToLegacy` — per-session absent; returns legacy token.
  - `TestReadCurrentBearer_EmptySessionID_UsesLegacy` — empty sessionID skips per-session lookup.
  - `TestReadCurrentBearer_PostMigrationStub_PassesThrough` — stub value passes through unchanged.
- `go build ./...` and `go test ./...` both clean.
