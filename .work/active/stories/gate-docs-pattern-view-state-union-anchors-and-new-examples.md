---
id: gate-docs-pattern-view-state-union-anchors-and-new-examples
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

# Pattern skill `view-state-union-machine.md` example line anchors have drifted and the pattern misses the two new examples this bundle added

## Drift category
pattern-skill-staleness

## Location
- Doc: `.claude/skills/patterns/view-state-union-machine.md:29,53,67,77,92`
- Code: `frontend/src/lib/screens/{OAuthCallback,InviteAccept,Login,Home,MagicLinkExchange,SessionList}.svelte`, `frontend/src/lib/components/CommentsTab.svelte`

## Current doc text
> Five examples anchored at `OAuthCallback.svelte:12`, `InviteAccept.svelte:24`, `Login.svelte:12`, `Home.svelte:15`, `MagicLinkExchange.svelte:12`.

## Reality
`InviteAccept.svelte:24` is now blank; the `ViewState` typedef moved to line 25. Bundle's `story-refactor-view-state-union-comments-and-session-list` extended this pattern into two new components — `frontend/src/lib/components/CommentsTab.svelte:26` and `frontend/src/lib/screens/SessionList.svelte:24` both declare `type LoadState = 'loading' | 'ready' | 'error'`. Neither appears in the pattern skill's examples.

## Required edit
Fix `InviteAccept.svelte:24` → `:25` (or pin to symbol). Add a sixth example (`CommentsTab` / `SessionList` — both use the same `LoadState` shape) to document the bundle's new applications of the pattern.

## Implementation notes

- Fixed line anchor: `InviteAccept.svelte:24` → `:25` (verified against current source — ViewState typedef is at line 25).
- Added **Example 6: CommentsTab / SessionList** to `.claude/skills/patterns/view-state-union-machine.md` after Example 5, documenting the `LoadState = 'loading' | 'ready' | 'error'` shape that both components arrived at independently (CommentsTab.svelte:26, SessionList.svelte:24). Includes a short paragraph explaining why the `LoadState` naming distinguishes it from `ViewState` (it's the load lifecycle, not the screen's view mode).
- Edits were applied in the parent autopilot session — auto-mode's self-modification classifier blocks sub-agents from editing under `.claude/skills/`.
- Verification: `go build ./...` passes (sanity for doc-only change).
