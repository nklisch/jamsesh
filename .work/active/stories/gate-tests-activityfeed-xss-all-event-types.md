---
id: gate-tests-activityfeed-xss-all-event-types
kind: story
stage: review
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

## Implementation notes

Added `it.each(xssCases)` table-driven test in `ActivityFeed.test.ts` covering
every `formatEvent` branch and every injectable string field (20 cases total).
Two event types (`session.finalizing`, `session.ended`) have no user-supplied
fields — only static literal text — so they need no XSS cases.
The `default` branch renders `env.type` itself, which is validated against a
closed `as const` union in the subscription setup and never comes from a
user-controlled payload; no XSS case needed.

### Event-type × field coverage matrix

| Event type            | Field                  | Fragment kind | Covered |
|-----------------------|------------------------|---------------|---------|
| commit.arrived        | author_id              | who (emphasis)| yes     |
| commit.arrived        | sha                    | sha           | yes     |
| commit.arrived        | ref                    | txt (template)| yes     |
| commit.arrived        | summary                | txt (template)| yes     |
| merge.succeeded       | source_sha             | sha           | yes     |
| conflict.detected     | source_ref             | who (emphasis)| yes     |
| conflict.resolved     | resolving_commit_sha   | sha           | yes     |
| comment.added         | author_id              | who (emphasis)| yes     |
| comment.added         | body                   | txt           | yes     |
| comment.resolved      | resolved_by            | who (emphasis)| yes     |
| ref.forked            | ref                    | sha           | yes     |
| ref.forked            | parent_sha             | sha           | yes     |
| mode.changed          | ref                    | sha           | yes     |
| mode.changed          | old_mode               | txt (template)| yes     |
| mode.changed          | new_mode               | txt (template)| yes     |
| turn.ended            | user_id                | who (emphasis)| yes     |
| turn.ended            | ref                    | txt (template)| yes     |
| presence.updated      | user_id                | who (emphasis)| yes     |
| presence.updated      | ref                    | txt (template)| yes     |
| session.finalizing    | (no user fields)       | n/a           | n/a     |
| session.ended         | (no user fields)       | n/a           | n/a     |
| default branch        | env.type (closed union)| txt           | n/a     |

### Assertions per case
1. `container.querySelector('img')` is null — no live img element parsed.
2. `window.__pwned_<field>` is undefined — onerror never executed.
3. For non-sha fields: `container.innerHTML` matches `/&lt;img/i` — payload
   rendered as escaped text, not silently dropped.
   For sha fields: `container.textContent` contains the truncated safe probe
   value — confirms the field was rendered after sha() truncation.

### Test count
- 7 pre-existing tests (rendering, subscriptions, capping)
- 2 focused XSS tests from gate-tests-xss-activityfeed-component
- 20 new table-driven XSS cases (this story)
- Total: 29 passing
