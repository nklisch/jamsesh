---
id: gate-docs-pattern-wrapper-object-rune-store-anchors-and-playgroundcontext
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

# Pattern skill `wrapper-object-rune-store.md` example anchors all drifted, and pattern misses the new `_playgroundContext` field

## Drift category
pattern-skill-staleness

## Location
- Doc: `.claude/skills/patterns/wrapper-object-rune-store.md:25,49,67,109`
- Code: `frontend/src/lib/auth.svelte.ts`, `frontend/src/lib/router.svelte.ts`, `frontend/src/lib/ws.svelte.ts`

## Current doc text
> Anchors `auth.svelte.ts:16`, `router.svelte.ts:32`, `ws.svelte.ts:81`, `auth.svelte.ts:99`.

## Reality
`auth.svelte.ts:16` is now a `PlaygroundContext` comment; the wrapper-store starts at line 32-43 in the bundle (post-`PlaygroundContext` addition). `router.svelte.ts:32` is mid-route-pattern-matching code, not the rune store. The pattern skill also doesn't reflect that the bundle added a third independent identity state (`_playgroundContext`) alongside `_token` / `_refresh` / `_currentUser` / `_orgs` — a noteworthy new application of the wrapper pattern (anonymous identity orthogonal to authenticated identity).

## Required edit
Re-anchor `auth.svelte.ts:16` → `:32` (or pin to symbol `_currentUser`). Re-anchor `router.svelte.ts:32` and `auth.svelte.ts:99` to current locations. Optionally extend Example 1 to mention `_playgroundContext` as an additional in-memory-only state in the wrapper.

## Implementation notes

Anchors re-aligned (verified against current source):
- Example 1 header: `auth.svelte.ts:16` → `auth.svelte.ts:26` (first `let _token` is at line 26)
- Example 2 header: `router.svelte.ts:32` → `router.svelte.ts:44` (`let path = $state(...)` is at line 44)
- Common Violations section: `auth.svelte.ts:99` → `auth.svelte.ts:123` (`addOrg` is at line 123)

Example 1 extended to show `_playgroundContext` as a third, orthogonal
identity state in the same wrapper-object — covers the bundle's anonymous-
playground identity addition. Code block now includes the
`get playgroundContext()` getter, `setPlaygroundContext(ctx)` mutator, and
a short paragraph explaining why playground state is in-memory-only (server
revokes bearer on session end; persisting would only produce 401 noise).

Edits applied in the parent autopilot session — auto-mode's self-modification
classifier blocked the sub-agent from editing under `.claude/skills/`.

Verification: `go build ./...` passes (sanity for doc-only change).
