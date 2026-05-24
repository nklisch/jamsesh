---
id: story-fix-playground-base-ref-trailer-exemption
kind: story
stage: review
tags: [bug, portal, playground, prereceive]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Exempt base-ref first pushes from per-commit trailer validation

## Symptom

`jamsesh new --playground` from a vanilla git repo (no plugin commit-msg
hook installed) fails its initial git push with:

```
fatal: the remote end hung up unexpectedly
fatal: the remote end hung up unexpectedly
error: failed to push some refs to 'http://localhost:.../git/org_playground/<id>.git'
push failed (playground session <id> is live with base_sha: null): exit status 1
```

Portal log: `POST /git/org_playground/<id>.git/git-receive-pack ...
status:200, duration_ms:1, bytes:166`.

Surfaced by e2e tests `TestCLI_JamPlayground` and
`TestPlayground_SoloCreatePushTombstone` (currently `t.Skip`-annotated
pending this fix). Tracked in
`.work/backlog/bug-playground-git-receive-pack-fails-with-200-hangup.md`.

## Root cause

`prereceive.Validator.Validate` calls `WalkAndValidate` on every ref
update without exemption. `WalkAndValidate` walks the pushed commits
and `validateCommit` runs `CheckRequiredTrailers` against
`requiredTrailers = []string{"Jam-Session", "Jam-Turn", "Jam-Author"}`
(`internal/portal/prereceive/commits.go:15`).

For a base-ref first push (`refs/heads/jam/<session>/base` with
`OldSHA=""`), the pushed commits are the user's pre-session working-tree
commits. By definition those commits predate the session existing and
cannot carry session-aware trailers naming this session. The trailer
requirement exists to attribute *collaborative session work* — it's a
category error to apply it to the bootstrap commits that establish the
starting state.

The 200/166-byte HTTP response is `writeReportStatusRejection` emitting
`unpack ok` + `ng refs/heads/jam/<id>/base <missing-trailers-message>` +
flush pkt. Git client sees the truncated success/reject mix and surfaces
"fatal: the remote end hung up unexpectedly" instead of a proper
`remote: rejected:` message.

This is the chicken-and-egg the parked bug calls out: trailer enforcement
on the bootstrap push is incoherent because the bootstrap commits cannot
satisfy the trailer requirement.

## Fix approach

In `internal/portal/prereceive/validate.go`, skip `WalkAndValidate`
(both trailer check and scope check) when the update is the base-ref
first push:

```go
for _, u := range in.Updates {
    rejections = append(rejections, ValidateRef(ctx, in.Repo, in.Session.ID, in.Account.ID, u)...)
    if isBaseRefFirstPush(in.Session.ID, u) {
        continue // bootstrap push — trailer/scope validation incoherent
    }
    rejections = append(rejections, WalkAndValidate(ctx, in.Repo, u, scope)...)
}
```

with a new helper:

```go
// isBaseRefFirstPush reports whether u is the inaugural push to the
// session's base ref (refs/heads/jam/<sessionID>/base with empty OldSHA).
// The seed commits the user pushes here predate the session and so
// cannot carry session-aware trailers (Jam-Session, Jam-Turn,
// Jam-Author). Trailer enforcement applies to subsequent collaborative
// pushes only.
func isBaseRefFirstPush(sessionID string, u RefUpdate) bool {
    return u.OldSHA == "" && u.Ref == "refs/heads/jam/"+sessionID+"/base"
}
```

Scope validation is also skipped — the seed commit is the establishing
state of the session; scope rules apply to *additions* to that state.
For the playground default scope `["**"]`, this is a no-op; for
user-specified narrower scopes, the seed is allowed to contain
out-of-scope files because it predates the scope decision.

The exemption is narrow on two axes — only base ref, and only first
push (OldSHA empty). Subsequent pushes to base (rare; usually
force-push for reseeding) still go through full validation.

## Regression test

`internal/portal/prereceive/validate_test.go`:

- `TestValidate_BaseRefFirstPush_ExemptFromTrailerValidation` — asserts
  a base-ref first push with an untrailered seed commit produces
  `OK=true` (the fix). Reproduces the exact production scenario:
  `refs/heads/jam/sess-1/base` + `OldSHA=""` + commit message
  `"initial commit"` (no trailers).
- `TestValidate_BaseRefSecondPush_StillValidated` — asserts a base-ref
  push with `OldSHA` set (i.e. NOT first push) still rejects on missing
  trailers. Proves the exemption is scoped to first push only.
- `TestValidate_NonBaseRef_TrailerStillRequired_RegressionGuard` —
  asserts a normal user-ref push with missing trailers still rejects.
  Proves the exemption doesn't leak to other refs.

Also added `dropRefs` test helper that removes concrete `refs/heads/*`
references from the test repo, matching the production-validation-repo
state (where refs are only updated after pre-receive accepts).

## Implementation notes

### Files changed

1. `internal/portal/prereceive/validate.go` — added `isBaseRefFirstPush`
   helper and the `continue` skip in the per-ref loop. Narrow exemption:
   only base ref (`refs/heads/jam/<id>/base`), only first push (`OldSHA`
   empty). Both trailer and scope validation skipped for the seed.

2. `internal/portal/prereceive/validate_test.go` — added 3 new tests
   (`TestValidate_BaseRefFirstPush_ExemptFromTrailerValidation`,
   `TestValidate_BaseRefSecondPush_StillValidated`,
   `TestValidate_NonBaseRef_TrailerStillRequired_RegressionGuard`) plus
   the `dropRefs` helper that strips concrete refs from the test repo to
   match production-validation-repo state.

3. `tests/e2e/golden/cli_jam_playground_flag_test.go` — removed `t.Skip`;
   fixed stale token-format assertion (the anonymous bearer is a 64-char
   hex string, no `jamsesh_anon_` prefix — that prefix only exists on the
   internal account email `anon_<id>@playground.local`, never on the
   bearer token itself).

4. `tests/e2e/golden/playground_solo_create_push_tombstone_test.go` —
   removed `t.Skip`; relaxed the post-destruction status assertion from
   strict `== 404` to `in {401, 404}` (both are honest outcomes — the
   bearer is revoked synchronously with destruction, so auth fails before
   the session-lookup branch). Matches the abandonment-destruction
   test's assertion shape.

### Verification

- `go test ./internal/portal/prereceive/ -count=1`: ok
- `go test ./internal/portal/... ./cmd/... -count=1`: only pre-existing
  failures (the parked `bug-playground-join-with-nickname-returns-410-on-fresh-session`
  pair); no new regressions
- `cd tests/e2e && go test ./golden/ -run 'TestCLI_JamPlayground|TestPlayground_SoloCreatePushTombstone|TestPlayground_AbandonmentDestructionSweep' -count=1`:
  3 of 3 PASS end-to-end against real portal + real Postgres + real git
  subprocess.

### Adjacent issues parked for separate consideration

None. The fix is narrowly scoped to the one root cause; all other
playground bugs surfaced during the autopilot run were already either
fixed inline or parked separately.

### Resolves

- `.work/backlog/bug-playground-git-receive-pack-fails-with-200-hangup.md`
  (removed in bookkeeping commit)

### Unblocks

- `e2e-audit-cli-jam-playground-flag-end-to-end` (test now passes
  end-to-end; story already at `stage: review`)
- `e2e-audit-playground-solo-create-push-tombstone-journey` (test now
  passes end-to-end; story already at `stage: review`)
- `e2e-audit-playground-two-participant-join-merge-journey` (Wave 2,
  still at `stage: implementing` — no longer blocked on this bug;
  ready for autopilot pickup)
