---
id: gate-patterns-openapi-fetch-anonymous-exchange-exception
kind: story
stage: implementing
tags: [refactor]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: patterns
created: 2026-05-31
updated: 2026-05-31
---

# Document the unauthenticated exchange exception to the openapi-fetch middleware pattern

## Existing pattern
`openapi-fetch-middleware-client`

## Bundle code that diverges
`frontend/src/lib/screens/ResumeExchange.svelte:75`

## Nature of divergence
The screen calls bare `fetch` against an OpenAPI endpoint to intentionally omit
ambient bearer middleware. This is a legitimate exception, but the existing
pattern's "When NOT to Use" section does not document it.

## Reconciliation direction
Update `.agents/skills/patterns/openapi-fetch-middleware-client.md` to document
that intentionally unauthenticated exchange endpoints may bypass the shared
client when ambient bearer middleware would be wrong, and include the
`ResumeExchange.svelte` call site as the example.


## Autopilot scope/design (2026-05-31)

Promoted by the gate-drain autopilot run. Implement the remediation direction above as a focused single-stride story, keep edits limited to the named surface, and verify with the targeted test or check that covers the changed file. For older backlog gate items, this run binds the work to `v0.5.0` because the user explicitly requested all gate-related work be scoped, designed, and implemented before release.
