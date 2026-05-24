---
id: story-spec-discipline-audit-and-close-emit-vs-yaml-gaps
kind: story
stage: implementing
tags: [portal, ui]
parent: feature-spec-discipline
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Audit Go emit vs openapi.yaml; close all event-type spec gaps

## Brief

Feature-design Phase 3 discovery: the original story scope ("add
`playground.activity_reset` and `session.destroyed`") was based on a
wrong frontend assumption about server emit names. The portal-ui
session-view-extensions story (commit `d50e575`) corrected the frontend
to use the actual server-emitted events (`session.ended`,
`playground.destruction_warning`) — both of which ARE in
`docs/openapi.yaml`.

A proper audit instead revealed a different, real gap class:

- **`session.created`** — emitted by `internal/portal/sessions/handler.go:155`
  and referenced by `frontend/src/lib/screens/SessionList.svelte`
  (used to refresh the session list), but **absent from `docs/openapi.yaml`'s
  `EventEnvelope.type` enum**. No payload schema declared either.

- **`playground.destruction_warning`** — schema IS in
  `docs/openapi.yaml` (lines 172 + 194 + 210) but **absent from the
  generated `frontend/src/lib/api/types.gen.ts`** — `make generate`
  hasn't been re-run since that schema landed.

This story closes the gaps and runs codegen.

## Concrete work

### 1. Audit (verify before adding)

Run the complete audit. Compare every event-type string emitted by Go
(`grep -rE 'Emit\(.*"[a-z]+\.' internal/portal --include='*.go' | grep -v _test.go`)
against the YAML enum. Known gaps as of 2026-05-24:

| Event | Emitted | In YAML enum | In types.gen.ts |
|---|---|---|---|
| `session.created` | `sessions/handler.go:155` | NO | NO |
| `playground.destruction_warning` | TBD (verify emit site) | YES | NO (codegen stale) |

Add any additional gaps the audit surfaces.

### 2. Close YAML gaps

For each emitted-but-not-specced event:

- Add to `EventEnvelope.type` enum
- Add a payload schema (verify field shape against the Go payload struct)
- Add to `payload.oneOf`
- Add to `discriminator.mapping`

For `session.created` specifically, read sessions/handler.go around
line 155 to see the payload struct — likely fields include
`session_id`, `org_id`, `name`, `goal`, `created_at`. The
SessionList consumer's needs from this event define which fields the
schema MUST include.

### 3. Run codegen

```bash
make generate
```

Verify both Go (`internal/api/openapi/*.gen.go`) and TS
(`frontend/src/lib/api/types.gen.ts`) regenerate cleanly.

### 4. Frontend type imports (unblock sibling story)

`frontend/src/lib/screens/SessionViewShell.svelte` and
`CountdownBadge.svelte` still have inline TODOs about replacing
inline event-type annotations with generated types. After codegen,
those TODOs unblock — the sibling story
`story-refactor-replace-inline-event-types-with-openapi-typescript-gen`
does that swap.

## Acceptance criteria

- [ ] Every event-type string emitted by Go exists in `docs/openapi.yaml`'s
      `EventEnvelope.type` enum, `payload.oneOf`, and
      `discriminator.mapping`.
- [ ] Every payload schema in the YAML matches the Go-emitted payload
      struct field-for-field.
- [ ] `make generate` runs clean.
- [ ] `frontend/src/lib/api/types.gen.ts` contains all event types
      and payload schemas.
- [ ] `go build ./...` and `go test ./...` clean.
- [ ] `npm run check`, `npm run test`, `npm run build` clean.

## Implementation discovery

This story originated as "add `playground.activity_reset` and
`session.destroyed`" — but those names don't exist in the codebase.
The actual server events were `session.ended` and
`playground.destruction_warning`. The frontend was correctly
re-pointed to the real events by `d50e575`. The real gap class is
server-emits-not-specced, with `session.created` the first known
concrete example. The CI drift check (sibling story
`story-spec-discipline-drift-ci-check`) catches this class going
forward.

## Risk

**Low.** Spec additions only. Frontend may need minor adjustments
where it was inlining types; switch to generated types.

## Rollback

`git revert` the implementation commit.
