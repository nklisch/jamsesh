---
id: bug-playground-git-receive-pack-fails-with-200-hangup
created: 2026-05-24
tags: [bug, playground, githttp, portal]
---

When the CLI binary `jamsesh new --playground` pushes its base ref to
the just-created playground session, the `POST /git/org_playground/<sessionID>.git/git-receive-pack`
request returns HTTP 200 with ~166 bytes of body in ~1ms, but git
client-side reports "fatal: the remote end hung up unexpectedly" and
exits non-zero. Surfaced by e2e test `TestCLI_JamPlayground` at commit
TBD (after the URL-missing-org_id fix in `newPlaygroundAction`).

Auth passes (the route's `basicAuth` middleware accepts the
`jamsesh_anon_*` bearer via `tokens.BasicAuthValidator`), and the URL
shape is correct (`/git/{orgID}/{sessionID}.git/git-receive-pack`
matches the route registered at `internal/portal/githttp/handler.go:90`).
The failure is inside the receive-pack subprocess pipeline. Likely
causes to investigate:

1. The anonymous-bearer Account returned by `tokens.BasicAuthValidator`
   may not satisfy the assumptions `receivePack` makes about the
   account shape (e.g. expects an OAuth-issued account with an
   `external_id`, but the anon account has none).
2. The receive-pack subprocess wiring may pass arguments that work for
   durable sessions but not for the reserved `org_playground` org
   (e.g. the on-disk repo path differs for the reserved org).
3. The pre-receive hook installed by the portal's storage service may
   not work against the playground org's storage layout.

Reproduction:
1. `cd tests/e2e && go test ./golden/ -run TestCLI_JamPlayground -count=1 -v`
2. Observe portal log: `POST /git/org_playground/<id>.git/git-receive-pack ... status:200, duration_ms:1, bytes:166`
3. Observe client log: `fatal: the remote end hung up unexpectedly` then `error: failed to push some refs`

This bug is the root cause of one of the four failure-mode observations
the e2e suite is catching that unit tests miss (per the playground
audit). The receive-pack subprocess pipeline cannot be unit-tested
faithfully — it requires a real `git-receive-pack` subprocess + a real
on-disk bare repo + a real auth-resolved account.

Suggested investigation start: add a `slog.Debug` at the start and end
of `receivePack` (and at each pkt-line write) to confirm the subprocess
starts, runs, and returns expected output sizes. Then trace the 166-byte
response back to its pkt-line origin — it's probably an early ng-status
or sideband-error before the actual packfile-processing happens.

## Severity

High. The CLI's primary v0.4.0 capability (`jamsesh new --playground`)
cannot complete its first push end-to-end against a real portal. Unit
tests pass because they use `httptest.NewServer` + `stubStorage` — they
never invoke the real `git-receive-pack` subprocess.
