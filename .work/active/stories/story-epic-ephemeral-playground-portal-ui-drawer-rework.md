---
id: story-epic-ephemeral-playground-portal-ui-drawer-rework
kind: story
stage: implementing
tags: [ui]
parent: feature-epic-ephemeral-playground-portal-ui
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# NewSessionDrawer rework (CLI + skill output)

## Scope

Story 4 of the parent feature. Replaces the NewSessionDrawer's
POST-to-create-API logic with a CLI + skill command-output renderer.
The drawer becomes "what to ask your agent" + "what to type yourself"
generator, parallel to the agent-primary mental model locked in for
the wave-1 cli-first-creation feature.

Full design in the parent feature body's "Story 4" section.

## Files delivered

- `frontend/src/lib/components/NewSessionDrawer.svelte` (modify)
- `frontend/src/lib/components/NewSessionDrawer.test.ts` (extend)

## Acceptance criteria

See the parent feature body's "Story 4 acceptance criteria" section.
Summary: form submit renders two copyable command blocks (skill +
raw CLI); copy buttons work; no API call made on submit; form
validation still runs.

## Notes for the implementing agent

- The two command forms are:
  1. `/jamsesh:jam --org X --goal '<text>' --scope '<glob>' --mode <mode> --invite a@x,b@y`
     (skill form — pastes into CC; consumed by the wave-3 plugin-skills
     feature's `/jamsesh:jam` skill body)
  2. `jamsesh new --org X --goal '<text>' --scope '<glob>' --mode <mode> --invite a@x,b@y`
     (raw CLI form — pastes into a terminal)
- Quote the goal/scope/invite values with single-quotes if they
  contain spaces or special characters. Use a small shell-escape
  helper rather than naively concatenating.
- Empty form fields: omit the corresponding flag entirely (don't render
  `--goal ''`). This keeps the rendered command minimal and idiomatic.
- Clipboard copy: use `navigator.clipboard.writeText(...)` with a small
  success toast / state-change indicator on the button ("Copied!").
- The drawer stays in the same place in the UI (triggered from
  SessionList's "New session" button). The change is just what happens
  on submit.
- This story is independent (`depends_on: []`) — doesn't need the
  router refactor or any of the playground-side changes. Can run in
  parallel with Stories 1, 2, 3.
