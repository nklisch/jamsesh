---
id: golden-cloud-native-suite-reds
kind: story
stage: done
tags: [portal, infra, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: []
release_binding: v0.5.0
gate_origin: null
created: 2026-05-31
updated: 2026-05-31
---

# Golden cloud-native suite reds — ref-name helpers + RPO0 test premises

Fix the four red golden cloud-native tests:

- `TestSessionHandoffCleanDrain`
- `TestSessionHandoffIdleEviction`
- `TestLifecycleEvictOnLeaseRelease`
- `TestObjectStorageRPO0` (subtests `refs_only_update`, `tag_creation`)

## Bug 1 — empty SHA / "could not resolve ref tip" (TEST DEBT)

### Root cause

The golden ref-tip-via-REST helpers compared the SHORT push ref
`jam/<sid>/<uid>/main` against the `/api/.../refs` endpoint's FULL ref name
`refs/heads/jam/...`. The REST API reports the canonical name
(`r.Name().String()` → `refs/heads/jam/...`); the test passed the short push
form, so `if r.Ref == ref` never matched → empty SHA → the three
`require.NotEmpty` assertions fired.

This is the IDENTICAL bug Agent A already fixed in the chaos suite
(`podKillRefTip` in `handoff_under_pod_kill_test.go`, `stosChaosRefTip` in
`handoff_under_object_storage_chaos_test.go`): canonicalize the expected ref to
`refs/heads/<ref>` before comparing.

### Fix

- `tests/e2e/golden/session_handoff_clean_drain_test.go` — `handoffRevParseViaPod`
  now canonicalizes `ref` to `refs/heads/<ref>` before the `r.Ref == wantRef`
  comparison. This single shared helper is also called by
  `session_handoff_idle_eviction_test.go` (line 112) and
  `lifecycle_evict_on_lease_release_test.go` (line 138), so fixing it once
  repairs all three Bug 1 reds.
- `tests/e2e/fixtures/portalcluster/state_compare.go` — `fetchSessionState`
  now normalizes both the API ref name (strip `refs/heads/`) and stores it in
  the short form so cross-pod `CompareSessionState` / `RequireSessionStateMatch`
  diff on a consistent key. (Belt-and-suspenders: the golden tests that fail
  use `handoffRevParseViaPod`, but the shared fixture had the same latent
  mismatch and is used by other handoff tests that key by short ref.)

`handoffGetRefTipFromClone` (clone + `git rev-parse origin/...`) is UNAFFECTED —
git resolves the short ref locally; not touched.

## Bug 2 — TestObjectStorageRPO0 (TEST PREMISE errors, NOT product bugs)

### refs_only_update — root cause

The subtest does `git reset --hard firstSHA` then `git push --force`, which is a
**non-fast-forward** update to a `jam/<sid>/<uid>/main` ref. The pre-receive
hook (`internal/portal/prereceive/refs.go` `checkForcePush`) **intentionally
rejects** non-fast-forward force-pushes on jam refs (rule 4 of `ValidateRef`)
with `push.force_push_rejected`. This is correct, intended product policy —
jam refs are append-only collaborative refs; rewriting history is disallowed.

The test premise ("force-push must still be durable / return 2xx") was wrong:
the server correctly returns a rejection, so `git push --force` exits non-zero
and `rpo0GitForcePush`'s `t.Fatalf` fired.

Verdict: **TEST fix** (per test-integrity: assert the real behavior). The
force-push is now expected to be REJECTED; the subtest asserts the rejection
(non-zero push exit + `force_push_rejected`/non-fast-forward in stderr) and then
re-confirms RPO=0 holds — the bucket still reflects the last *accepted* tip and
is non-empty. The accepted refs-only durability path (fast-forward) is already
covered by subtests `small_commit` / `multi_pack_push`.

### tag_creation — root cause

The subtest pushes an **annotated tag object** as a *branch* ref tip
(`git push origin v1.0:refs/heads/jam/<sid>/<uid>/v1.0`). `v1.0` resolves to the
annotated-tag object SHA, so the ref update's `NewSHA` is a TAG object, not a
commit. Pre-receive `WalkAndValidate` (`internal/portal/prereceive/commits.go`)
runs `repo.Log(&git.LogOptions{From: newHash})`, and go-git's
`Repository.log` calls `CommitObject(from)` — which fails on a tag object →
the rejection `push.scope_violation` "could not walk commits: object not found".

This is NOT the thin-pack disk-base class fixed in `receive_pack.go` (1cc1369e):
the tag object IS present in the pushed pack; the failure is that a branch ref
tip pointing at a non-commit object is not a supported product operation. Branch
refs (`refs/heads/...`) hold commits by git convention; the jam namespace
`jam/<sid>/<uid>/<branch>` is a user *branch* namespace, and the entire
pre-receive policy (trailers, scope) is commit-oriented. The test author's own
comment ("we push the tag into the jam/ namespace to stay within allowed ref
namespaces") shows this was a workaround to dodge the namespace policy, not a
real use case.

Verdict: **TEST fix**. Pushing a tag object as a branch tip is correctly
rejected; the subtest now asserts the rejection and re-confirms the bucket from
the preceding accepted commit push is still durable (RPO=0). (The portal does
not expose a `refs/tags/*` namespace to users today; if annotated-tag support
becomes a product requirement that is a separate feature, not an RPO=0 red.)

## Verification results

Test-only changes — reused existing `jamsesh/portal:e2e` + `jamsesh/router:e2e`
images (no rebuild needed). `go vet ./golden/ ./fixtures/portalcluster/` clean.

```
cd tests/e2e && GOTMPDIR/TMPDIR=$HOME/.cache/gotmp \
  go test -p 1 -count=1 -timeout 1800s ./golden/ \
  -run 'TestSessionHandoffCleanDrain|TestSessionHandoffIdleEviction|TestLifecycleEvictOnLeaseRelease|TestObjectStorageRPO0' -v
```

All GREEN (single run, no flake):

- `--- PASS: TestLifecycleEvictOnLeaseRelease (22.24s)`
- `--- PASS: TestObjectStorageRPO0 (4.20s)`
  - `--- PASS: .../small_commit`
  - `--- PASS: .../multi_pack_push`
  - `--- PASS: .../refs_only_update` — "force-push correctly rejected by
    pre-receive"; 7 object(s) still durable after rejected force-push
  - `--- PASS: .../tag_creation` — "tag-object-as-branch-tip push correctly
    rejected by pre-receive"; 4 object(s) still durable after rejected tag push
- `--- PASS: TestSessionHandoffCleanDrain (13.97s)`
- `--- PASS: TestSessionHandoffIdleEviction (14.97s)` — `draftTipBefore` now
  resolves to a real SHA (was empty pre-fix), confirming the Bug 1 helper fix.
- `ok  jamsesh/tests/e2e/golden  55.417s` (GO_TEST_EXIT=0)

### Product code

No product code changed. Both Bug 2 subtests were test-premise errors; the
pre-receive hook's force-push and commit-walk behavior is correct as-is.

### Out of scope (confirmed not touched)

`TestFinalizeLockStateMachine`, `TestForkAndComment`, `TestCLI_JamPlayground`,
`TestRouterMCPSessionHeader`, `TestFencingTokenFuzz` — other epics / infra
cold-start flakes. Not run, not modified.
