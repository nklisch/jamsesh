---
id: epic-finalize-flow-plugin-finalize-command-fetch-source-selection-and-cleanup
kind: story
stage: done
tags: [plugin]
parent: epic-finalize-flow-plugin-finalize-command
depends_on: [epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Finalize Command — Fetch Source Selection + Cleanup

## Scope

Replace the placeholder `chooseFetchSource` (added by story 1) with
the real local-first vs HTTPS-fallback logic and wire a
`cleanupStack` into the `finalize-run` action so the temporary
`jamsesh` remote, stash, and original-branch checkout are unwound on
every exit path (clean success, clean failure, SIGINT).

## Units delivered

- `cmd/jamsesh/finalizecmd/fetchsource.go` — real
  `chooseFetchSource` and `performFetch`
- `cmd/jamsesh/finalizecmd/fetchsource_stub.go` — DELETED (replaced)
- `cmd/jamsesh/finalizecmd/cleanup.go` — `cleanupStack` with
  Push/Run, SIGINT-aware goroutine listening on `ctx.Done()`
- `cmd/jamsesh/finalizecmd/finalizerun.go` (edit) — wire the
  cleanup stack into the action body; register stash-pop,
  branch-restore, and remote-removal cleanups in order; pass an
  `outcome` bit to `Run` so conditional cleanups run only on clean
  exit
- Test files: integration tests against `httptest.NewServer` for
  the HTTPS fallback path; real-git tests verifying `git remote -v`
  is clean after both happy-path and SIGINT-simulation runs

## Acceptance Criteria

- [x] Local-first: when
      `${CLAUDE_PLUGIN_DATA}/sessions/<sid>/local_path` exists AND
      points to a real git repo on disk, `chooseFetchSource` returns
      `kind: "local"` and `performFetch` runs `git fetch <path>`
- [x] Local-first absent: when the state file is missing OR its
      path doesn't exist, fall back to HTTPS
- [x] HTTPS fallback:
      `POST /api/sessions/<sid>/finalize/fetch-token` is called with
      the existing portalclient (Bearer attached, refresh-on-401)
- [x] HTTPS URL form: `https://x-access-token:<token>@<portal>/git/<org>/<sid>.git`
      (mirror `sessioncmd.buildCloneURL` shape)
- [x] Temp remote lifecycle: `git remote add jamsesh <url>` →
      `git fetch jamsesh` → `git remote remove jamsesh` on EVERY exit
      path (clean success, conflict halt, pre-flight failure after
      remote-add, SIGINT)
- [x] After a clean finalize-run, `git remote -v` shows no `jamsesh`
      entry
- [x] After a SIGINT mid-run (simulated by canceling the root
      context), `git remote -v` shows no `jamsesh` entry
- [x] Stash created in pre-flight is popped on clean exit; LEFT IN
      PLACE on conflict exit (so the user can resume cleanly)
- [x] Original branch is restored via `git checkout -` on clean exit;
      NOT restored on conflict exit (user stays on the partial target
      branch)
- [x] Remote-removal failure is logged but does NOT mask the primary
      error (best-effort cleanup)
- [x] `go build ./...` clean
- [x] `go test ./cmd/jamsesh/finalizecmd/...` passes (all of story
      1's tests still pass after the stub is replaced)

## Notes

- The placeholder from story 1 must be deleted, not edited — `git
  rm cmd/jamsesh/finalizecmd/fetchsource_stub.go` then add the real
  file. The function signature stays the same so the call site in
  `finalizerun.go` doesn't need changes.
- The SIGINT handler does NOT install its own `signal.Notify`; it
  watches `ctx.Done()` which is wired at the root in
  `cmd/jamsesh/main.go` via `signal.NotifyContext(ctx,
  os.Interrupt)`. The cleanup goroutine's `defer` of `os.Exit(130)`
  is reachable only on the cancel path.
- Cleanups are idempotent. Running `git remote remove jamsesh` when
  the remote no longer exists is treated as success (we eat the
  "No such remote" error). This makes double-invocation of the
  cleanup stack safe.
- The ephemeral fetch token has a ~5min TTL on the portal side
  (separate concern, owned by plan-generation). If our cleanup is
  killed with `kill -9`, the user's `git remote -v` may briefly
  show a `jamsesh` entry but the embedded credential is dead within
  5 min — a defense-in-depth backstop, not the primary cleanup
  guarantee.
- `fetchsource_test.go` uses `httptest.NewServer` patterned after
  `sessioncmd/fork_test.go` — same `setupSession(t, sid, portalURL)`
  helper shape. Token-fetch endpoint returns
  `{"token":"<jwt>","expires_at":"<rfc3339>"}` for the happy path,
  401 for the auth-fail path (asserts refresh-or-fail propagates).

## Implementation notes

### Files

- DELETED `cmd/jamsesh/finalizecmd/fetchsource_stub.go` (the
  placeholder from story 1)
- NEW `cmd/jamsesh/finalizecmd/fetchsource.go` — real
  `chooseFetchSource`, `performFetch`, idempotent `removeJamseshRemote`,
  and `localPathForSession` / `looksLikeGitRepo` helpers
- NEW `cmd/jamsesh/finalizecmd/cleanup.go` — `cleanupStack` with
  `Push`, `Run`, LIFO drain, `outcomeSuccess`/`outcomeAborted` bit,
  goroutine watcher on `ctx.Done()` (the root context already wires
  `signal.NotifyContext(ctx, os.Interrupt)` in `cmd/jamsesh/main.go`,
  so SIGINT propagates cleanly without any new signal handler here)
- EDITED `cmd/jamsesh/finalizecmd/finalizerun.go` — wired the cleanup
  stack, registered (in order) stash-pop (conditional), original-branch
  restore (conditional), and fetch-source cleanup (unconditional); the
  conditional cleanups are pushed BEFORE the unconditional one so the
  LIFO drain runs `remote remove jamsesh` first on a clean exit
- EDITED `cmd/jamsesh/finalizecmd/git.go` — added `runGitCombined`
  (captures stdout+stderr) so `removeJamseshRemote` can classify the
  "No such remote" stderr without spewing it to the user's terminal
- EDITED `cmd/jamsesh/finalizecmd/testhelpers_test.go` — added
  `runGitCombined` to the `pinGitToCwd` override set

### Tests added (18)

- `cleanup_test.go` (8): LIFO drain, conditional-skipped on abort,
  idempotent Run, Push-after-drain no-op, joined errors,
  ctx-cancel-drains-stack (the SIGINT goroutine path), main-flow-Run
  stops the watcher, idempotent-cleanup-fn safety
- `fetchsource_test.go` (10): local-first happy, local-path-missing
  → HTTPS, local-path-not-a-repo → HTTPS, HTTPS 401 propagation,
  empty `remote_url` rejection, `removeJamseshRemote` idempotent on
  missing, `removeJamseshRemote` happy path, `performFetch` local
  kind, `performFetch` HTTPS uses the named remote (does NOT leak
  the credential to the verbose log), `performFetch` unknown kind
- `finalizerun_cleanup_test.go` (2 integration): full finalize-run
  against an `httptest.NewServer` portal + a real source repo,
  asserting `git remote -v` is clean post-run; SIGINT simulation
  by stubbing `runGit("fetch","jamsesh")` to cancel the root ctx
  + return an error, then asserting the remote was still removed
  via the deferred `cleanup.Run`

### SIGINT-coverage approach

The story spec calls out testing the SIGINT path "by canceling the
root context." Two layers cover this:

1. **Unit-level** (`TestCleanupStack_CtxCancelDrainsStack`): proves
   the watcher goroutine fires `Run(outcomeAborted)` when ctx is
   cancelled, and that conditional cleanups are skipped.
2. **Integration-level**
   (`TestFinalizeRun_SIGINTSimulated_RemoteRemovedAfterCancel`): stubs
   the `git fetch jamsesh` invocation to cancel the root context AND
   return an error, simulating the OS delivering SIGINT mid-fetch. The
   test asserts the temporary jamsesh remote is gone after the action
   returns. The cancel-vs-cleanup race is real (which is why
   `cleanupStack.Run` is idempotent); we don't assert which goroutine
   wins, only that `git remote -v` is clean afterwards.

No subprocess-based SIGINT test was added — the in-process
ctx-cancel proxy exercises the same code paths (watcher goroutine,
LIFO drain, idempotency), and a real-SIGINT test would only verify
the OS plumbing which is already tested by signal.NotifyContext
upstream.

### Design notes / deviations

- The cleanup stack stores tasks in registration order and iterates
  in reverse on Run — simpler than a doubly-linked list and exactly
  matches the shell-defer mental model.
- `chooseFetchSource` now takes `orgID` as a parameter (in addition
  to `sessionID`), because the fetch-token endpoint is
  org-scoped: `POST /api/orgs/{orgID}/sessions/{sessionID}/finalize/fetch-token`.
  The story 1 call site in `finalizerun.go` already had `orgID`
  resolved via `readOrgIDForSession`, so threading it through was a
  one-line change.
- The HTTPS branch uses the portal-supplied `remote_url` field
  verbatim — the portal's `composeFetchRemoteURL` already produces
  the exact `https://x-access-token:<token>@<host>/git/<org>/<sid>.git`
  shape called for in the acceptance criteria. The plugin does NOT
  reconstruct the URL itself; this lets the portal's URL composition
  evolve (e.g. add path prefixes for proxies) without a plugin
  rebuild.
- `removeJamseshRemote` uses `runGitCombined` (combined output
  capture) rather than parsing exit codes — git prints
  "error: No such remote: 'jamsesh'" to stderr on the missing case;
  swallowing it by stderr-string match is the simplest reliable
  classification. A subtle benefit: this also keeps the alarming
  stderr line out of the user's terminal scrollback on the normal
  success-cleanup path.
- `chooseFetchSource` pre-cleans any pre-existing `jamsesh` remote
  (best-effort) BEFORE `remote add` so a previous run killed with
  `kill -9` (which would leave the entry dangling per the spec's
  defense-in-depth note) doesn't fail the subsequent `remote add`
  with "already exists."
- `performFetch` (HTTPS branch) uses the named `jamsesh` remote in
  the verbose log line, NOT the URL — this prevents the embedded
  fetch token from being printed to the user's terminal (which would
  end up in their scrollback and any shell logs). A test asserts the
  credential never appears in the verbose output.
- The "user declined" path in `finalizerun.go` sets
  `outcome = outcomeSuccess` before returning so the deferred Run
  pops any stash we created. The unconditional remote cleanup is
  never pushed on this path (we bail before chooseFetchSource), so
  the success bit only governs the stash + branch-restore tasks.

### Risks / follow-ups

- The `local_path` state file is not written by any current code
  path. Until a sibling story teaches `sessioncmd.JoinCommand` to
  record the local-checkout path, every finalize-run will take the
  HTTPS fallback. This is the parked follow-up the parent feature
  notes; it does not block this story.
- The SIGINT integration test sleeps 150ms to give the watcher a
  chance to drain mid-stub. If a future CI host is dramatically
  slower, the sleep may need to grow — but the assertion is on the
  end state (`git remote -v` clean), not on intermediate ordering,
  so a slow host would still produce a correct verdict.

## Review

<!-- Filled in by /agile-workflow:review when this story reaches stage:review. -->

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: LIFO cleanup stack with ctx-watcher goroutine + idempotent cleanups is the right shape. SIGINT integration test uses pre-cancelled context approach. Verbose log shows remote-name (jamsesh) not URL so the embedded token never appears in terminal output.
