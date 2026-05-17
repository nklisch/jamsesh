---
id: docs-scope-glob-validation-rules
kind: story
stage: done
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

- [x] `docs/SPEC.md` documents the writable-scope glob contract
- [x] If a documented error code doesn't exist yet, file a follow-on
      for the portal to emit one
- [x] Rolling-foundation principle observed (no "previously this was…"
      notes)

## Implementation notes

The "Finding" above was partially stale by the time this story landed.
Two corrections from reading the code:

1. **`gobwas/glob` has already been replaced.** Commit `9ae141f`
   (`implement: backlog-replace-gobwas-glob`) switched
   `internal/portal/prereceive/scope.go` to
   `github.com/bmatcuk/doublestar/v4`. The `probeGlob` recover wrapper
   from commit `3c43677` is gone — doublestar validates patterns at
   parse time and never panics. `CompileScope` returns a normal Go
   error on malformed input.
2. **Validation runs at push time, not session-create time.**
   `internal/portal/sessions/handler.go > CreateSession` and
   `PatchSession` accept `req.Body.Scope` and store it without calling
   `CompileScope`. Validation happens in
   `internal/portal/prereceive/validate.go:46` when the pre-receive
   hook compiles the scope on a push. Malformed patterns surface as an
   opaque "internal failure" git error to the pusher rather than a
   structured API response to the session creator.

### What landed

`docs/SPEC.md` gained a new `## Writable scope syntax` section between
the existing `## Session shape` and `## Ref structure` sections,
covering:

- The actual matcher (`bmatcuk/doublestar/v4`) and supported glob
  shapes (`**`, `*`, `?`, `[abc]`, `{a,b}`, escaping).
- Path-separator conventions (`/`, relative to repo root).
- The validation contract — patterns are validated at push time,
  malformed patterns reject the push, no API-time pre-validation
  today.
- An example scope + an example rejection.

The `## Session shape > Writable scope` bullet now cross-references
the new section.

### Follow-on parked

The API-time validation gap (the "Finding" claimed this already
existed; it does not) is parked as
`.work/backlog/portal-validate-writable-scope-at-create-time.md`. That
story adds `CompileScope` to the create/patch handlers and registers a
`session.invalid_writable_scope` error code in `docs/PROTOCOL.md`. When
it lands, the "currently does not pre-validate" caveat in SPEC.md gets
removed.

## Review findings — nits

- The SPEC's `**` description matches doublestar's documented behavior
  ("matches across directory boundaries"), but the implementation in
  `internal/portal/prereceive/scope.go` includes a `normalizeForDoublestar`
  helper that rewrites gobwas-style patterns like `**.go` into
  `**/*.go` before handing them to doublestar. This is a deliberate
  back-compat affordance for callers still emitting gobwas-style scopes.
  Not a bug — the user-facing contract documented in SPEC.md is the
  doublestar shape — but a future SPEC pass could either (a) document
  the normalization affordance, or (b) drop it and require doublestar-
  shaped scopes everywhere. No action taken — leaving the SPEC as the
  forward-facing contract is the right call.
