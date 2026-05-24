---
id: story-spec-discipline-add-playground-event-payloads
kind: story
stage: drafting
tags: [portal, ui]
parent: feature-spec-discipline
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Add playground.activity_reset + session.destroyed payloads to docs/openapi.yaml

## Brief

Two WS event types are emitted by the Go server (`internal/portal/`
event-emission code) and consumed by the frontend
(`SessionViewShell.svelte`) but are **absent from
`docs/openapi.yaml`**. This story adds them as full citizens of the
`EventEnvelope` discriminator and runs codegen.

## Current state

`docs/openapi.yaml` declares 13 event types in `EventEnvelope.type`
(lines 159-172). Missing:

- `playground.activity_reset` — fired when activity resets the
  playground idle timer. Frontend inline shape:
  ```ts
  type PlaygroundActivityResetEvent = {
    type: 'playground.activity_reset';
    last_substantive_activity_at: string; // ISO 8601
    idle_timeout_at: string;              // ISO 8601
  };
  ```
- `session.destroyed` — fired when a playground session is destroyed.
  Frontend inline shape:
  ```ts
  type SessionDestroyedEvent = {
    type: 'session.destroyed';
  };
  ```
  (Payload may be empty per the frontend handler, but the Go server
   may carry fields like `session_id`/`destroyed_at` — verify before
   declaring empty.)

Additionally, `PlaygroundDestructionWarningPayload` IS in the YAML
(line 194) but **absent from `frontend/src/lib/api/types.gen.ts`** —
`make generate` has not been re-run since that schema landed.

## Design questions for feature-design / drafting pass

This story is at `drafting` because the YAML edits need to align with
the **actual** server emit shape, which requires verification:

1. **Inspect the Go server-side emit code.** Find where each event is
   emitted (likely `internal/portal/playground/` for activity_reset
   and the destruction worker; check both). Capture the exact payload
   struct fields the server marshals. The frontend inline shape is a
   *guess at what the server sends*; the spec must reflect the actual
   wire shape.

2. **`session.destroyed` payload completeness.** Empty payload is
   common in event-bus designs but unusual in jamsesh — every other
   event carries at least context (`session_id` lives on the
   envelope, not the payload). Verify whether the server-side struct
   includes anything (e.g. `destroyed_at`, `reason`) and reflect that
   in the schema.

3. **Discriminator hygiene.** OpenAPI 3.0.3 + the project's authoring
   rules (per `docs/SPEC.md` and the oapi-codegen 3.1-workaround
   docs). Schemas must be added to: (a) `EventEnvelope.type` enum,
   (b) the `payload.oneOf` list, and (c) the
   `discriminator.mapping` table. All three or the discriminator
   breaks.

4. **Go server alignment.** After codegen, the Go side will have new
   payload struct types in `internal/api/openapi/*.gen.go`. The
   server emit code should switch from any ad-hoc payload structs to
   the generated ones. Audit emit call sites and update.

5. **Frontend codegen-stale check.** `PlaygroundDestructionWarningPayload`
   is the proof that `make generate` wasn't run after a previous
   schema add. Running it now also fixes that side-effect.

## Acceptance criteria (target)

- `docs/openapi.yaml` declares both schemas with field shapes matching
  the verified server-side emit.
- Both schemas appear in `EventEnvelope.type` enum, `payload.oneOf`,
  and `discriminator.mapping`.
- `make generate` is clean (both Go and TS regenerate without error).
- `frontend/src/lib/api/types.gen.ts` contains both new types AND the
  previously-missing `PlaygroundDestructionWarningPayload`.
- Go server emit code uses the generated payload structs (no ad-hoc
  inline types remain).
- `go build ./...` clean, `go test ./...` clean.
- `npm run check` clean, `npm run test` passes, `npm run build` clean.

## Implementation note for the design pass

This is NOT a `[refactor]`-tagged story because it adds capability
(the spec now declares events it didn't before). When the design pass
picks it up, it routes through `feature-design`, not `refactor-design`.
