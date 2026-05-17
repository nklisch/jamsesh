---
id: docs-scope-glob-validation-rules
kind: story
stage: implementing
tags: [documentation, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Document session-scope glob validation rules

## Finding

After commit `3c43677` (pre-receive-validators fuzz), `CompileScope`
in `internal/portal/prereceive/scope.go` now rejects malformed glob
patterns at session-creation time rather than letting them panic at
push time. The `probeGlob` helper detects patterns the upstream
`gobwas/glob` library accepts but can't actually match against (e.g.,
unclosed `{`, invalid UTF-8 sequences).

This is a behavior change visible to callers — session creation with a
malformed `writable_scope` now returns an error.

## Why it matters

- Foundation-doc principle: docs describe the system NOW. The scope
  field's validation contract should be documented.
- API consumers (the SPA, the CLI's `jamsesh join`, any third-party
  integrations) need to know what shapes of glob are accepted.

## Suggested resolutions

Update `docs/SPEC.md` (or wherever session config is documented) with:

1. The supported glob shapes (`**`, `*`, `?`, `{a,b,c}`, character
   classes per gobwas/glob's documented syntax).
2. The rejection contract — malformed patterns return an error at
   session-creation with a clear code (e.g., `scope.invalid_pattern`).
3. Cross-reference to the backlog item for replacing gobwas/glob
   (`bug-gobwas-glob-panic-on-malformed-pattern`) so future readers
   know the underlying library is being evaluated.

## Acceptance criteria

- [ ] `docs/SPEC.md` documents the writable-scope glob contract
- [ ] If a documented error code doesn't exist yet, file a follow-on
      for the portal to emit one
- [ ] Rolling-foundation principle observed (no "previously this was…"
      notes)
