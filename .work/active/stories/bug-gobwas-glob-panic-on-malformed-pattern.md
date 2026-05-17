---
id: bug-gobwas-glob-panic-on-malformed-pattern
kind: story
stage: implementing
tags: [bug, security, portal, prereceive]
parent: null
depends_on: []
release_binding: null
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
   As of 2026-05-17 `v0.2.3` is the latest stable.
3. **Replace gobwas/glob** — switch to a maintained library that does not
   panic on malformed patterns (e.g. `github.com/bmatcuk/doublestar`).

Option 1 is the minimal safe fix. Options 2/3 are longer-term.

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

The remaining work in this story is to address the root cause (upgrade or
replace gobwas/glob) and add a regression test in `scope_test.go` for the
`"0{"` pattern. The inline fix is a workaround, not a cure.
