---
id: epic-cloud-native-deploy-operational-polish-readyz
kind: story
stage: done
tags: [infra, portal]
parent: epic-cloud-native-deploy-operational-polish
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Operational Polish — `/readyz` endpoint

## Scope

Add a `/readyz` HTTP endpoint to the portal that probes database
connectivity and storage-root accessibility, returning 200 ready or
503 not-ready with a structured JSON body.

Implements **Unit 1** of `epic-cloud-native-deploy-operational-polish`.
See parent feature body for full design rationale.

## Files

New:
- `internal/portal/probes/probes.go`
- `internal/portal/probes/probes_test.go`

Edit:
- `internal/portal/router/router.go` — mount `/readyz`; add optional
  `ReadyzCheck` field (or similar) to `Deps`
- `cmd/portal/main.go` — wire DB ping + storage stat probes into the
  router Deps

## Interface

```go
// internal/portal/probes/probes.go
package probes

type Check struct {
    Name string
    Fn   func(ctx context.Context) error
}

func Handler(checks []Check) http.Handler
```

Response body shape:

```json
{
  "status": "ready",
  "checks": [
    {"name": "db", "ok": true},
    {"name": "storage", "ok": true}
  ]
}
```

On any check failure: HTTP 503, `status: "not_ready"`, and each
failed check carries `"error": "<message>"`.

## Acceptance criteria

- [x] `GET /readyz` returns 200 + `{"status":"ready",...}` when DB
  ping and storage stat both succeed.
- [x] `GET /readyz` returns 503 + `{"status":"not_ready",...}` when
  any check fails, with per-check `ok` and `error` fields.
- [x] Checks run in parallel — N checks each taking T total no more
  than ~T+overhead, not N*T.
- [x] Each check has a 2-second timeout; exceeded checks report
  `"error": "timeout"`.
- [x] `/healthz` continues to return its existing 200 response unchanged.
- [x] Unit tests for `probes.Handler` cover: all-ok, one-fail,
  all-fail, timeout, parallel timing.

## Implementation notes

Fresh implementation. All acceptance criteria verified by automated tests.

**New package** `internal/portal/probes`:
- `Check` struct + `Handler([]Check) http.Handler` as designed
- Checks run in parallel via goroutines + `sync.WaitGroup`; each gets a
  `context.WithTimeout` derived from the request context (2s ceiling)
- Timeout detection: `ctx.Err() != nil` after the check returns — the
  error message is replaced with `"timeout"` in that case
- `"error"` field is `omitempty` so passing checks produce clean JSON
- 6 unit tests: all-ok, one-fail, all-fail, timeout, parallel timing, empty

**Router** (`internal/portal/router/router.go`):
- Added `ReadyzChecks []probes.Check` to `Deps`
- `/readyz` is registered only when `len(d.ReadyzChecks) > 0`; empty
  `Deps{}` (used in existing tests) still 404s on `/readyz`

**Store interface** (`internal/db/store/store.go`):
- Added `Ping(ctx context.Context) error` to `Store`
- `sqliteAdapter.Ping` wraps `*sql.DB.PingContext`
- `postgresAdapter.Ping` wraps `pgxpool.Pool.Ping`
- Updated `handlerauth_test.go` `stubStore` to satisfy the extended interface

**main.go wiring**: `ReadyzChecks` slice with `db` (store.Ping) and
`storage` (os.Stat on cfg.Storage) probes.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- Inside the allOK check loop, `for _, r := range results` shadows the outer `r *http.Request` parameter. Cosmetic; no actual use of the outer `r` after that point.

**Notes**: Clean implementation. Probes package has good separation (Check struct + Handler builder), thorough doc comments. Per-check `context.WithTimeout` derives from request context (cancels if client disconnects — correct). Timeout detection via `ctx.Err()` after Fn returns handles both DeadlineExceeded and Canceled as "timeout" — acceptable user-facing meaning.

Tests are external (`package probes_test`), assert on behavioral contracts (status code, JSON envelope, per-check fields) not implementation details. 6 tests covering all-ok, one-fail, all-fail, timeout, parallel timing, and empty list.

Store interface change (`Ping(ctx) error`) was propagated to all implementations — verified the only stubStore in `internal/portal/handlerauth/handlerauth_test.go` was updated; no other mock/fake stores exist in the codebase.

`/readyz` not yet documented in SELF_HOST.md — belongs to the sibling docs story, not a finding here.
