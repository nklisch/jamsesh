---
id: epic-finalize-flow-plugin-finalize-command-fetch-source-selection-and-cleanup
kind: story
stage: implementing
tags: [plugin]
parent: epic-finalize-flow-plugin-finalize-command
depends_on: [epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow]
release_binding: null
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

- [ ] Local-first: when
      `${CLAUDE_PLUGIN_DATA}/sessions/<sid>/local_path` exists AND
      points to a real git repo on disk, `chooseFetchSource` returns
      `kind: "local"` and `performFetch` runs `git fetch <path>`
- [ ] Local-first absent: when the state file is missing OR its
      path doesn't exist, fall back to HTTPS
- [ ] HTTPS fallback:
      `POST /api/sessions/<sid>/finalize/fetch-token` is called with
      the existing portalclient (Bearer attached, refresh-on-401)
- [ ] HTTPS URL form: `https://x-access-token:<token>@<portal>/git/<org>/<sid>.git`
      (mirror `sessioncmd.buildCloneURL` shape)
- [ ] Temp remote lifecycle: `git remote add jamsesh <url>` →
      `git fetch jamsesh` → `git remote remove jamsesh` on EVERY exit
      path (clean success, conflict halt, pre-flight failure after
      remote-add, SIGINT)
- [ ] After a clean finalize-run, `git remote -v` shows no `jamsesh`
      entry
- [ ] After a SIGINT mid-run (simulated by canceling the root
      context), `git remote -v` shows no `jamsesh` entry
- [ ] Stash created in pre-flight is popped on clean exit; LEFT IN
      PLACE on conflict exit (so the user can resume cleanly)
- [ ] Original branch is restored via `git checkout -` on clean exit;
      NOT restored on conflict exit (user stays on the partial target
      branch)
- [ ] Remote-removal failure is logged but does NOT mask the primary
      error (best-effort cleanup)
- [ ] `go build ./...` clean
- [ ] `go test ./cmd/jamsesh/finalizecmd/...` passes (all of story
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

<!-- Filled in by /agile-workflow:implement after work completes. -->

## Review

<!-- Filled in by /agile-workflow:review when this story reaches stage:review. -->
