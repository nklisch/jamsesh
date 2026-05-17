---
id: backlog-replace-gobwas-glob
kind: story
stage: implementing
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

- [ ] Replace `gobwas/glob` import with the chosen library in
      `internal/portal/prereceive/scope.go`
- [ ] Remove `probeGlob` (no longer needed if the replacement validates at
      compile time)
- [ ] All existing tests in `scope_test.go` pass
- [ ] Fuzz harness `FuzzPathScopeValidate` passes with the new library
- [ ] `go.mod` / `go.sum` updated; no residual `gobwas/glob` dependency
- [ ] Update `docs-scope-glob-validation-rules` backlog if glob syntax changes
