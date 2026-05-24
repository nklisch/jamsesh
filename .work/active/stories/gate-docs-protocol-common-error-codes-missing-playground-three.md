---
id: gate-docs-protocol-common-error-codes-missing-playground-three
kind: story
stage: review
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# PROTOCOL.md `Common error codes` list omits the playground error codes

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/PROTOCOL.md:422-436`
- Code: `internal/portal/playground/handler.go:221,229,244,262`, `internal/portal/prereceive/playground_caps.go:12`

## Current doc text
> The "Common error codes" bullet list includes `session.invalid_writable_scope` (which mentions playground in passing) but does not enumerate `playground.session_full`, `playground.session_ended`, or `playground.size_exceeded`.

## Reality
Three playground-specific error codes ship and are returned by the new endpoints / pre-receive checks; all three are described in `docs/SPEC.md` (lines 288, 300) and `docs/SECURITY.md` (lines 335, 348) but absent from PROTOCOL.md's authoritative error-code roster.

## Required edit
Add three bullets to the error-code list in `docs/PROTOCOL.md`: `playground.session_full` (409; playground join at MaxParticipants cap; body includes `retry_after_seconds`), `playground.session_ended` (410; playground session past `hard_cap_at`), `playground.size_exceeded` (pre-receive; playground content-size cap reached).

## Implementation notes

Three bullets added to the "Common error codes" list in `docs/PROTOCOL.md`, inserted between `fork.invalid_target_ref` and `oauth.invalid_grant` to keep the playground-specific codes grouped:

1. `playground.session_full` (409) — verified via `JoinPlaygroundSession409JSONResponse` in `internal/portal/playground/handler.go:249`; `retry_after_seconds` confirmed in `docs/openapi.yaml:3295`.
2. `playground.session_ended` (410) — verified via `JoinPlaygroundSession410JSONResponse` in `internal/portal/playground/handler.go:226,234,274`.
3. `playground.size_exceeded` (pre-receive) — verified via `CodePlaygroundSizeExceeded` constant and `Rejection` struct in `internal/portal/prereceive/playground_caps.go:12,55`; `details` fields listed from `map[string]any` at line 57.

No discrepancies between story-named codes and actual code constants. Format follows the existing inline-parenthetical style used by `session.invalid_writable_scope` and `oauth.invalid_grant`.
