---
id: e2e-audit-playground-content-cap-pre-receive-enforcement
kind: story
stage: drafting
tags: [testing, e2e-test, audit, playground]
parent: feature-e2e-playground-coverage-failure
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# 50 MiB per-session content cap has no e2e test — enforced only at pre-receive but never verified end-to-end

## Severity
High

## Finding type
missing-taxonomy-layer

## Evidence

The content cap is documented as "enforced at pre-receive". Grep for any
test that exercises a real git push against a playground session at the
cap boundary:

```
$ grep -rIn -E "50 MiB|50MiB|content.cap|MaxRepoSize|playground.*cap" tests/e2e/
(no output)
$ grep -rIn -E "50 MiB|content.cap" internal/portal/playground/
(unknown — not read per audit rules, but no test name in handler_test.go
 list mentions a cap test)
```

The handler unit tests inventoried in `internal/portal/playground/handler_test.go`
(via `grep "^func Test"`) include no `TestContentCap*` or similar. Pre-receive
hook enforcement is a real `git-receive-pack` subprocess concern (see the
`git-smart-http` skill) — by design it cannot be unit-tested without a real
git subprocess + a real packfile + a real `internal/portal/git` pre-receive
hook installed in the bare repo.

## Why this matters

The content cap is the **second** abuse defense after the create rate
limit. A bug here means a single anonymous user can fill the portal disk.
Production failure modes hidden from unit tests:
- Cap counted in compressed bytes vs uncompressed bytes mismatch.
- Cap enforced per-push vs cumulative across the session.
- Cap enforcement happening after the packfile is already written
  (defeats the purpose).
- Cap not applied when the session is over the limit AFTER a push (only
  on the next push).
- Cap silently disabled when env var is unset (production footgun).

The pre-receive code path lives in
`internal/portal/git/prereceive.go`-ish territory (per the `git-smart-http`
skill) and is fundamentally a real-subprocess concern. Unit tests cannot
faithfully simulate it.

## Suggested remedy

Add `tests/e2e/failure/playground_content_cap_test.go` using the existing
`gitclient` fixture to build a packfile that exceeds the cap. Configure
the portal with a low test cap (e.g. 1 MiB) so the test is fast. Assert:
1. First push of e.g. 500 KiB succeeds.
2. Second push that would bring total to 1.5 MiB returns a non-zero git
   exit code, AND the bucket size on disk is still ≤ 1 MiB (no partial
   write committed — same RPO invariant as
   `object_storage_partition_test.go`).
3. Error envelope (if surfaced over the smart-HTTP error channel)
   identifies the cap as the cause.

A companion subtest can verify the cap is per-session (one session's
fill does not consume another's quota).

## Test sketch

```go
// tests/e2e/failure/playground_content_cap_test.go
func TestPlayground_ContentCap_PreReceiveRejectsOversize(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    p := portal.Start(ctx, t, portal.Options{
        DBDriver:              "postgres",
        DBDSN:                 pg.ContainerDSN,
        PlaygroundEnabled:     true,
        PlaygroundMaxBytes:    1 << 20, // 1 MiB for test speed
    })

    sess := createPlayground(t, p.URL)
    repoURL := p.URL + "/git/playground/" + sess.ID + ".git"

    // First push: 500 KiB blob, succeeds.
    require.NoError(t, gitclient.PushBlob(t, repoURL, sess.Bearer, 500<<10))

    // Second push: 800 KiB blob, would bring total over 1 MiB → reject.
    err := gitclient.PushBlob(t, repoURL, sess.Bearer, 800<<10)
    require.Error(t, err, "oversize push must be rejected at pre-receive")

    // On-disk repo size must be at or below the cap.
    size := dockerExecDuBytes(t, p, portalRepoPath("playground", sess.ID))
    require.LessOrEqualf(t, size, int64(1<<20), "repo size %d exceeds cap", size)
}
```
