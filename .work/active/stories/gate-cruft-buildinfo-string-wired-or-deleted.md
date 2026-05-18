---
id: gate-cruft-buildinfo-string-wired-or-deleted
kind: story
stage: review
tags: [cleanup, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# `buildinfo.String()` documented as `--version`/`/healthz` consumer but never called

## Confidence
High

## Category
dead function

## Location
`internal/buildinfo/buildinfo.go:21-26`

## Evidence
```go
// String returns a human-readable version+commit string suitable for
// --version flags and /healthz responses.
func String() string { return Version + " (" + Commit + ")" }
```

## Removal
`cmd/jamsesh/main.go:26` reads `buildinfo.Version` directly, never calls
`String()`. No `/healthz` handler references it either. Either wire
`String()` into the actual `--version` / `/healthz` surfaces or delete
it (and drop the stale doc-comment promise).

## Implementation notes

Chose to wire `buildinfo.String()` into both documented surfaces:

1. **CLI `--version`** (`cmd/jamsesh/main.go`): Changed `Version: buildinfo.Version` to `Version: buildinfo.String()`. The `cli/v3` framework renders this value when the user passes `--version`, so users now see `"dev (unknown)"` in development and `"v1.2.3 (abc1234)"` in production builds.

2. **`/healthz` response** (`internal/portal/router/router.go`): Switched from a hard-coded `{"status":"ok"}` byte literal to `json.NewEncoder(w).Encode(...)` with a `map[string]string{"status": "ok", "version": buildinfo.String()}`. Added `encoding/json` and `jamsesh/internal/buildinfo` imports.

3. **Test update** (`internal/portal/router/router_test.go`): Added an assertion that `body["version"]` is non-empty in `TestHealthz` — previously the test didn't assert on the new field, which would have let the wire silently regress.

All packages build cleanly (`go build ./...`) and the target test suites pass (`internal/buildinfo`, `internal/portal/router`, `cmd/...`).
