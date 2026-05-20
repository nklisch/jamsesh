---
id: gate-tests-unknown-role-titlecase
kind: story
stage: review
tags: [testing]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: tests
created: 2026-05-20
updated: 2026-05-20
---

# Non-creator (non-"member") arbitrary role string title-casing not covered

## Priority
Low

## Spec reference
Item: `spa-logged-in-landing-home-screen`
Acceptance criterion: "Role badges title-case the role string. The `creator` role uses a distinct class (`role-creator`) ... other roles fall through to the neutral pill." Risks section: "MeOrgMembership.role field is untyped string. ... Future role values render in the picker with title-casing."

## Gap type
missing test for boundary / equivalence partition

## Suggested test
```ts
it('title-cases arbitrary unknown role values', () => {
  setOrgs([
    { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
    { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'reviewer' },
  ]);
  render(Home);
  expect(screen.getByText('Reviewer')).toBeInTheDocument();
  expect(screen.getByText('Reviewer')).not.toHaveClass('role-creator');
});
```

## Test location (suggested)
`frontend/src/lib/screens/Home.test.ts`

## Context
Only `creator` and `member` are tested. Schema is documented as untyped
`string` — the spec promises title-casing works for "any value the
server returns" and unknown roles fall through to the neutral pill.
Equivalence-partition coverage: tested = creator (accent), member
(neutral); untested = arbitrary-other (neutral).

## Implementation notes
Added the test to `Home.test.ts` immediately after the "non-creator badge does not have role-creator class" test, co-located in the "Role badges" section. Used the suggested snippet verbatim — sets two orgs (creator + reviewer), asserts `'Reviewer'` is in the document (verifying `roleLabel` title-cases the arbitrary value) and does not carry the `role-creator` class (verifying it gets the neutral pill). Test passes; total 34 Home.test.ts tests green.
