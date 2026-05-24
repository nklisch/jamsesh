---
id: e2e-audit-playground-content-cap-pre-receive-enforcement
kind: story
stage: review
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

## Implementation notes

**File**: `tests/e2e/failure/playground_content_cap_test.go`

**Test function**: `TestPlayground_ContentCap_PreReceiveRejectsOversize`

**What was implemented**:
Two subtests:
1. `oversize_push_rejected` — pushes a small seed on the base ref (succeeds), then pushes
   a ~1.5 MiB random blob (gzip-incompressible) on a user ref. Push exits non-zero.
   On-disk repo size after rejection is 23,286 bytes (cap=1 MiB, well within limit).
   The pre-receive check fires BEFORE spawning the git-receive-pack subprocess, so
   the oversize pack is never written to disk. No partial-write bug found.

2. `per_session_isolation` — S1 pushes 900 KiB (succeeds), then S2 (independent session)
   pushes 900 KiB (also succeeds). Quota is confirmed per-session, not global.

**`CommitBytes` added to gitclient**: New method on `*Repo` in
`tests/e2e/fixtures/gitclient/gitclient.go` that takes `[]byte` instead of `string`,
enabling binary/random-data blobs without string-encoding roundtrips.

**Rejection message format finding (parked)**:
The pre-receive rejection IS enforced (push exits non-zero), but the git client shows
`fatal: the remote end hung up unexpectedly` instead of the human-readable cap message
(`playground session content limit exceeded`). This is because git's stateless-RPC
two-POST protocol may cause the capabilities (including `side-band-64k`) to be absent
from the second POST's command list, making `writeReportStatusRejection` write
non-sideband pkt-lines while git expects sideband format.

Parked as:
`.work/backlog/bug-playground-content-cap-rejection-message-not-surfaced-to-git-client.md`

**All subtests pass**. Run with:
```
cd tests/e2e && go test ./failure/ -run TestPlayground_ContentCap -count=1 -v
```
