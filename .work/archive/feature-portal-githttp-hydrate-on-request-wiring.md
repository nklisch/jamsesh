---
id: feature-portal-githttp-hydrate-on-request-wiring
kind: story
stage: done
tags: [bug, portal, clustered, git]
parent: feature-portal-githttp-hydrate-on-request
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-19
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

## Implementation Notes

### Files touched

| File | Change |
|------|--------|
| `internal/portal/httperr/httperr.go` | Added `ErrObjectStorageUnavailable(cause error) *Error` constructor — 503 + `Retry-After: 1` + `dep.object_storage_unavailable` code. |
| `internal/portal/githttp/handler.go` | Added unexported `lifecycleAcquirer` interface + exported `Lifecycle lifecycleAcquirer` field + `acquireForGitRequest` helper. Used a local interface instead of the concrete `*objectstore.LifecycleManager` so test stubs don't need to import the full objectstore package. |
| `internal/portal/githttp/info_refs.go` | Inserted `acquireForGitRequest` call between URL-param extraction and `Storage.RepoPath`. Returns 503 on failure. |
| `internal/portal/githttp/upload_pack.go` | Same insertion, same error shape. |
| `internal/portal/githttp/receive_pack.go` | Inserted `acquireForGitRequest` after body parse + command-list parse, immediately before `buildValidationRepo` / `Storage.RepoPath`. This placement means the hydration failure cannot fire after the content-type check (400) or body-size limit (413), avoiding premature 503 for malformed requests. |
| `cmd/portal/main.go:685` | Added `Lifecycle: objLifecycle,` to the `gitHandler` literal. `objLifecycle` is nil in single-instance mode — existing nil-guard in the helper makes this a no-op. |
| `internal/portal/githttp/handler_test.go` | Added `TestLifecycle_NilNoOp` and `TestLifecycle_AcquireError` with a `lifecycleStub` that satisfies the unexported `lifecycleAcquirer` interface from outside the package via Go's structural typing. |

### Error constructor choice

`deperr.WrapObjectStorage` does not exist. Rather than adding a new deperr
sentinel (which would require wiring `translate.go` too), the 503 is emitted
directly via a new `httperr.ErrObjectStorageUnavailable` constructor. This
keeps the diff minimal — one constructor, no new sentinel chain.

### Fencing-token plumbing finding

This is scenario (c) from the design: the `receive-pack` subprocess env
(`cmd.Env` around line 178) does NOT currently set a fencing-token env var.
Pre-receive validation runs go-git in-process (not via the subprocess) and
does not consult the fencing token at all. So single-mode tests pass with
fencing token = 0, and the gap is pre-existing. After this fix the handle
from `AcquireForRequest` is discarded (`_, err := ...`) — the fencing token
is still not plumbed into the subprocess env. This is acceptable: the
minimum acceptance (503 fix) is met. Plumbing the fencing token into the
subprocess env for pre-receive is a separate gap, left for a follow-up story.

### Local test outcomes

```
go test ./internal/portal/githttp/...   PASS (all 29 tests, 1.07s)
go test ./internal/portal/postreceive/... PASS
go vet ./...                            clean
go build ./cmd/portal                   clean
```

### Deviations from design

- Used `httperr.ErrObjectStorageUnavailable` (new constructor) instead of
  `deperr.WrapObjectStorage` + `httperr.WriteFromError` (which would require
  adding a new deperr sentinel). Effect is identical: 503 + `Retry-After: 1`
  + JSON body. Kept diff focused.
- `Handler.Lifecycle` is typed as the local `lifecycleAcquirer` interface
  rather than `*objectstore.LifecycleManager` (concrete). This is strictly
  better: avoids a circular-import risk, makes tests simpler, and
  `*objectstore.LifecycleManager` satisfies the interface automatically.
