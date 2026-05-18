---
id: gate-tests-activityfeed-xss-all-event-types
kind: story
stage: implementing
tags: [testing, security, ui]
parent: null
depends_on: [gate-security-xss-html-render-ws-events]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# ActivityFeed needs explicit malicious-payload assertion across all 9 `formatEvent` cases

## Priority
Medium

## Spec reference
Item: `gate-security-xss-html-render-ws-events`
Acceptance criterion: apply across every branch of `formatEvent` (commit
subjects, ref names, author IDs, comment bodies all flow through here).

## Gap type
missing test for valid partition (each event type is its own injection
sink). `ActivityFeed.svelte` has 9+ event-type cases. A single test on
comment body would miss the same bug in `ref.forked.ref` or
`commit.arrived.summary`.

## Suggested test
```ts
// ActivityFeed.test.ts — table-driven over EventEnvelope[]
//   For each event type, supply a payload where each string field is
//   `<img src=x onerror="window.__pwned_<field>=1">`. After mount,
//   assert window.__pwned_<field> is undefined for every field.
```

## Test location (suggested)
`frontend/src/lib/components/ActivityFeed.test.ts`
