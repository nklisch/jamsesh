---
id: feature-playground-foundation-docs-rollup
kind: feature
stage: drafting
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

## Acceptance (rollup)

- Both children at stage:done with verdicts ≥ approve
- No drift between PROTOCOL.md and `docs/openapi.yaml` for the
  destruction-warning event
- `docs/ARCHITECTURE.md` Components list reads cleanly with the
  destruction worker entry alongside auto-merger workers
