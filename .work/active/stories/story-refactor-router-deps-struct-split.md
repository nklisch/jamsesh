---
id: story-refactor-router-deps-struct-split
kind: story
stage: review
tags: [portal, refactor]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Split portal router Deps god-struct into role-scoped sub-structs

## Brief

`internal/portal/router/router.go` defines a `Deps` struct that
accumulates 10+ fields covering TLS posture, proxy-header trust,
mount hooks for ~5 subsystems, body limits, readyz checks, and the
metrics handler. The struct is passed wholesale to `New` and every
subsystem either ignores most of it or reaches across unrelated
concerns.

Surfaced by a discovery-mode `/agile-workflow:refactor-design` scan.

## Current state

```go
type Deps struct {
    TLSMode            string
    TrustProxyHeaders  bool
    MountAPI           func(r chi.Router)
    MountMCP           func(r chi.Router)
    MountGit           func(r chi.Router)
    MountWS            func(r chi.Router)
    MountFinalize      func(r chi.Router)
    APIBodyLimitBytes  int64
    ReadyzChecks       []probes.Check
    MetricsHandler     http.Handler
    // ...
}
```

## Target state

Group fields into role-scoped sub-structs that callers compose:

```go
type Security struct {
    TLSMode           string
    TrustProxyHeaders bool
}

type Mounts struct {
    API      func(r chi.Router)
    MCP      func(r chi.Router)
    Git      func(r chi.Router)
    WS       func(r chi.Router)
    Finalize func(r chi.Router)
}

type Probes struct {
    Ready []probes.Check
}

type Deps struct {
    Security Security
    Mounts   Mounts
    Probes   Probes
    Metrics  http.Handler
    APIBodyLimitBytes int64
}
```

`New(Deps)` keeps the same external contract — callers update field
paths but the wiring shape is unchanged.

## Acceptance criteria

- `internal/portal/router/router.go` `Deps` is partitioned into the
  sub-structs above (or equivalent named partition produced during
  implementation).
- Every call site in `cmd/portal/main.go` and tests is updated.
- `go build ./...` clean.
- `go test ./internal/portal/router/...` and the portal integration
  smoke pass.

## Notes

Behavior-preserving — pure naming + nesting change, no logic moves.

## Implementation notes

Partitioned `Deps` into four role-scoped sub-structs as specified:

- `Security` — `TLSMode`, `TrustProxyHeaders`
- `Mounts` — `API`, `MCP`, `Git`, `WS`, `UI`, `Test` (renamed from `Mount*` prefix; parent struct name conveys "mount")
- `Probes` — `Ready` (renamed from `ReadyzChecks`)
- `Metrics` — `Handler`, `Token`, `Registry` (renamed from `MetricsHandler`, `MetricsToken`, `MetricsRegistry`)
- `APIBodyLimitBytes` stays at top-level `Deps` — no natural group

Callers updated:
- `internal/portal/router/router.go` — struct definitions + `New()` body
- `internal/portal/router/router_test.go`
- `internal/portal/router/security_headers_test.go`
- `internal/portal/router/metrics_auth_test.go`
- `internal/portal/router/body_limits_test.go`
- `internal/portal/router/body_limits_api_test.go`
- `internal/portal/server/server_test.go` — extra caller beyond listed; uses `TrustProxyHeaders`
- `cmd/portal/main.go`
- `cmd/portal/test_clock_advance_e2e_test.go`
- `cmd/portal/test_clock_advance_prod_test.go`

`go build ./...` clean. `go test ./...` — all 57 packages pass.
