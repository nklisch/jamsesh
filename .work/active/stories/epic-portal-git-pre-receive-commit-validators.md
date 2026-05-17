---
id: epic-portal-git-pre-receive-commit-validators
kind: story
stage: implementing
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

## Sequencing note

The sibling story `ref-and-size-validators` depends on this one for the Rejection types and code constants.
