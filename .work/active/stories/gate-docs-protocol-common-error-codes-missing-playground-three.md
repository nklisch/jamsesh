---
id: gate-docs-protocol-common-error-codes-missing-playground-three
kind: story
stage: drafting
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
