---
id: gate-cruft-ws-test-unused-close-binding
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

# Unused `close` binding in WebSocket reconnect test

## Confidence
High

## Category
unused variable

## Location
`frontend/src/lib/ws.test.ts:1154`

## Evidence
```ts
const { subscribe, close } = await import('$lib/ws.svelte');
```

`tsc --noUnusedLocals --noUnusedParameters` reports: `'close' is declared but its value is never read.`

## Removal
Remove `close` from the destructure if the test is meant to use `unsub()`, or
change the teardown step to call `close('sess-ac2')` and drop the unused `unsub`
binding if the test title is accurate.

