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

## Root cause (identified in follow-up autopilot pass)

The 200/166-byte response is `writeReportStatusRejection` —
`prereceive.Validator.Validate` rejects the base-ref push because
`WalkAndValidate` walks every commit in the pushed pack and
`validateCommit` runs `CheckRequiredTrailers` against
`requiredTrailers = []string{"Jam-Session", "Jam-Turn", "Jam-Author"}`
(`internal/portal/prereceive/commits.go:15`). A vanilla "initial commit"
made by `git commit -m '...'` (with no plugin hook in place) has none
of these trailers, so the seed commit fails validation and the base
ref push is rejected.

The 166 bytes is the rejection response (`unpack ok` + `ng refs/heads/jam/<id>/base <missing-trailers-message>` + flush pkt, sideband-wrapped). 1ms duration is consistent with in-process validation — the subprocess never runs.

## Real-world implication

`jamsesh new --playground` from a vanilla git repo will fail the same
way for real users. The CLI binary in `cmd/jamsesh/sessioncmd/new.go`
calls `pushBaseRefWithBearer` which does a raw `git push` of HEAD —
HEAD is whatever the user committed, without trailers, so it gets
rejected.

In production this currently "works" only because users with the
jamsesh CC plugin installed have a commit-msg hook that auto-appends
trailers to every commit before it lands in their working tree. A
user who runs `jamsesh new --playground` from a fresh repo BEFORE
ever attaching to a CC session — i.e. the exact "spin up a quick
playground" UX the v0.4.0 epic promised — will be locked out.

## Fix options

Three reasonable approaches; pick one or combine:

1. **Exempt base-ref pushes from trailer validation** in
   `prereceive.WalkAndValidate` when the update is the
   `refs/heads/jam/<session>/base` ref AND the OldSHA is empty (first
   push, repo was empty). The trailer requirement is about
   provenance for COLLABORATIVE work within a session; the seed
   commit predates the session so the requirement is incoherent.

2. **Auto-add trailers in `jamsesh new --playground`** by amending the
   HEAD commit (or creating a synthetic seed commit) with
   `Jam-Session: <new-session-id>`, `Jam-Turn: <some-sentinel>`,
   `Jam-Author: <playground-handle>` trailers before pushing. More
   invasive — modifies the user's local repo.

3. **Lift the trailer requirement entirely for playground sessions**
   in the validator, gated by `in.Session.OrgID == playground.ReservedOrgID`.
   Simpler than (1) but broader: also exempts later session pushes
   from trailer enforcement, which may not be desired.

Option (1) is recommended — surgical fix, preserves trailer enforcement
for actual session work, addresses the chicken-and-egg of bootstrapping.

## Severity

High. The CLI's primary v0.4.0 capability (`jamsesh new --playground`)
cannot complete its first push end-to-end against a real portal for
users without a pre-installed CC commit-msg hook. Unit tests pass
because they use `httptest.NewServer` + `stubStorage` — they never
invoke the real `git-receive-pack` subprocess and never run the real
prereceive validator against a real commit.

## Reproduction (verified)

```
$ cd tests/e2e && go test ./golden/ -run TestCLI_JamPlayground -count=1 -v
```

Portal log shows `POST /git/org_playground/<id>.git/git-receive-pack
... status:200, duration_ms:1, bytes:166`. Client shows `fatal: the
remote end hung up unexpectedly`. The 166-byte body is the
report-status rejection citing missing trailers on the seed commit.
