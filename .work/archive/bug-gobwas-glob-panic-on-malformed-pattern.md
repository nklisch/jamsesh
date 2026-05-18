---
id: bug-gobwas-glob-panic-on-malformed-pattern
kind: story
stage: done
tags: [bug, security, portal, prereceive]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Bug: gobwas/glob panics on Match when pattern has unclosed `{`

## Discovery

Discovered by `FuzzPathScopeValidate` harness during implementation of
`epic-e2e-tests-fuzzing-pre-receive-validators`. Discovering seed:

```
internal/portal/prereceive/testdata/fuzz/FuzzPathScopeValidate/fc37b996e5096fc7
```

Contents: pattern `"0{"`, path `"0"`.

## Reproduction

```go
g, _ := glob.Compile("0{", '/')
g.Match("0") // panics: runtime error: slice bounds out of range [:2] with length 1
```

## Root cause

`gobwas/glob@v0.2.3` compiles a pattern with an unclosed `{` (group/alternatives
syntax) without returning an error. The resulting internal `Row` matcher has a
malformed matcher slice. When `Match` is called, `Row.matchAll` accesses
`segments[:2]` on a 1-element slice, causing a panic.

## Production impact

The pre-receive pipeline calls:

```
session.WritableScope (JSON) → parseWritableScope → CompileScope → scope.Match(path)
```

`CompileScope` wraps `glob.Compile` and returns the error — **but gobwas/glob
returns no error** for patterns like `"0{"`. The compiled glob is stored
silently. Any subsequent push to that session panics the portal process.

Attack surface: any user with permission to create or update a session can
set a malformed scope glob and cause a panic on every push to that session.

## Fix options

1. **Validate on compile** — after `glob.Compile`, call `g.Match("")` in
   `CompileScope` inside a deferred `recover()`. If it panics, return an error.
   This is a defensive wrapper that catches the library bug without a full fork.
2. **Upgrade gobwas/glob** — check whether a newer release fixes the panic.
   As of 2026-05-17 `v0.2.3` is the latest stable. No upgrade exists.
3. **Replace gobwas/glob** — switch to a maintained library that does not
   panic on malformed patterns (e.g. `github.com/bmatcuk/doublestar`).

**Chosen path: Option 1.** The `probeGlob` wrapper (already landed in
`scope.go`) is the correct minimal fix. It probes each compiled glob against
byte-prefixes of the pattern plus common short strings inside a deferred
`recover`, turning the would-be panic into a compile-time error. Options 2 and
3 are out of scope for this story — see the backlog item
`backlog-replace-gobwas-glob` for the replacement follow-up.

## Files to touch

- `internal/portal/prereceive/scope.go` — add panic-recovery validation in
  `CompileScope` (or a separate `validateCompiledGlob` helper).
- `internal/portal/prereceive/scope_test.go` — add a test for the pattern `"0{"`.
- The fuzz harness `FuzzPathScopeValidate` will turn green once this is fixed.

## Partial mitigation (inline fix, story epic-e2e-tests-fuzzing-pre-receive-validators)

A `probeGlob` wrapper was added to `CompileScope` in
`internal/portal/prereceive/scope.go` as an inline fix. It probes each
compiled glob against short strings (including byte-prefixes of the pattern)
inside a deferred `recover`, turning the would-be panic into a compile-time
error. The fuzz harness now passes.

The remaining work in this story was to add a regression test in `scope_test.go`
for the `"0{"` pattern and document the chosen fix path. Upgrading or replacing
`gobwas/glob` is tracked as a separate follow-up backlog item.

## Implementation notes

### What was added

Regression test in `internal/portal/prereceive/scope_test.go` within the
existing `TestCompileScope` function. Three subtests cover the known-bad class
of patterns — a literal prefix followed by an unclosed `{` — which gobwas/glob
compiles silently but panics on Match:

- `rejects malformed: unclosed brace after digit` — pattern `"0{"` (primary
  fuzz trigger, seed `fc37b996e5096fc7`)
- `rejects malformed: unclosed brace after alpha` — pattern `"a{"` (same class,
  alpha prefix)
- `rejects malformed: unclosed brace after path prefix` — pattern `"src/{"`
  (longer prefix, exercises the byte-prefix probe loop further)

Each subtest calls `CompileScope([]string{pattern})` and asserts the returned
error is non-nil.

### Discovery during implementation

A bare `"{"` (no literal prefix) does NOT panic. gobwas/glob treats it as an
empty alternatives group and `Match("")` returns `true`. Only patterns of the
form `"<literal-prefix>{"` trigger the panic because the literal prefix produces
a two-element internal matcher that `Row.matchAll` then indexes out of bounds.
The test was corrected to remove the bare `"{"` case (it is not a security bug)
and replaced with a more informative `"a{"` case from the same panic class.

### Files touched

- `internal/portal/prereceive/scope_test.go` — regression tests added
- `.work/active/stories/bug-gobwas-glob-panic-on-malformed-pattern.md` — this file
- `.work/backlog/backlog-replace-gobwas-glob.md` — follow-up filed

### Verification

```
go test ./internal/portal/prereceive/...   # PASS (all 5 TestCompileScope subtests, 17 TestScopeMatcher_Match subtests)
go build ./...                             # ok
```

### Follow-up

A parked backlog item `backlog-replace-gobwas-glob` tracks the longer-term
option of replacing `gobwas/glob` with a library that validates patterns
correctly at compile time (e.g. `github.com/bmatcuk/doublestar`). Not blocking
this story — the `probeGlob` defense is sound.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**:
- Tests verify the security contract (malformed-glob class rejected at
  compile time) rather than implementation details of `probeGlob`. Three
  subtests cover the panic class shape — digit prefix, alpha prefix, and
  path prefix — which exercise different positions of the byte-prefix
  probe loop. Good coverage without padding.
- The "Discovery during implementation" finding (bare `"{"` does not
  panic; only `"<literal-prefix>{"` does) is recorded in both the story
  body and inline test comments. Future readers will not be confused by
  the missing bare-`"{"` case.
- Follow-up backlog item `backlog-replace-gobwas-glob` is well-scoped and
  documents both the workaround's limitations and replacement candidates.
  Right call to defer the library swap — this story stays minimal-safe.
- `go test ./internal/portal/prereceive/...` and `go build ./...` both
  passed after the change.

## What's now possible

The pre-receive scope validator is now defended against the gobwas/glob
`<literal>{` panic class with both an inline runtime probe and an explicit
regression test. A user with permission to set a scope glob can no longer
crash the portal process on push by submitting `"0{"` or similar. The
fuzz harness `FuzzPathScopeValidate` stays green.
