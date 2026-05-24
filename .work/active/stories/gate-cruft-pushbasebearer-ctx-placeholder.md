---
id: gate-cruft-pushbasebearer-ctx-placeholder
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

## Implementation notes

Chose to drop the `ctx` parameter from `pushBaseRefWithBearer` rather than wiring it through, because `runGitWithEnv` (a package-level var) does not accept a context — adding one would change the signature for all callers and is a broader refactor. Dropping the parameter removes the placeholder entirely and keeps the function signature honest.

Changes:
- `cmd/jamsesh/sessioncmd/new.go`: removed `ctx context.Context` from `pushBaseRefWithBearer` signature; removed `_ = ctx` placeholder line; updated the single call site in `newPlaygroundAction` to pass three args instead of four.
- `context` import is retained — it is used by multiple other functions in the file.
- `go build ./...` and `go test ./cmd/jamsesh/sessioncmd/...` both pass cleanly.
