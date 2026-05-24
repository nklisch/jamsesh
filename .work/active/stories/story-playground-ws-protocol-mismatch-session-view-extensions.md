---
id: story-playground-ws-protocol-mismatch-session-view-extensions
kind: story
stage: implementing
tags: [bug, playground, portal, ws, protocol]
parent: feature-epic-ephemeral-playground-portal-ui
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Playground SessionViewShell WS protocol mismatch (countdown never renders, never resets, never tombstones)

## Scope

Surface a triad of protocol mismatches introduced by
`story-epic-ephemeral-playground-portal-ui-session-view-extensions`.
The UI subscribes to / reads from event-types and schema fields that
the backend never emits or carries. Net effect: in production, the
playground countdown badge never renders, the idle timer never resets
on activity, and the destruction transition to the tombstone page never
fires. All four feature-level acceptance criteria that mention WS
behaviour silently fail.

## Reproduction

1. Join (or create) a playground session and reach SessionViewShell.
2. The header should render PlaygroundChip + CountdownBadge.

Observed (per the actual contract):

- `client.GET('/api/orgs/{orgID}/sessions/{sessionID}')` returns the
  generic `Session` schema, which has no `hard_cap_at`,
  `idle_timeout_at`, or `last_substantive_activity_at` fields
  (`docs/openapi.yaml:748-810`). The runtime in-checks in
  `SessionViewShell.svelte:159-167` therefore never pass, and
  `playgroundHardCapAt`, `playgroundIdleTimeoutAt`,
  `playgroundLastActivityAt` stay `null`.
- The `{#if isPlayground && playgroundHardCapAt && playgroundIdleTimeoutAt && playgroundLastActivityAt}`
  gate around `<CountdownBadge>` (`SessionViewShell.svelte:248`)
  therefore never renders the badge.
- Without a CountdownBadge mount, `onremainingupdate` is never called,
  so `idleRemainingMs` / `hardCapRemainingMs` remain `Infinity`, so
  `<DestructionWarningBanner>` never crosses its WARN_THRESHOLD_MS guard
  either. Two acceptance criteria silently fail.
- The WS subscriptions to `'playground.activity_reset'` and
  `'session.destroyed'` (`SessionViewShell.svelte:190-211`) never fire,
  because neither event type exists in `docs/openapi.yaml` EventEnvelope
  discriminator (lines 159-172, 197-210) nor in any Go emitter in
  `internal/`. The session-lifecycle feature added only
  `playground.destruction_warning`. So:
  - idle timer cannot be reset on substantive activity;
  - post-destruction navigation to `/playground/s/:id/ended` never fires.

## Root cause

The story was implemented against a spec that pre-dated the
session-lifecycle feature's actual protocol decisions, and the inline
WS-payload type stubs in `SessionViewShell.svelte:30-39` (with the
acknowledged TODO) were never reconciled with what session-lifecycle
shipped. The Session REST response was also expected to carry timer
fields it does not carry.

## Fix sketch

Pick the smallest set of changes that aligns UI behaviour with the
session-lifecycle contract. Three substantive choices, any combination
of which can land in this story:

1. **Use `playground.destruction_warning` directly.** Drop the
   client-side countdown reset assumption. CountdownBadge can still
   tick locally from a seed value but treat the destruction-warning
   payload's `remaining_seconds` + `reason` as the authoritative
   "render the banner now" signal. The warning is emitted once per
   reason per session per
   `internal/api/openapi/server.gen.go:1218-1224`.
2. **Extend OpenAPI with `playground.activity_reset` and
   `session.destroyed` envelopes.** Update `docs/openapi.yaml` and
   emit them from the lifecycle layer (`internal/portal/playground/`).
   This restores the original design intent of the
   session-view-extensions story.
3. **Add `hard_cap_at` / `idle_timeout_at` /
   `last_substantive_activity_at` to the Session schema for
   playground sessions** (nullable on durable). Without these the
   client cannot even seed CountdownBadge — the WS path alone is
   insufficient at first render.

Implementation note: replace the inline event type stubs in
`SessionViewShell.svelte:30-39` with the
openapi-typescript-generated types once the schema lands. Then
remove the TODO comment.

## Acceptance criteria

- [ ] PlaygroundChip + CountdownBadge actually render in a real
      playground session (verified manually or in an integration test
      that mocks the full REST + WS flow with the agreed contract).
- [ ] Substantive activity (commit push, comment, finalize action)
      visibly resets the idle remaining time within ~1 second.
- [ ] `session.destroyed` (or whatever event replaces it per fix
      choice) navigates to `/playground/s/:id/ended` end-to-end.
- [ ] DestructionWarningBanner renders for both `idle_timeout` and
      `hard_cap` reasons, with hard-cap priority preserved.
- [ ] Inline WS-payload type stubs in `SessionViewShell.svelte` are
      removed in favour of openapi-typescript generated types.
- [ ] `frontend/src/lib/screens/SessionViewShell.test.ts` exercises
      the new WS flow end-to-end (subscribe → state mutation → banner
      visibility / navigation), not just the durable path.

## Notes for the implementing agent

- This fix necessarily spans the portal-ui and session-lifecycle
  features. Coordinate with whoever picks up the OpenAPI extension —
  the existing tests in `frontend/src/lib/components/CountdownBadge.test.ts`
  and `DestructionWarningBanner.test.ts` pass because they exercise
  the components in isolation with hand-crafted props; they do not
  catch this protocol-level gap.
- See also the parent feature's deviation note "WS event payload types
  defined inline with TODO pointing at openapi-typescript regeneration"
  which acknowledged the issue but did not catch that the underlying
  events were also never wired server-side.
