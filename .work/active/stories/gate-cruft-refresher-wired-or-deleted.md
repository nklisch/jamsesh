---
id: gate-cruft-refresher-wired-or-deleted
kind: story
stage: review
tags: [cleanup, plugin]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# Unused `Refresher` token-refresh infrastructure

## Confidence
High

## Category
dead function / unused infrastructure

## Location
`cmd/jamsesh/portalclient/refresh.go:26-114`

## Evidence
```go
type Refresher struct { BaseURL string; HTTP *http.Client; group singleflight.Group }
func (r *Refresher) httpClient() *http.Client { ... }
func (r *Refresher) Refresh(ctx context.Context) error { ... }
func (r *Refresher) doRefresh(ctx context.Context) error { ... }
```

## Removal

**User directive (2026-05-18): wire it.** Path (a) is the chosen
resolution.

Wire `Refresher.Refresh` into the production
`portalclient.Client.Refresh` field at every call site:
- `cmd/jamsesh/finalizecmd/finalize.go:76`
- `cmd/jamsesh/sessioncmd/join.go:81`
- `cmd/jamsesh/sessioncmd/status.go:60`
- `cmd/jamsesh/hooks/sessionstart.go:129`
- `cmd/jamsesh/finalizecmd/finalizerun.go:61`

After this, 401s from the portal trigger the singleflight refresh path,
honoring the refresh token persisted via `state.{Read,Write}RefreshToken`.

## Implementation notes

### Helper introduced

Added `cmd/jamsesh/portalclient/wire_refresh.go` exporting a single function:

```go
func WireRefresh(client *Client)
```

It instantiates a `Refresher{BaseURL: client.BaseURL, HTTP: client.HTTP}` and
assigns `r.Refresh` to `client.Refresh`. Using the client's own `HTTP` field
means tests that inject a custom transport get the same transport in the
refresher — no divergence.

### Call sites wired

All five call sites now call `portalclient.WireRefresh(pc)` immediately after
constructing the `Client`:

- `cmd/jamsesh/finalizecmd/finalize.go` — `finalizeLocal`
- `cmd/jamsesh/sessioncmd/join.go` — `joinAction`
- `cmd/jamsesh/sessioncmd/status.go` — `statusAction`
- `cmd/jamsesh/hooks/sessionstart.go` — `buildPortalClient` helper (wired
  once; all hook paths that call this helper inherit the refresh)
- `cmd/jamsesh/finalizecmd/finalizerun.go` — `finalizeRunAction`

### Test coverage

Existing tests all pass (`go test ./cmd/jamsesh/...`):

- `portalclient.TestRefresher_Refresh_*` — cover the `Refresher` primitive in
  isolation (writes tokens, singleflight dedup, server error, missing token).
- `portalclient.TestClient_Do_401ThenSuccess` — confirms the `Client.Refresh`
  hook path end-to-end (401 → refresh → retry with new token → 200).

No new integration test added for the call-site wiring. The existing
`TestClient_Do_401ThenSuccess` test already verifies the full 401→refresh→retry
path at the `Client` level, and `TestRefresher_Refresh_WritesTokens` verifies
that `Refresher.Refresh` persists tokens correctly. Together they cover the
composed behaviour. End-to-end verification via `jamsesh status` with an
expired access token is left as a manual sanity check.
