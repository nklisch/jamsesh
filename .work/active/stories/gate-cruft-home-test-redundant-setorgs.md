---
id: gate-cruft-home-test-redundant-setorgs
kind: story
stage: implementing
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: cruft
created: 2026-05-20
updated: 2026-05-20
---

# Redundant `setOrgs([single-org])` immediately overwritten by `setOrgs([two-orgs])`

## Confidence
High

## Category
dead function (dead test setup call)

## Location
`frontend/src/lib/screens/Home.test.ts:184`

## Evidence
```ts
it('clicking an org row navigates via navigate() and prevents default', async () => {
  setOrgs([{ id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' }]);
  // Need 2 orgs so picker renders (single-org auto-routes)
  setOrgs([
    { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
    { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
  ]);
```

## Removal
Delete line 184 (the single-org `setOrgs` call). The inline comment
already acknowledges this — "Need 2 orgs so picker renders (single-org
auto-routes)" was added to explain why the second call replaces the
first. The first call is a refactor leftover.
