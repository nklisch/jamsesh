---
id: epic-portal-git-pre-receive-commit-validators
kind: story
stage: review
tags: [portal, security]
parent: epic-portal-git-pre-receive
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Pre-Receive — Commit Validators (Trailers + Scope)

## Scope

Build the per-commit validation primitives: rejection types,
trailer parser, scope-glob matcher, and the commit walker that
applies them.

## Units delivered

- `internal/portal/prereceive/types.go` — Rejection, RefUpdate, ValidateInput, ValidateResult, error code constants
- `internal/portal/prereceive/trailers.go` — `Trailers(msg)` + `CheckRequiredTrailers`
- `internal/portal/prereceive/scope.go` — `CompileScope`, `ScopeMatcher.Match`
- `internal/portal/prereceive/commits.go` — `WalkAndValidate` per parent feature Unit 4
- Tests
- go.mod: add `github.com/gobwas/glob@latest`

## Acceptance Criteria

- [ ] Required-trailer test matrix: present/absent/empty for each of `Jam-Session`, `Jam-Turn`, `Jam-Author`
- [ ] Scope match: `docs/**` matches `docs/foo/bar.md`; `*.md` matches `README.md` but not `src/x.md` (unless `**.md`)
- [ ] WalkAndValidate visits each NEW commit in `OldSHA..NewSHA`; rejects on missing trailer or out-of-scope path with proper Code + Details
- [ ] Tests use a synthetic repo (via go-git's `git.PlainInit` + commit helpers) — no testdata corpus needed
- [ ] No DB, no HTTP, no events — pure validation library

## Notes

- The auto-loaded `go-git` skill carries the API patterns for
  ref walks + commit-tree diffing. Consult for `commitObject.NewIter`
  + `object.NewLogIter` or whichever shape is current.
- Trailer parser is intentionally simple (last paragraph, `Key:
  value` lines). Document the simplification.
- gobwas/glob with separator `/` provides `**` matching.

## Implementation notes

- `go.mod`: added `github.com/gobwas/glob v0.2.3` (latest as of 2026-05-16).
- `types.go`: defines all five `Code*` constants, `Rejection`, `RefUpdate`,
  `ValidateInput`, and `ValidateResult`. `ValidateInput` references
  `*git.Repository`, `*store.Session`, `*store.Account` directly — no
  abstraction layer needed for this story.
- `trailers.go`: strict last-paragraph-only parser using a regexp
  `^([A-Za-z][A-Za-z0-9_-]*):\s+(.+)$`. Any non-trailer line or mid-block
  blank in the final paragraph disqualifies the block. Folded continuations
  (lines starting with whitespace) are appended to the previous entry value.
  Duplicate keys: first occurrence wins. This matches the simplification
  documented in `docs/PROTOCOL.md` and the go-git skill's trailers reference.
- `scope.go`: `gobwas/glob` compiled with `'/'` as separator so `**` spans
  directories while `*` stays within a single segment. Empty scope denies all
  paths (deny-by-default). `raw` slice retained on `ScopeMatcher` for future
  error messages.
- `commits.go`: uses `repo.Log` starting from `NewSHA`, stops when it reaches
  `OldSHA` (or walks entire ancestry for root-commit case when OldSHA is "").
  Per-commit validation: trailer check first, then scope check (skipped if
  scope is nil or has no patterns). Tree diff via `object.DiffTreeWithOptions`
  with `DefaultDiffTreeOptions` (rename-aware, score 60). Both `From` and `To`
  paths checked; nil `parentTree` (root commit) produces a full-tree diff
  against empty baseline.
- Tests use `git init` + `go-git Worktree.Commit` for a real synthetic repo
  (no testdata corpus). Covers: root commit, 3-commit chain, mid-chain missing
  trailer, out-of-scope path, both violations simultaneously, no-op update,
  nil scope.

## Sequencing note

The sibling story `ref-and-size-validators` depends on this one for the Rejection types and code constants.
