---
id: epic-cloud-native-deploy-operational-polish-readyz
kind: story
stage: implementing
tags: [infra, portal]
parent: epic-cloud-native-deploy-operational-polish
depends_on: []
release_binding: null
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

- [ ] `GET /readyz` returns 200 + `{"status":"ready",...}` when DB
  ping and storage stat both succeed.
- [ ] `GET /readyz` returns 503 + `{"status":"not_ready",...}` when
  any check fails, with per-check `ok` and `error` fields.
- [ ] Checks run in parallel — N checks each taking T total no more
  than ~T+overhead, not N*T.
- [ ] Each check has a 2-second timeout; exceeded checks report
  `"error": "timeout"`.
- [ ] `/healthz` continues to return its existing 200 response unchanged.
- [ ] Unit tests for `probes.Handler` cover: all-ok, one-fail,
  all-fail, timeout, parallel timing.
