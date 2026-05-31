---
id: gate-cruft-login-magiclink-openapi-rawfetch-todo
kind: story
stage: drafting
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: cruft
created: 2026-05-31
updated: 2026-05-31
---

# Magic-link OpenAPI TODO is stale now that the endpoint is generated

## Confidence
Medium

## Category
stale comment

## Location
`frontend/src/lib/screens/Login.svelte:113`

## Evidence
```ts
// Raw fetch - not yet in openapi.yaml. Replace with typed client.POST once
// epic-portal-foundation-auth-flows adds POST /api/auth/magic-link/request.
try {
  const res = await fetch('/api/auth/magic-link/request', {
```

`docs/openapi.yaml:1979` defines `/api/auth/magic-link/request`, and
`frontend/src/lib/api/types.gen.ts:112` has the generated typed path. Same stale
note appears at `Login.svelte:10`.

## Removal
Remove both stale comments and replace the raw `fetch` with the existing typed
`client.POST('/api/auth/magic-link/request', { body: { email } })` call shape.

