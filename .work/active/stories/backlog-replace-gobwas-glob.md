---
id: backlog-replace-gobwas-glob
kind: story
stage: review
tags: [security, portal, prereceive, dependency]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Replace gobwas/glob with a library that validates patterns at compile time

## Background

`gobwas/glob@v0.2.3` (the latest stable release as of 2026-05-17) silently
compiles malformed patterns — specifically any pattern of the form
`"<literal-prefix>{"` — without returning an error. Calling `Match` on such a
compiled glob panics with a slice-bounds-out-of-range inside `Row.matchAll`.

The immediate bug was fixed in `bug-gobwas-glob-panic-on-malformed-pattern` via
the `probeGlob` wrapper in `internal/portal/prereceive/scope.go`. That wrapper
is a defense-in-depth workaround: it probes the compiled glob against a set of
short strings in a deferred `recover`, turning the would-be panic into a
compile-time error. It is sound but relies on the probe set covering all
panic-triggering inputs.

## Why replace the library

- `gobwas/glob` has not had a release since 2019 (`v0.2.3`). The project
  appears unmaintained.
- The `probeGlob` approach is a workaround, not a cure. A future change to
  gobwas/glob's internal representation could introduce new panic shapes that
  the probe set does not cover.
- Alternative libraries such as `github.com/bmatcuk/doublestar` validate
  patterns at compile time and return proper errors for malformed input.

## Suggested replacement candidates

- `github.com/bmatcuk/doublestar` — well-maintained, supports `**` recursive
  matching with `/` separator semantics, validates at compile time.
- Consider running the existing fuzz harness (`FuzzPathScopeValidate`) against
  the replacement to confirm no panic regressions and that path-traversal
  properties hold.

## Acceptance criteria

- [x] Replace `gobwas/glob` import with the chosen library in
      `internal/portal/prereceive/scope.go`
- [x] Remove `probeGlob` (no longer needed if the replacement validates at
      compile time)
- [x] All existing tests in `scope_test.go` pass
- [x] Fuzz harness `FuzzPathScopeValidate` passes with the new library
- [x] `go.mod` / `go.sum` updated; no residual `gobwas/glob` dependency
- [ ] Update `docs-scope-glob-validation-rules` backlog if glob syntax changes

## Implementation notes

**Chosen library:** `github.com/bmatcuk/doublestar/v4@v4.10.0`

Well-maintained (active commits as of 2026), supports `**` recursive matching
with `/` as the separator, and validates patterns at parse time — returning
`ErrBadPattern` for malformed input rather than panicking.

**What changed in `scope.go`:**

- `probeGlob` removed entirely. The deferred-recover workaround is no longer
  needed: `doublestar.ValidatePattern` catches all malformed patterns at
  compile time, including the `"0{"` / `"a{"` / `"src/{"` variants that
  triggered the original gobwas panic.
- `ScopeMatcher` struct changed from `{globs []glob.Glob, raw []string}` to
  `{patterns []string}`. doublestar does not offer a compiled-pattern object;
  pattern strings are stored normalized and matched via `doublestar.Match` on
  each `Match` call.
- Added `normalizeForDoublestar` helper: doublestar treats `**` as a
  recursive wildcard only when surrounded by `/`. A mid-pattern `**` not
  followed by `/` behaves like a single-segment `*` (bash globstar semantics).
  gobwas/glob made `**` universally recursive regardless of context. To
  preserve the existing behavioral contract (`**.md` matches `src/x.md`,
  `src/**.go` matches `src/sub/pkg.go`), the helper rewrites any `**` followed
  by a non-`/` character to `**/*` so the suffix becomes its own path segment.
  This is a pure normalization applied transparently at compile time; callers
  use the same patterns they always have.
- `commits.go:94` updated: field reference `scope.globs` → `scope.patterns`.

**Existing tests:** all 17 `TestScopeMatcher_Match` cases and all
`TestCompileScope` cases (including the three malformed-pattern regression
cases) passed without modification.

**Fuzz harness result:** `FuzzPathScopeValidate` ran for 30 s, executed
~2.06 million iterations across 16 workers, found no failures. All known-bad
seed files (traversal payloads, the original `fc37b996` panic trigger) passed.

**`go.mod` cleanup:** `go mod tidy` completed cleanly. `gobwas/glob` is absent
from both `go.mod` and `go.sum`. `doublestar/v4@v4.10.0` added as a direct
dependency (zero transitive dependencies of its own).

**`docs-scope-glob-validation-rules` backlog:** no glob syntax changes visible
to users — patterns are normalized transparently. No doc update needed.
