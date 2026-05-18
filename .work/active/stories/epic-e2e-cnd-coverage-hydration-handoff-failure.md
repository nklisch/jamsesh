---
id: epic-e2e-cnd-coverage-hydration-handoff-failure
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-hydration-handoff
depends_on: [epic-e2e-cnd-coverage-hydration-handoff-infra]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Hydration-Handoff Failure — Missing Pack Refuses Cleanly

## Scope

One failure-mode test: corrupt-bucket scenario where a pack object is deleted
out-of-band from MinIO before pod B attempts to hydrate. The safety invariant
is that hydration **fails loudly** — no silent truncation, no partial state
served.

This is the heaviest test-integrity story in the hydration-handoff feature.
If the implementation silently truncates a session on hydration failure, that
is a Critical production bug — park it, don't suppress the test.

## Unit 1: `tests/e2e/failure/hydration_with_missing_pack_test.go`

```
Package: failure_test
Test: TestHydrationWithMissingPack
```

**Invariant:** "If a pack object is missing from the MinIO bucket when pod B
attempts to hydrate session S, pod B refuses to serve S and returns a
documented error (not a 200 with truncated history). No partial state is
silently returned."

**Stack:** `postgres.Start` + `minio.Start` + `mailhog.Start` +
`portalcluster.Start(Pods: 2, Router: false)` — Router: false so we can
address each pod directly without the static-discoverer bug interfering.

**Setup:**
1. Alice signs in via pod 0, creates org + session.
2. Push enough commits to guarantee at least one packfile is created
   server-side (push 15 commits with moderately-sized content, mirroring the
   `multi_pack_push` pattern in `object_storage_rpo0_test.go`).
3. Record `draftTipBefore` from pod 0.
4. Verify bucket has objects: `mn.ListObjects("sessions/"+sessionID+"/")`.
   Extract the pack key(s): keys matching `sessions/<id>/objects/pack/*.pack`.

**Corruption action:**
5. Pick the first pack key. Call `mn.DeleteObject(ctx, packKey)` to remove it
   from the bucket out-of-band.
6. Verify deletion: `mn.ListObjects` no longer returns `packKey`.

**Trigger hydration on pod 1:**
7. Pod 1 has never seen the session (it has been idle, Router: false, no
   requests to pod 1 yet). Attempt a git push via pod 1:
   `gitclient.Clone(ctx, t, c.Pods[1].URL, ...)` + `repo.Commit` + `repo.Push`.
8. The push requires pod 1 to acquire the lease and hydrate. But the pack is
   missing — hydration must fail.

**Assertions:**
9. `repo.Push` must fail with a non-zero git exit (gitclient.Push returns an
   error or fatals — update gitclient to return error variant if needed, or
   use `gitclient.TryPush` if available). The push failure surfaces as a
   non-200 HTTP from git smart-HTTP.
10. Confirm pod 1 does NOT serve the session in a partial state: attempt
    `git ls-remote` against pod 1 for the session. Either:
    - Returns an error (session unavailable — preferred; loudest failure), OR
    - Returns the empty ref set (session unknown — also acceptable)
    It must NOT return refs pointing at commits from before the pack was deleted
    (which would indicate partial hydration silently succeeded).
11. `mn.ListObjects("sessions/"+sessionID+"/")` from the test process: the
    bucket should be in the same state it was after corruption (pod 1's failed
    hydration must not write any new manifest claiming success).

**Error code assertion:** If the portal exposes a machine-readable error code
on the git smart-HTTP error response (e.g. in the error body or a custom
header), assert it contains a hydration-corruption indicator
(e.g. `hydration.corrupt_bucket`, `ErrMissingPack`, or similar). If no such
code exists, assert only on the HTTP status (non-200) and document the absence
of a machine-readable code as a `Medium` finding in the test body with a
`t.Logf` noting the gap — this is not a test bug, it is a missing feature.
Do not park it unless the HTTP status itself is 200 (that would be Critical).

**Recovery assertion (optional, non-blocking):**
After the deletion, restore the object: `mn.PutObject(ctx, packKey, originalData)`.
Re-attempt the push. Confirm it now succeeds and `draftTipAfter` matches
`draftTipBefore`. This is a "recovery after repair" scenario and demonstrates
the failure is transient (not data loss). Mark this subtest as `t.Run("recovery_after_repair", ...)`.

## Acceptance criteria

- [ ] `TestHydrationWithMissingPack` green; push to pod 1 fails with non-zero
      exit when pack is deleted
- [ ] Pod 1 does NOT serve partial state after failed hydration (ls-remote
      check passes)
- [ ] Bucket is not mutated by the failed hydration attempt (manifest not
      updated to claim success)
- [ ] `recovery_after_repair` subtest green: restoring the pack allows
      hydration to succeed
- [ ] No in-process mocks

## Test integrity (from parent feature)

**This story carries the heaviest test-integrity weight in the feature.**

- If `repo.Push` to pod 1 returns **success** (zero exit) AND the session
  is served in a truncated state — that is a **Critical production bug**.
  Park it immediately via `/agile-workflow:park` with severity Critical.
  Land the test with `t.Skip("bug-<id>: hydration silently serves partial
  state after missing pack — safety invariant violated")`.
  A failing test that documents this bug is more honest than a green suite
  that hides it.

- If `repo.Push` fails (correct) but the error is not a clean HTTP 4xx/5xx
  (e.g. git hangs, or returns a 200 with garbled output) — that is a
  Medium production bug. Park and land with a skip + backlog id.

- If the test is flaky because the gitclient does not distinguish push
  failure from test infrastructure failure, fix the gitclient to return
  an error variant (`TryPush`) rather than always fataling — that is a
  test-infrastructure bug to fix in-session.

- Never game: do not assert `true == true` or remove the assertion because
  "the system didn't return what we expected." The invariant is the point.
