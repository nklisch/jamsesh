---
id: story-state-readtoken-sweep-step-2-callsites
kind: story
stage: review
tags: [plugin, refactor]
parent: feature-state-readtoken-per-session-sweep
depends_on: [story-state-readtoken-sweep-step-1-helper]
release_binding: null
gate_origin: refactor-design
created: 2026-05-24
updated: 2026-05-24
---

# Sweep 5 state.ReadToken callsites to state.ReadCurrentBearer

## Brief

Step 2 of the parent feature. Replace `state.ReadToken()` with
`state.ReadCurrentBearer(sessionID)` at every read callsite in the
binary so post-migration callers don't receive the
`MIGRATED_TO_PER_SESSION` stub. Auth-write callsites stay on the
legacy path (they run pre-binding; that's intentional).

Depends on step 1 (`state.ReadCurrentBearer` helper).

## Callsites (verified 2026-05-24)

| File | Line | Current call | Session-id source |
|---|---|---|---|
| `cmd/jamsesh/portalclient/client.go` | 93 | `tok, err := state.ReadToken()` (in `attachBearer`) | Hot path — needs the binding session. The portal client is invoked per-request; the bound sessionID is available in the client's struct (or via the caller). |
| `cmd/jamsesh/sessioncmd/new.go` | 282 | `token, err := state.ReadToken()` (git Basic-auth push) | The newly-created session ID; available in scope. |
| `cmd/jamsesh/sessioncmd/new.go` | 349 | `if _, err := state.ReadToken(); err != nil {` (auth check in `buildPortalClient`) | Pre-binding auth check — sessionID is empty here, helper falls back to legacy. |
| `cmd/jamsesh/sessioncmd/fork.go` | 62 | `token, err := state.ReadToken()` (MCP fork call) | The current bound session ID; available in scope. |
| `cmd/jamsesh/sessioncmd/join.go` | 70 | `tok, err := state.ReadToken()` (auth check on join) | Pre-binding (just before binding completes) — pass empty sessionID; falls back to legacy. |

## Per-callsite swap

```go
// Before
tok, err := state.ReadToken()

// After
tok, err := state.ReadCurrentBearer(sessionID)  // sessionID per the table above
```

For pre-binding callsites (new.go:349, join.go:70) pass `""` — the helper
falls back to legacy, preserving the existing behavior.

For post-binding callsites (client.go:93, new.go:282, fork.go:62) thread
the session ID through. If the caller's struct doesn't carry it, add a
field/parameter.

## Acceptance criteria

- [ ] All 5 callsites use `state.ReadCurrentBearer(sessionID)`.
- [ ] `git grep -n "state.ReadToken" -- 'cmd/'` returns ONLY the two
      auth-write callsites (`auth/browser.go:187`, `auth/device.go:106`),
      which call WriteToken, not ReadToken — actually verify the grep
      returns NO `ReadToken` callers besides the helper's internal
      fallback. The auth-write files use `WriteToken`, not `ReadToken`.
- [ ] `go build ./...` clean.
- [ ] `go test ./cmd/jamsesh/...` clean.
- [ ] `go test ./...` clean.
- [ ] At least one integration-style test that exercises a post-migration
      path (legacy token is the stub) ends up rejecting with a clear
      error rather than sending the stub as a Bearer header. This may
      already exist; if not, add a unit test on the calling function.

## Implementation notes

- The portal client's `attachBearer` is the hot-path callsite. If its
  struct doesn't already track the bound session ID, add a field
  populated at construction time. The exact wiring is implementer's
  judgment.
- For sessioncmd callsites, the session ID is typically available from
  the command's flags or the just-bound state — read the function in
  context to find the right value.
- This story is a pure refactor (behavior is preserved). The
  `MIGRATED_TO_PER_SESSION` stub being rejected by the portal is the
  CURRENT behavior; this refactor doesn't change that — it just routes
  reads through the per-session path FIRST, so the stub is never the
  primary read for a bound session.

## Risk

**Low.** Mechanical sweep using a helper that's already tested in step 1.
The compiler catches every callsite that needs a sessionID parameter.

## Rollback

`git revert` the implementation commit. Step 1 (the helper) stays in
place as additive surface — no callers need it but its presence isn't
harmful.

## Sequencing

`depends_on: [story-state-readtoken-sweep-step-1-helper]` — needs the
helper to exist before the sweep can use it.

## Implementation notes

Swept 6 callsites total (story table listed 5; grep also found
`cmd/jamsesh/mcpheaders/mcpheaders.go:44` which was swept in the same pass).

### Callsite decisions

| File | Session-ID passed | Rationale |
|---|---|---|
| `cmd/jamsesh/portalclient/client.go:93` | `c.SessionID` (new struct field) | Added `SessionID string` field to `Client`; zero value `""` gives legacy fallback for all existing callers unchanged |
| `cmd/jamsesh/sessioncmd/new.go:282` | `sessionID` (func param) | Just-created session ID is the parameter to `pushBaseRef` — threaded directly |
| `cmd/jamsesh/sessioncmd/new.go:349` | `""` | Pre-binding auth check in `buildPortalClient`; no session yet |
| `cmd/jamsesh/sessioncmd/fork.go:62` | `sessionID` | Returned by `ResolveSession()` earlier in the same function |
| `cmd/jamsesh/sessioncmd/join.go:70` | `""` | Pre-binding auth check; session ID not yet resolved |
| `cmd/jamsesh/mcpheaders/mcpheaders.go:44` | `""` | Fallback branch reached only when no bound session; pre-binding |

### portalclient.Client wiring

The `Client.SessionID` field (zero value `""`) means all existing
`&portalclient.Client{BaseURL: ...}` literal sites in `hooks/`,
`finalizecmd/`, and tests pick up legacy-fallback behavior automatically
with no code changes needed at those callsites.

### Tests added

Two new tests in `cmd/jamsesh/portalclient/client_test.go`:
- `TestClient_SessionID_UsesPerSessionToken` — verifies that a bound client
  sends the per-session token and NOT the `MIGRATED_TO_PER_SESSION` stub.
- `TestClient_NoSessionID_UsesLegacyToken` — verifies the pre-binding path
  (empty SessionID) still uses the legacy account-wide token.

### Verification

```
go build ./...                          # clean
go test ./...                           # all pass
git grep -n "state.ReadToken" -- 'cmd/' # returns nothing
```
