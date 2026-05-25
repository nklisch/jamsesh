---
id: gate-docs-pattern-per-instance-factory-sessionviewshell-moved
kind: story
stage: done
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# per-instance-factory-rune-store pattern points at moved SessionViewShell

## Drift category
pattern-skill-staleness

## Location
- Doc: `.claude/skills/patterns/per-instance-factory-rune-store.md:89-90`
  (summary list of consumers)
- Code: `frontend/src/lib/screens/SessionViewShell.svelte:54-57` (actual
  location of the four factory calls)

## Current doc text
> 5 factories consumed by `SessionViewShell.svelte:54-57` and
> `NewSessionDrawer.svelte:21`.

The path `SessionViewShell.svelte:54-57` is written as a bare filename,
which implies the file lives in the same directory bucket the rest of the
pattern's anchors point to (`frontend/src/lib/components/`).
`NewSessionDrawer.svelte:21` is also a bare filename.

## Reality
`SessionViewShell.svelte` lives at **`frontend/src/lib/screens/`**, not
under `components/` — the god-component refactor moved it during v0.4.0
(see `feature-refactor-frontend-god-components`). The four factory calls
are still at lines 54-57 of that file, so only the directory path is
wrong.

`NewSessionDrawer.svelte` is at `frontend/src/lib/components/` and its
factory call (`createNewSessionForm`) is at line 21 — that anchor is OK.

## Required edit
In `.claude/skills/patterns/per-instance-factory-rune-store.md:89`, qualify
the `SessionViewShell.svelte:54-57` reference with its full path:
`frontend/src/lib/screens/SessionViewShell.svelte:54-57`. Optionally also
fully-qualify `NewSessionDrawer.svelte:21` →
`frontend/src/lib/components/NewSessionDrawer.svelte:21` for symmetry, so
no reader has to infer the directory from context.

Apply rolling-foundation: just update the path. No "moved from
components/" annotation.

## Implementation notes

`.claude/skills/patterns/per-instance-factory-rune-store.md:89` summary
qualified with full paths:
`frontend/src/lib/screens/SessionViewShell.svelte:54-57` and
`frontend/src/lib/components/NewSessionDrawer.svelte:21`. No
"moved from" prose — present-tense only.

Edits applied in the parent autopilot session. `go build ./...` clean.
