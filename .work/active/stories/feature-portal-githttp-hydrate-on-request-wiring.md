---
id: feature-portal-githttp-hydrate-on-request-wiring
kind: story
stage: implementing
tags: [bug, portal, clustered, git]
parent: feature-portal-githttp-hydrate-on-request
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Wire LifecycleManager into git smart-HTTP handler

## Scope

Single-story implementation of the parent feature. Five files touched:

1. `internal/portal/githttp/handler.go` — add `Lifecycle *objectstore.LifecycleManager` field + `acquireForGitRequest` helper.
2. `internal/portal/githttp/info_refs.go` — call helper at entry, return 503 on hydration failure.
3. `internal/portal/githttp/upload_pack.go` — same.
4. `internal/portal/githttp/receive_pack.go` — same. Verify fencing-token plumbing still works.
5. `cmd/portal/main.go` line ~671 — add `Lifecycle: objLifecycle` to the `gitHandler` literal.

Plus a unit test in `internal/portal/githttp/handler_test.go` (or sibling) that
verifies nil-Lifecycle no-ops and that a stubbed Lifecycle returning an error
produces a 503 + `Retry-After: 1` response.

## Implementation pointers

- Parent feature: `.work/active/features/feature-portal-githttp-hydrate-on-request.md` — full design, code shapes, acceptance criteria.
- Existing pattern reference: `internal/portal/postreceive/emitter.go:97-110` — how Emitter checks-and-calls Lifecycle.
- LifecycleManager API: `internal/portal/storage/objectstore/lifecycle.go` — `AcquireForRequest(ctx, sessionID) (lease.Handle, error)`.
- deperr usage: `internal/portal/auth/magic_link.go` for the `httperr.WriteFromError` + `deperr.Wrap*` translate pattern.

## Acceptance criteria

See parent feature. Key local checks:

- [ ] `go test ./internal/portal/githttp/...` passes.
- [ ] `go vet ./...` clean.
- [ ] `go build ./cmd/portal` clean.
- [ ] All four sites of the wiring change reviewed once; no copy-paste drift
      in error handling between info_refs / upload_pack / receive_pack.
- [ ] receive_pack.go's fencing-token env plumbing for pre-receive still
      works — confirm by reading the existing flow; no functional change to
      single-mode tests is expected.
- [ ] On the next push to origin, the e2e workflow shows the ~12 clustered
      tests passing (the canary). If not all clear, surface the remaining
      failures in implementation notes — they may be downstream of separate
      bugs (lease-helper IP mapping, metrics format, finalize fetch-token
      route) that this story does NOT address.

## Notes

- This is the FIRST CALL of LifecycleManager.AcquireForRequest from outside
  the Emitter. The function's semantics (idempotent per pod, owns handle
  lifetime) are designed for exactly this pattern.
- Single-mode tests are unaffected — `Lifecycle == nil` short-circuit in
  the helper means the new code path is dead for the default deployment.
- Don't refactor existing handler structure beyond inserting the acquire
  call. Keep the diff focused; the e2e suite is the regression guard.
