---
id: gate-cruft-pushbasebearer-ctx-placeholder
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

# Reserved-for-future placeholder `_ = ctx` in `pushBaseRefWithBearer`

## Confidence
Medium

## Category
compatibility shim

## Location
`cmd/jamsesh/sessioncmd/new.go:430-431`

## Evidence
```go
func pushBaseRefWithBearer(ctx context.Context, baseURL, sessionID, bearer string) error {
    _ = ctx // reserved for future cancellation propagation
```

## Removal
Either wire `ctx` through `runGitWithEnv` / `exec.CommandContext` for real cancellation propagation now (it'd be a tiny change and is the documented intent), or drop the `ctx` parameter and the placeholder line. "Reserved for future" parameters tend to ossify; pick a direction.
