---
id: gate-tests-picker-submit-name-trim
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

# Picker multi-org submit-name-trim not covered

## Priority
Low

## Spec reference
Item: `spa-logged-in-landing-home-screen`
Acceptance criterion: "Submitting a non-empty name calls `POST /api/orgs` with `{ name: <trimmed> }` exactly once per submit."

## Gap type
missing test for valid partition

## Suggested test
```ts
it('picker state submit also trims the name before posting', async () => {
  setOrgs([
    { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
    { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
  ]);
  mockPOST.mockResolvedValue({ data: { id: 'n', name: 'foo', slug: 'foo' }, error: undefined });
  render(Home);
  const input = screen.getByLabelText('Create another org') as HTMLInputElement;
  input.value = '  foo  ';
  await fireEvent.input(input);
  await fireEvent.submit(input.closest('form')!);
  await waitFor(() => expect(mockPOST).toHaveBeenCalledWith('/api/orgs', { body: { name: 'foo' } }));
});
```

## Test location (suggested)
`frontend/src/lib/screens/Home.test.ts`

## Context
Trim is tested in empty-state (label "Name your org"); same snippet
renders in picker-state with a different label. Since the snippet shares
logic, behavior should be identical — and a test pins that, guarding
against future divergence if the snippet is split.

## Implementation notes
Added the test to `Home.test.ts` immediately after the "picker state shows 'or' divider" test, co-located in the "Picker: create form" section. Used the suggested snippet as-is — it matches the existing test style. The test sets two orgs (to enter picker state), uses the "Create another org" label to find the input, sets a padded value `'  foo  '`, fires an input event, submits, and asserts that `mockPOST` was called with the trimmed `'foo'`. Test passes; total 33 Home.test.ts tests green.
