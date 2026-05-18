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
Either (a) wire `Refresher.Refresh` into the production
`portalclient.Client.Refresh` field at every call site
(`finalize.go:76`, `sessioncmd/join.go:81`, `sessioncmd/status.go:60`,
`hooks/sessionstart.go:129`, `finalizerun.go:61`) so 401s actually
trigger refresh, or (b) delete `refresh.go`, `state.ReadRefreshToken`,
and `state.WriteRefreshToken` if auto-refresh is intentionally deferred.
The current state — sophisticated singleflight refresh logic tested but
never wired — is the worst of both worlds and contradicts the
epic-cc-plugin notes that claim refresh ships in binary-foundation.
