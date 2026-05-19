---
id: feature-portal-githttp-hydrate-on-request
kind: feature
stage: implementing
tags: [bug, portal, clustered, git]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Wire LifecycleManager.AcquireForRequest into git smart-HTTP entry paths

## Brief

In clustered mode (`JAMSESH_DEPLOY_MODE=clustered`), a freshly-created session
returns HTTP 500 on the first git operation (clone or push) routed to any pod
that didn't handle the session-create API call. Twelve e2e tests fail with
this shape (chaos, failure-mode, fuzz, and scaffolding's `TestClusteredSmoke`).
Single-instance mode is unaffected.

Root cause: the hydration infrastructure is correct and complete —
`objectstore.LifecycleManager.AcquireForRequest(ctx, sessionID)` acquires the
distributed Postgres advisory lease AND hydrates the bare repo from object
storage (or initialises an empty one for a fresh session) — but its only call
site is `internal/portal/postreceive/emitter.go:102`, which fires AFTER
`git-receive-pack` has already written to a (non-existent on this pod) bare
repo. The three git smart-HTTP entry points
(`internal/portal/githttp/{info_refs.go:35, upload_pack.go:22,
receive_pack.go:119}`) call `Storage.RepoPath` directly with no hydration
trigger. So peer pods serve git requests against a path that doesn't exist
locally → `git-upload-pack` / `git-receive-pack` subprocess fails → 500.

The fix is one piece of plumbing: thread `LifecycleManager` into the git
handler and call `AcquireForRequest` at the start of each smart-HTTP entry
point. Single-instance mode (Lifecycle == nil) falls through to existing
behaviour — no regression.

## Strategic decisions

Locked at scope/design time.

- **Add `Lifecycle` to `githttp.Handler`, not a global.** The Emitter already
  carries a `Lifecycle` field; mirror the pattern. Keeps wiring local.
- **Acquire at entry, release on lifecycle terms.** Per the existing Emitter
  pattern: do NOT release the handle at handler return. `LifecycleManager`
  owns handle lifetime (idle eviction / LRU / lease loss / SIGTERM).
- **Hydration failure → 503 with `Retry-After: 1`.** Routes router redispatch.
- **No-op in single mode.** `if h.Lifecycle == nil { skip }` — same shape as
  the Emitter's nil-check.
- **Acquire is idempotent on this pod.** `LifecycleManager` caches active
  sessions and returns the existing handle on second AcquireForRequest.

## Architectural choice

Choosing **hydrate-on-miss in the git smart-HTTP handler** because:
- It leverages existing `LifecycleManager.AcquireForRequest`, already wired
  with lease + hydration + fencing tokens.
- The change is contained: four files in `githttp/` plus the constructor in
  `cmd/portal/main.go`.
- Matches how the Emitter does it — one consistent pattern.

Rejected alternatives:
- **Mirror empty bare-repo at session create** — doesn't fully solve it
  (peer pods still need a hydration trigger). The chosen approach handles
  the fresh-session case via `Hydrator`'s `Storage.CreateRepo` fallback.
- **Lease-aware routing** — would require router to query Postgres per
  request for the current lease holder. Significantly more architecture;
  defers a working fix indefinitely.

## Implementation Units

### Unit 1: Wire `Lifecycle` into `githttp.Handler` and acquire at entry

**Files**:
- `internal/portal/githttp/handler.go` (add field + helper)
- `internal/portal/githttp/info_refs.go` (acquire before `RepoPath` at ~line 35)
- `internal/portal/githttp/upload_pack.go` (acquire before `RepoPath` at ~line 22)
- `internal/portal/githttp/receive_pack.go` (acquire before `RepoPath` at ~line 119)
- `cmd/portal/main.go` (wire `Lifecycle: objLifecycle` into the `githttp.Handler` literal at ~line 671)

**Story**: `feature-portal-githttp-hydrate-on-request-wiring`

Add to `Handler` struct:

```go
// Lifecycle, when non-nil, hydrates the bare repo on first request for a
// session on this pod and holds the distributed lease. Nil in single-instance
// mode — the bare repo lives on the only pod's local disk from session-create
// onward. Mirrors postreceive.Emitter's Lifecycle field shape and semantics.
Lifecycle *objectstore.LifecycleManager
```

Add helper method:

```go
// acquireForGitRequest invokes LifecycleManager.AcquireForRequest in
// clustered mode to ensure the session's bare repo is hydrated to this pod's
// local cache before serving the smart-HTTP operation. The returned handle is
// owned by LifecycleManager (idle eviction / LRU / lease loss / SIGTERM); the
// caller MUST NOT release it. Returns nil immediately when Lifecycle is nil
// (single-instance mode).
func (h *Handler) acquireForGitRequest(ctx context.Context, sessionID string) error {
    if h.Lifecycle == nil {
        return nil
    }
    _, err := h.Lifecycle.AcquireForRequest(ctx, sessionID)
    return err
}
```

Call site shape (in each of info_refs.go, upload_pack.go, receive_pack.go),
inserted between auth/session-lookup and `Storage.RepoPath`:

```go
if err := h.acquireForGitRequest(r.Context(), sessionID); err != nil {
    // Hydration failure: clustered backend can't get the bare repo. 503
    // signals the router to redispatch; Retry-After hint sets cadence.
    w.Header().Set("Retry-After", "1")
    httperr.WriteFromError(w, deperr.WrapObjectStorage(
        fmt.Errorf("githttp: hydrate session %s: %w", sessionID, err)))
    return
}
```

Constructor change in `cmd/portal/main.go:671`:

```go
gitHandler := &githttp.Handler{
    Store:   dbStore,
    Tokens:  tokenSvc,
    Storage: storageSvc,
    Validator: ...,
    Emitter: ...,
    Lifecycle: objLifecycle, // NEW — nil in single-instance, hydrates+leases in clustered
    Metrics:        metricsReg,
    ReceivePackSem: receivePackSem,
}
```

**Implementation Notes**:
- Auth MUST run before acquire. Don't hydrate for an unauth probe — attack surface.
- Session-existence check (DB lookup) likewise before acquire.
- Use `deperr.Wrap*` per the pattern in `auth/magic_link.go`. If
  `deperr.WrapObjectStorage` doesn't exist today, either use
  `deperr.WrapDBIfTransient` shape OR add the new wrapper aligned with
  `deperr-translate-pipeline` pattern. Verify the exact wrapper name at
  implement time.
- `info_refs` is the FIRST request in a clone (git fetches `info/refs` to
  discover refs). Wiring acquire there is mandatory. `upload_pack` direct
  hits without `info_refs` are rare but possible — wire it too (defence
  in depth, idempotent).
- For `receive_pack` (push), the existing post-receive Emitter at request
  END also calls `AcquireForRequest`. After this change, that path is
  idempotent — entry acquire grabs the handle, Emitter's acquire returns
  the same cached handle. Verified by `lifecycle.go`'s `AcquireForRequest`
  semantics.
- The fencing token threaded from the lease handle into pre-receive's
  subprocess env (today populated by the Emitter at post-receive time): with
  this fix, the handle is now also available at entry. The plumbing may need
  to be updated so pre-receive sees the token. Check `receive_pack.go`'s
  subprocess env construction; if the token flows from `Emitter` after-the-fact,
  may need to capture from the entry acquire. Verify at implement time —
  acceptable if existing flow is preserved.

**Acceptance Criteria**:
- [ ] `githttp.Handler` struct has `Lifecycle *objectstore.LifecycleManager`
      field with a doc comment matching the Emitter's wording.
- [ ] `acquireForGitRequest` helper added: nil-tolerant, single-pass.
- [ ] All three entry handlers call `acquireForGitRequest` between
      session-lookup and `Storage.RepoPath`.
- [ ] Hydration failure → HTTP 503 + `Retry-After: 1` + typed `httperr` body.
- [ ] `cmd/portal/main.go` constructs `gitHandler` with `Lifecycle:
      objLifecycle`. Single-mode (where `objLifecycle == nil`) is a graceful
      no-op.
- [ ] Existing unit test sweep passes: `go test ./internal/portal/...`.
- [ ] After push, the e2e suite's clustered tests pass (verify in CI):
      `TestClusteredSmoke`, `TestCrossPodClockSkew`,
      `TestHandoffUnderObjectStorageChaos`, `TestHandoffUnderPodKill`,
      `TestLeaseHolderKilled`,
      `TestRouterBackendDead/dead_pod_removed_from_routing_pool`,
      `TestRouterLeaseUnavailable/*`, `TestStaleFencingTokenRejected`,
      `TestFencingTokenFuzz/*`, `TestPackManifestFuzz/*`.
- [ ] `TestLeaseAlreadyHeld` returns 503 (lease held by test process) rather
      than 500 (missing repo) — proves the fix routed the request through
      the lease check.

---

## Implementation Order

Single story. The handler wiring + the constructor change ship as one commit.
The e2e suite is the regression guard; no new test code needed (existing
`TestClusteredSmoke` covers the canonical path).

## Testing

### Unit
Existing `internal/portal/githttp/*_test.go` use a nil `Lifecycle` (single-mode).
Add a small unit test that verifies behaviour with a fake `Lifecycle`:
- Returns nil: handler proceeds to `Storage.RepoPath`.
- Returns error: handler emits 503 + Retry-After.

A minimal fake `Lifecycle` interface (or a concrete `*LifecycleManager` with
test wiring) is acceptable — match what the existing Emitter tests do.

### E2E
`TestClusteredSmoke` is the canonical regression. After Unit 1 lands, the
e2e suite should clear ~12 tests in one shot.

## Risks

- **Acquire blocks while holding HTTP request goroutine.** Hydration can be
  multi-second for large sessions. Router has its own request timeout
  (chi `middleware.Timeout`). If hydration exceeds it, router 503s anyway;
  next request finds hydration already done. Lifecycle's idempotent caching
  makes the retry safe.
- **Hydration storms on cold cluster start.** Two pods both try to acquire
  + hydrate the same session simultaneously. PostgreSQL advisory lock
  serialises — the second blocks (or gets ErrAlreadyHeld) until the first
  releases. Cost paid once. Acceptable.
- **Lease loss mid-request.** If a pod loses lease (Postgres blip) while
  serving a clone, the in-flight clone continues against the now-cached
  repo. Reads return what was on disk at clone start; push would fence-fail
  at pre-receive. Existing risk; this fix doesn't change it.
- **Pre-receive fencing-token plumbing.** Pre-receive expects a fencing
  token in env from the lease handle. The Emitter currently feeds this at
  post-receive time. With acquire-at-entry, the handle is available
  throughout. Verify at implement that the existing token flow remains
  intact; if not, plumb token from entry-acquire through to subprocess env.
  Should be a small addition to `receive_pack.go`.

<!-- Implementation Notes accumulate as work lands. -->
