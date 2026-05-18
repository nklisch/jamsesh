---
id: gate-cruft-refresher-wired-or-deleted
kind: story
stage: implementing
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
