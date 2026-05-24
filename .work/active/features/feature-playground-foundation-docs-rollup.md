---
id: feature-playground-foundation-docs-rollup
kind: feature
stage: implementing
tags: [documentation, playground]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Playground foundation-docs rollup

## Brief

Two foundation-doc updates that the ephemeral-playground epic
promised in design but no story actually owned. Surfaced from
review of `feature-epic-ephemeral-playground-session-lifecycle` and
`story-epic-ephemeral-playground-plugin-skills-destruction-warning`.
Both are described in their parent feature bodies under
"Foundation references" but slipped between stories.

## Why a feature

Two cohesive doc edits across `docs/`. Each child is a single
well-specified file edit, but bundling under one feature lets them
land in a single doc-rollup PR with one verdict and avoids two
separate top-level top-level stories cluttering the substrate.

## Child stories

- `story-playground-foundation-docs-rollup-protocol-destruction-warning` —
  add `playground.destruction_warning` to PROTOCOL.md WebSocket
  event-type taxonomy and digest section
- `story-playground-foundation-docs-rollup-architecture-destruction-worker` —
  add playground destruction worker to ARCHITECTURE.md Components
  list

## Design notes (for /agile-workflow:feature-design)

Both stories are well-specified — the child bodies carry concrete
text suggestions and acceptance criteria. The feature-design pass
should be light: confirm the framing (present-tense, no "previously"
prose per rolling-foundation principle) and sequence the children
(they can land in parallel — no inter-dependency).

## Design decisions

- **Optional add-ons** (all selected): the two children carry their required updates AND the following previously-optional touches:
  - PROTOCOL.md: addressing-convention note for anonymous handles (e.g. `amber-otter` participates in `@<handle>` mentions identically to durable handles) — adds to the protocol-destruction-warning story's scope.
  - PROTOCOL.md: cross-link `docs/openapi.yaml` schema anchors next to each event-type entry so readers can jump to canonical payload field definitions — adds to the protocol-destruction-warning story's scope.
  - ARCHITECTURE.md: update the ASCII portal block diagram (lines ~20–31) to include the destruction worker alongside auto-merger workers — adds to the architecture-destruction-worker story's scope.

  Rationale: the optional touches close drift the original ephemeral-playground epic had implicitly committed to and keep diagram-vs-prose in sync. Cheap to land in the same PRs.

## Acceptance (rollup)

- Both children at stage:done with verdicts ≥ approve
- No drift between PROTOCOL.md and `docs/openapi.yaml` for the
  destruction-warning event
- PROTOCOL.md addressing-convention section mentions anonymous handles
- PROTOCOL.md event-type entries cross-link to their openapi.yaml schemas
- `docs/ARCHITECTURE.md` Components list reads cleanly with the
  destruction worker entry alongside auto-merger workers
- `docs/ARCHITECTURE.md` ASCII portal block diagram includes the
  destruction worker alongside auto-merger workers

## Architectural choice

Docs-only rollup. No production code, no architectural unit selection —
this feature is a documentation alignment pass closing two drift gaps the
ephemeral-playground epic implicitly committed to but no child story
owned. The "architecture" here is editorial:

- **Single PR per child story** (rejected): two narrowly-scoped PRs would
  fragment review and miss the rollup framing. The parent feature exists
  precisely to let both land under one verdict.
- **One squash PR for the feature** (chosen): the two children land in
  parallel, the feature is the unit of review, the rollup is atomic from
  a foundation-doc consumer's perspective.

Per `agile-workflow.md` rolling-foundation principle: every edit reads in
present tense — the destruction worker IS a subsystem, the event IS
emitted — no "newly added" or "previously" framing.

## Implementation Units

### Unit 1: PROTOCOL.md event-type + digest + cross-links
**File**: `docs/PROTOCOL.md`
**Story**: `story-playground-foundation-docs-rollup-protocol-destruction-warning`

Edits (in order of appearance in the file):

1. **§ "WebSocket event types"** (~line 369–382) — add
   `playground.destruction_warning` to the event-type bullet list. Place
   adjacent to existing `session.*` lifecycle entries since it shares
   that semantic family. Payload summary mirrors the openapi schema:
   `{reason: "idle_timeout" | "hard_cap", ends_at, remaining_seconds, session_id}`.
2. **Cross-link openapi.yaml schemas** — alongside each event-type entry
   in the same bullet list, add a parenthetical link of the form
   `(schema: [PlaygroundDestructionWarningPayload](./openapi.yaml#L537))`
   or equivalent anchor reference. Apply to every event-type entry in the
   list for consistency, not just the new one — the cross-link convention
   is per `## Design decisions` an across-the-list touch, so readers can
   jump from any event name to its canonical payload definition.
3. **§ digest section** (~line 178, `pre_turn_digest`) — note that the
   digest may surface an `urgent_events` field for time-sensitive events
   that the binary renders in a prominent "Urgent" section above regular
   digest text. Reference `playground.destruction_warning` as the current
   member of that class.
4. **§ Addressing syntax** (~lines 291–307) — NO EDIT NEEDED. The
   anonymous-handles note ("Anonymous session participants use the same
   `@<nickname>` form…") is already present in PROTOCOL.md at lines
   302–307. Implementer should verify the prose still matches the
   `## Design decisions` intent (mentions `amber-otter` style example,
   explains durable-vs-anonymous equivalence) and leave it untouched.
   Capture this verification in the story implementation notes so the
   reviewer doesn't flag the unchanged section as a missed acceptance
   criterion.

**Acceptance Criteria**:
- [ ] `playground.destruction_warning` listed in event-types bullet list
      with correct payload summary
- [ ] Every event-type entry has a `(schema: ...)` cross-link to its
      openapi.yaml schema anchor
- [ ] `urgent_events` field documented in `pre_turn_digest` section
- [ ] Addressing-convention section verified to already mention anonymous
      handles (no edit, just verification note in PR description)
- [ ] No "previously" / "newly added" framing anywhere in the edits
- [ ] `grep -n "playground.destruction_warning" docs/PROTOCOL.md docs/openapi.yaml`
      shows the event in both files; no drift in payload field names

---

### Unit 2: ARCHITECTURE.md Components list + ASCII diagram
**File**: `docs/ARCHITECTURE.md`
**Story**: `story-playground-foundation-docs-rollup-architecture-destruction-worker`

Edits (in order of appearance in the file):

1. **§ System overview ASCII diagram** (lines ~7–38) — add a line to the
   portal block listing destruction worker alongside auto-merger workers.
   The current line 28 reads `• Auto-merger workers`; add `• Playground
   destruction worker` immediately beneath it, keeping the box-art
   alignment intact. The portal block's vertical real-estate is tight
   but has room — verify the trailing pipe characters line up after the
   edit.
2. **§ Components → Portal** (lines ~44–86) — add a new bold-prefixed
   paragraph between **Auto-merger workers** (line 73) and **WebSocket
   gateway** (line 78). Use the text from the story body verbatim as the
   starting point:

   > **Playground destruction worker** — single background goroutine
   > (started when `JAMSESH_PLAYGROUND_ENABLED=true`) that sweeps active
   > playground sessions on a configurable interval
   > (`JAMSESH_PLAYGROUND_SWEEP_INTERVAL_S`, default 60s). For each
   > session past its idle or hard-cap deadline, runs the destruction
   > cascade — record tombstone, revoke bearers, delete session rows
   > (FK cascades members + events + presence + bearers), delete
   > anonymous accounts, remove the bare repo from disk. Idempotent
   > across steps; partial-failure resumption on the next tick.
   > Periodic tombstone-TTL purge runs every 60th tick.

**Acceptance Criteria**:
- [ ] System overview ASCII diagram lists destruction worker in the
      portal block, alignment preserved
- [ ] Components → Portal contains a **Playground destruction worker**
      paragraph between Auto-merger workers and WebSocket gateway
- [ ] Paragraph covers: goroutine topology, interval config knob,
      destruction cascade summary, idempotency stance, tombstone purge
- [ ] Reads in present tense — no "previously" / "newly added"
- [ ] `grep -n "destruction worker" docs/ARCHITECTURE.md` shows entries
      in both the diagram and the Components list

---

## Implementation Order

Parallel. The two units edit different files with no shared section, no
shared assertion, no read-before-write dependency. Both children have
`depends_on: []` and can be picked up simultaneously by
`/agile-workflow:implement-orchestrator`.

1. **Wave 1 (parallel)**:
   - `story-playground-foundation-docs-rollup-protocol-destruction-warning`
   - `story-playground-foundation-docs-rollup-architecture-destruction-worker`

No Wave 2 — feature advances to review when both children reach
stage:review.

## Testing

No unit-test surface (docs-only). Verification is by inspection plus two
mechanical drift checks the reviewer runs:

1. **Lint** — markdown renders cleanly (no broken anchors, no malformed
   list items). Run whatever the project uses for doc lint, otherwise
   visual inspection.
2. **PROTOCOL.md ↔ openapi.yaml drift check**:
   ```bash
   grep -n "playground.destruction_warning" docs/PROTOCOL.md docs/openapi.yaml
   ```
   Both must surface the event. Payload field names in the PROTOCOL.md
   bullet (`reason`, `ends_at`, `remaining_seconds`, `session_id`) must
   match the openapi `PlaygroundDestructionWarningPayload` schema field
   names exactly.
3. **Cross-link integrity** — every `(schema: ...)` link added to
   PROTOCOL.md event-type entries must resolve to a real anchor or line
   number in `docs/openapi.yaml`. Spot-check at least three.
4. **Present-tense framing** — `grep -niE "previously|newly added|note: in|used to be" docs/PROTOCOL.md docs/ARCHITECTURE.md`
   should return zero results in the diff hunks.
5. **ARCHITECTURE.md diagram alignment** — visual diff of lines ~20–32
   to confirm the box-art pipe characters still align after the new
   bullet is inserted.

## Risks

None of significance. This is documentation-rollup work on present-tense
foundation docs with concrete openapi anchors to cross-link against. The
only failure modes are:

- **ASCII diagram alignment breaks** — mitigated by acceptance criterion
  in Unit 2 plus the reviewer spot-check.
- **Cross-link anchors drift if openapi.yaml is renumbered** — using
  schema-name anchors (e.g. `#/components/schemas/PlaygroundDestructionWarningPayload`
  rendered as a Markdown link to a stable section) rather than raw
  line-number anchors avoids this. Implementer choice between
  schema-name or line-number anchor lives in the story; either is
  acceptable since openapi.yaml is itself a foundation doc that rarely
  shifts unscheduled.

