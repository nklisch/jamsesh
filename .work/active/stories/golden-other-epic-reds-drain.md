---
id: golden-other-epic-reds-drain
kind: story
stage: review
tags: [testing, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-31
updated: 2026-05-31
---

# Drain the three remaining golden-suite reds belonging to other epics

After the cloud-native epic `e2e-cloud-native-multipod-suite-red` went green,
three golden-package e2e tests remained red. They belong to *other* epics
(finalize-flow, comments/MCP, CLI playground). This story reproduces, root-
causes, classifies, and drains all three so the `golden` package is exit-0.

## Verification setup

Docker is exclusively owned by this agent on the MAIN tree. `/tmp` is a tmpfs;
to keep Go's temp output on disk and reproduce the real environment the
autopilot runs under:

```
export GOTMPDIR=$HOME/.cache/gotmp TMPDIR=$HOME/.cache/gotmp
mkdir -p $GOTMPDIR
```

Per-test: `cd tests/e2e && go test -p 1 -count=1 -timeout 1200s ./golden/ -run '<Name>' -v`

Portal image reused: `jamsesh/portal:e2e` (no product code changed → no rebuild).

---

## RED 1 — golden/TestFinalizeLockStateMachine

**Symptom:** `finalize_plan_test.go:350: patchFinalizeLock: status 400 (want 200):
{"error":"session.invalid_base_sha","message":"base_sha must be a 40-character
lowercase hex SHA-1"}`

**Root cause:** At step 4 of the lock state-machine exercise the test PATCHes the
finalize lock with `BaseSha: ""` (empty string). The portal's
`PatchFinalizeLock` handler validates `base_sha` via `ValidateBaseSHA`
(`internal/portal/finalize/escape.go:60`, regex `^[a-f0-9]{40}$`), which rejects
the empty string and returns `400 session.invalid_base_sha`. This validation was
added intentionally by the security-hardening commit `cc8c2854 implement:
gate-security-finalize-script-shell-escape` and is locked in by the dedicated
unit test `TestPatchFinalizeLock_RejectsMalformedBaseSHA`
(`internal/portal/finalize/lock_patch_test.go:325`), whose cases explicitly
include `{"empty", ""}` → must return `session.invalid_base_sha`.

The e2e test predates that hardening; it assumed an empty base_sha was a valid
"minimal curation" value at PATCH time. It is not.

**Regression check (per task brief):** The shared gitclient fixture change
(commit `44f949b2`, `Clone` now checks out an existing `jam/<sid>/<uid>/main`
ref) is NOT involved. `TestFinalizeLockStateMachine` clones no repo and computes
no base_sha from git — it hardcodes `BaseSha: ""`. The gitclient change is a red
herring for this red.

**Classification:** TEST bug (drifted assertion). Product `ValidateBaseSHA` is
correct and contract-locked by a unit test.

**Fix:** Send a syntactically-valid 40-hex base_sha in the state-machine PATCH.
The state-machine test does not execute the plan, so any well-formed SHA-1
satisfies the handler; use a deterministic 40-hex constant.

**Disposition:** Fixed in-session (test debt). PASS.

---

## RED 2 — golden/TestForkAndComment

**Symptom:** `fork_and_comment_test.go: user-prompt-submit additionalContext
does not contain the comment text "Agent B, please review this initial commit."`
Captured `additionalContext` was initially **completely empty**.

**Two layers — a TEST issue masking a real PRODUCT bug:**

*Layer 1 (TEST, masking):* The ccdriver fixture set
`JAMSESH_DATA_DIR = t.TempDir()` (`tests/e2e/fixtures/ccdriver/driver.go:73`,
fed from the test at line 140). Under this environment's umask 0022, Go's
`t.TempDir()` numbered leaf (`.../001`) is created `os.Mkdir(dir, 0o777)` →
**0755** (`$GOROOT/src/testing/testing.go:1481`). The CLI's `state.DataDir()`
enforces `mode & 0o077 == 0` via `checkDirPerms`
(`cmd/jamsesh/state/state.go:78`, the
`gate-security-datadir-permissions-not-validated` gate). So `resolveSessionID`
errored, `resolveHookSession` swallowed it to `(nil, nil)`
(`cmd/jamsesh/hooks/sessionstart.go:47`), and the hook silently no-op'd with an
empty `additionalContext`. Fix: `os.Chmod(dataDir, 0o700)` in the test (what a
real operator does). This unblocked the hook so it could actually run.

*Layer 2 (PRODUCT, the real bug):* Once the hook ran, the digest GET returned
**401**, then refresh failed (`reading refresh token: ... not found`), so
`additionalContext` carried only `## Warning: could not fetch digest`.
Root cause: `main()` runs `state.MigrateToPerSessionTokens` on EVERY binary
invocation (`cmd/jamsesh/main.go:38`). Once a session dir exists, migration fans
the legacy account-wide `${data-dir}/token` out into `sessions/<id>/token` and
then **overwrites the legacy `token` file with the `MIGRATED_TO_PER_SESSION`
stub** (`cmd/jamsesh/state/migrate.go:89`). But the hooks' `buildPortalClient()`
constructed `portalclient.Client{BaseURL: ...}` with **no SessionID**
(`cmd/jamsesh/hooks/sessionstart.go`), so `attachBearer` →
`ReadCurrentBearer("")` → `ReadToken()` read the **stub** and sent
`Authorization: Bearer MIGRATED_TO_PER_SESSION` → 401 on every authenticated
hook call. This breaks the user-prompt-submit and session-start hooks in
**production** for any user who has ever joined a session (migration stubs the
legacy token on the next invocation). The session-scoped commands
(`sessioncmd` join/status/resume) already set `SessionID:` on their client —
the hooks were the outlier.

**Classification:** PRODUCT bug (small, clearly-correct fix; not deep — a
one-parameter threading change matching an established pattern).

**Fix (product):** Thread the resolved `ss.SessionID` into
`buildPortalClient(sessionID)` and set it on the client so `attachBearer` reads
`sessions/<id>/token` first. Updated `buildPortalClient` signature plus the two
call sites (`sessionstart.go`, `userpromptsubmit.go`). Hooks + state unit tests
stay green. The binary is rebuilt by the e2e `binary.Build` fixture, so no image
rebuild was needed.

**Disposition:** PRODUCT fix applied in-session (small + clearly correct per
test-integrity rules) + TEST chmod fix. PASS.

---

## RED 3 — golden/TestCLI_JamPlayground

**Symptom:** `cli_jam_playground_flag_test.go:82: jamsesh new --playground: exit
error: exit status 1`. Captured stderr:
`writing playground session token: data dir ".../001" has unsafe permissions
0755 (must be 0700 or tighter); run: chmod 700 ".../001"`.

**Root cause:** Identical mechanism to RED 2. The test sets
`JAMSESH_DATA_DIR = t.TempDir()` (`cli_jam_playground_flag_test.go:51` →
env at line 78). Under umask 0022 the `t.TempDir()` leaf is 0755; the CLI's
`state.DataDir()` permission gate refuses it and `jamsesh new --playground`
exits 1 while writing the session token. The state unit tests already encode
this contract — they `os.Chmod(dir, 0o700)` before exercising `DataDir()`, and
`state_test.go` explicitly asserts a 0755 dir is rejected.

**Classification:** TEST bug (fixture/environment interaction). Product
`DataDir()` is correct.

**Fix:** `os.Chmod(pluginDataDir, 0o700)` after `t.TempDir()` so the CLI's data
dir satisfies the security contract.

**Disposition:** Fixed in-session (test debt). PASS.

---

## Summary

- **RED 1** — TEST bug (drifted assertion: empty base_sha is contract-rejected).
  Fixed in test.
- **RED 2** — TEST chmod issue masking a real **PRODUCT bug**: the hooks'
  portal client wasn't session-scoped, so after per-session token migration it
  read the `MIGRATED_TO_PER_SESSION` stub and 401'd. Both fixed (test chmod +
  product `buildPortalClient(sessionID)`).
- **RED 3** — TEST bug (same 0755-`t.TempDir()`-vs-0700-gate interaction as
  RED 2's masking layer). Fixed in test.

**Product code changed:** `cmd/jamsesh/hooks/sessionstart.go` and
`cmd/jamsesh/hooks/userpromptsubmit.go` — `buildPortalClient` now takes and
applies a `sessionID` so the user-prompt-submit and session-start hooks read the
per-session bearer token instead of the migrated-away legacy stub. No portal
image rebuild needed (the CLI binary is rebuilt by the e2e `binary.Build`
fixture). No backlog items filed — the RED 2 product bug was small and
clearly-correct to fix in-session.

The data-dir permission gate and the base_sha validator are both working as
designed and unit-test-locked; the reds were a drifted assertion (RED 1), a
fixture/umask interaction (RED 3 + RED 2 layer 1), and a hook token-scoping bug
(RED 2 layer 2).
