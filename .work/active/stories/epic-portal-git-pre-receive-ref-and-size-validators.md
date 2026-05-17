---
id: epic-portal-git-pre-receive-ref-and-size-validators
kind: story
stage: review
tags: [portal, security]
parent: epic-portal-git-pre-receive
depends_on: [epic-portal-git-pre-receive-commit-validators]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Pre-Receive — Ref Namespace + Force-Push + Pack Size + Top-Level Validate

## Scope

Add the per-ref validators (namespace, force-push, shared-ref
protection), the pack-size guard, and the top-level `Validator.Validate`
entry that the smart-http handler will call.

## Units delivered

- `internal/portal/prereceive/refs.go` — `ValidateRef` per parent feature Unit 5
- `internal/portal/prereceive/size.go` — `CheckPackSize` per Unit 6
- `internal/portal/prereceive/validate.go` — `Validator` type + `Validate(in ValidateInput) (ValidateResult, error)` per Unit 7
- `internal/portal/config/config.go` (edit) — add `Git.MaxPackBytes int64` (default 52428800 = 50 MB)
- Tests

## Acceptance Criteria

- [ ] Ref in user's namespace passes: `refs/heads/jam/<sess>/<account.ID>/<branch>`
- [ ] Wrong-owner ref fails with `push.ref_namespace_violation`
- [ ] First push to `refs/heads/jam/<sess>/base` when repo is empty passes (creator's base push)
- [ ] Push to `refs/heads/jam/<sess>/base` when repo has refs fails (already created)
- [ ] Push to `refs/heads/jam/<sess>/draft` fails with `push.ref_namespace_violation` (server-only ref)
- [ ] Force-push (OldSHA not ancestor of NewSHA) fails with `push.force_push_rejected`
- [ ] Pack exceeding `MaxPackBytes` fails with `push.size_limit`
- [ ] `Validate` aggregates all rejections across all updates and all checks; OK=true only when no rejections

## Notes

- `Validate` is the single entry the smart-http handler calls.
- The first-push exception requires checking that the repo has no refs (use `repo.References()` and count).
- For ancestry check: `mergeBase, _ := repo.MergeBase(oldHash, newHash)` then verify `mergeBase[0].Hash == oldHash`. Or use `object.CommitNode.IsAncestor` if go-git exposes it.

## Wiring

After this story, the smart-http feature (next in the chain) imports `Validator` and calls it with the parsed update list + the streamed pack size. No `cmd/portal/main.go` wiring yet — that lands with smart-http.

## Implementation notes

### Files added
- `internal/portal/prereceive/refs.go` — `ValidateRef` + `checkRefNamespace` + `repoIsEmpty` + `checkForcePush`
- `internal/portal/prereceive/size.go` — `CheckPackSize`
- `internal/portal/prereceive/validate.go` — `Validator` type + `Validate` + `parseWritableScope`
- `internal/portal/prereceive/refs_test.go` — 8 ref namespace / force-push tests
- `internal/portal/prereceive/size_test.go` — 6 pack size tests
- `internal/portal/prereceive/validate_test.go` — 7 top-level integrate tests

### Files edited
- `internal/portal/config/config.go` — added `GitConfig` struct with `MaxPackBytes int64`, wired into `Config`, `defaults()` (52428800 = 50 MiB), and `applyEnv` via `JAMSESH_GIT_MAX_PACK_BYTES`
- `internal/portal/config/config_test.go` — added `TestGitMaxPackBytesEnvOverride`, `TestGitMaxPackBytesYAML`; extended `TestDefaults` and `clearEnv`

### Key implementation choices
- **repoIsEmpty**: skips `plumbing.SymbolicReference` entries (HEAD is always present in a fresh repo) and counts only hash references under `refs/`. Stops at the first real ref via `storer.ErrStop`.
- **force-push detection**: uses `object.Commit.IsAncestor` (go-git v5.19 exposes this directly), walking newCommit's history to check if oldCommit is reachable.
- **empty writable scope**: follows the existing behaviour in `commits.go` — when the compiled scope has no patterns, the path check is skipped (allow-all). The `ScopeMatcher.Match` deny-by-default applies only when the matcher is explicitly queried.
- **test for orphan commits**: go-git's `wt.Commit` always attaches to HEAD when `Parents` is nil (via `CommitOptions.Validate`). The force-push test creates an orphan branch via `git checkout --orphan` + CLI to obtain a genuine divergent root.
