---
id: gate-cruft-ws-eventenvelope-local-alias-stale
kind: story
stage: implementing
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: cruft
created: 2026-05-31
updated: 2026-05-31
---

# Local `EventEnvelope` compatibility alias remains after generated type landed

## Confidence
Medium

## Category
compatibility shim

## Location
`frontend/src/lib/ws.svelte.ts:31`

## Evidence
```ts
// EventEnvelope note:
// `EventEnvelope` is not yet in docs/openapi.yaml - no WS event
// schemas have landed. Until the discriminated union is generated,
// this module types the envelope as an open-ended object.
```

`docs/openapi.yaml:144` defines `EventEnvelope`, and
`frontend/src/lib/api/types.gen.ts:831` contains the generated
`components["schemas"]["EventEnvelope"]`.

## Removal
Import the generated `components` type, replace the local open-ended alias with
`components['schemas']['EventEnvelope']`, and remove the stale explanatory block.


## Autopilot scope/design (2026-05-31)

Promoted by the gate-drain autopilot run. Implement the remediation direction above as a focused single-stride story, keep edits limited to the named surface, and verify with the targeted test or check that covers the changed file. For older backlog gate items, this run binds the work to `v0.5.0` because the user explicitly requested all gate-related work be scoped, designed, and implemented before release.
