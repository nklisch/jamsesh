# Pattern: Spec-Driven Event Types

`docs/openapi.yaml` is the single source of truth for every WebSocket
event-type string. The Go constant block `events.AllTypes` in
`internal/portal/events/types.go` must mirror the
`EventEnvelope.type` enum in the YAML exactly. A bidirectional CI test
(`TestEventTypeConstants_MatchOpenAPIYAML`) enforces the invariant in both
directions and fails with an actionable diff if they diverge.

## Rationale

WebSocket event types cross a trust boundary: the Go server emits a string,
the TypeScript client consumes a discriminated union generated from the spec.
If a developer adds an emitter but forgets to update the YAML, the frontend
gets no generated type and the consumer silently receives an unknown payload.
The reverse (YAML declares a type with no emitter) is a spec lie that drifts
into dead documentation. Keeping the YAML as the single authoritative
declaration and then asserting Go matches it catches both failure modes at
CI time, not at runtime.

## The rule

1. **YAML is authoritative.** Every event-type string that the server emits
   MUST appear as an entry in the `EventEnvelope.type` enum in
   `docs/openapi.yaml` before any Go code can emit it.

2. **`events.AllTypes` mirrors the enum.** The `AllTypes` slice in
   `internal/portal/events/types.go` must contain exactly the same set of
   strings — no more, no less.

3. **Codegen is the consumer contract.** `make generate` regenerates
   `internal/api/openapi/server.gen.go` (Go typed constants + accessors)
   and the TypeScript types consumed by the Svelte client from the same YAML.
   Generated files are committed; CI verifies `make generate && git diff
   --exit-code` produces no changes.

## Adding a new event type — the full checklist

1. Add the payload schema to `docs/openapi.yaml` under
   `components.schemas` (e.g. `MyNewEventPayload`).
2. Add the type string to the `EventEnvelope.type` enum in the YAML.
3. Add a `$ref` to `EventEnvelope.payload.oneOf`.
4. Add a `discriminator.mapping` entry mapping the string to the schema ref.
5. Add the same string to `events.AllTypes` in
   `internal/portal/events/types.go`.
6. Run `make generate`.
7. Implement the emitter using the generated `EventEnvelopeType` constant
   from `internal/api/openapi/server.gen.go`.
8. Verify `TestEventTypeConstants_MatchOpenAPIYAML` passes.

## Examples

### Example 1: `session.created` — emitted by the sessions handler

**Spec** (`docs/openapi.yaml`): `session.created` is listed in the
`EventEnvelope.type` enum, has a `SessionCreatedPayload` schema, a `oneOf`
entry, and a `discriminator.mapping` entry pointing at the payload schema.

**Go constant** (`internal/portal/events/types.go`): `"session.created"` is
an entry in `AllTypes`.

**Generated constant** (`internal/api/openapi/server.gen.go:167`):
```go
SessionCreated EventEnvelopeType = "session.created"
```

**Emitter** (`internal/portal/sessions/handler.go:155`):
```go
_, _ = h.events.Emit(ctx, orgID, sessionID, "session.created", payload)
```

### Example 2: `auto-merger.backpressure` — emitted by the auto-merger worker

**Spec**: `auto-merger.backpressure` has a full `AutoMergerBackpressurePayload`
schema at `docs/openapi.yaml:220`, enum entry, `oneOf` branch, and mapping.

**Go constant** (`internal/portal/events/types.go`): first entry in `AllTypes`.

**Generated constant** (`internal/api/openapi/server.gen.go:156`):
```go
AutoMergerBackpressure EventEnvelopeType = "auto-merger.backpressure"
```

**Emitter** (`internal/portal/automerger/worker.go:352`):
```go
if _, err := w.Log.Emit(ctx, e.OrgID, e.SessionID, "auto-merger.backpressure", payload); err != nil {
```

## The CI test

**File**: `internal/portal/events/spec_drift_test.go`

`TestEventTypeConstants_MatchOpenAPIYAML` walks `docs/openapi.yaml`'s parsed
YAML tree to extract `components.schemas.EventEnvelope.properties.type.enum`,
sorts both slices, and compares them with a symmetric difference check. It
locates `docs/openapi.yaml` via `runtime.Caller(0)` so the path is absolute
and survives repo layout changes.

## Failure mode

When the test catches drift it prints an actionable diff to `t.Fatal`:

```
events.AllTypes and the docs/openapi.yaml EventEnvelope.type enum are out of sync.

Only in Go (events.AllTypes) — add to docs/openapi.yaml or remove from AllTypes:
  + my-new.event

Only in YAML enum — add to events.AllTypes or remove from docs/openapi.yaml:
  - some-removed.event

Resolution: see .claude/skills/patterns/spec-driven-event-types.md
```

## Resolution flow when the test fails

**Go has a type that YAML doesn't:**
1. Add the missing type to the YAML enum, `oneOf`, and `discriminator.mapping`.
2. Add the payload schema to `components.schemas`.
3. Run `make generate`.
4. Re-run the test.

**YAML has a type that Go doesn't:**
1. If the emitter exists but `AllTypes` was just not updated: add the string
   to `events.AllTypes`.
2. If the YAML declaration is premature (no emitter yet): remove it from the
   YAML until the emitter is ready, or add a stub entry to `AllTypes` with a
   comment marking it as unimplemented.
3. Run `make generate` and re-run the test.

## When to Use

- Any time you add a new WebSocket event type to the portal server.
- When reviewing a PR that touches `internal/portal/events/` or
  `docs/openapi.yaml` event schemas.

## When NOT to Use

- REST-only response schemas that are never emitted over WebSocket — those
  still belong in the YAML but do not require an `AllTypes` entry.
- Non-portal packages that have no WebSocket surface (CLI, git smart-HTTP).

## Common Violations

- Adding an `Emit(...)` call with a literal string not in `AllTypes` — the
  drift test catches this but only if `AllTypes` is the agreed registry.
  Always add to `AllTypes` first.
- Updating the YAML enum but forgetting `make generate` — the generated
  constant is stale and the TypeScript client has no matching type.
- Removing a type from `AllTypes` without removing the emitter — the emitter
  keeps firing an undeclared type; the drift test catches the asymmetry.
