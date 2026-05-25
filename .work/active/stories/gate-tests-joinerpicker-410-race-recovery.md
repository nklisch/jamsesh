---
id: gate-tests-joinerpicker-410-race-recovery
kind: story
stage: implementing
tags: [testing, ui, playground]
parent: feature-test-spec-drift-and-coverage
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-24
updated: 2026-05-25
---

# `JoinerPicker` 410 race-recovery test missing

## Priority
Low

## Spec reference
Item: `story-epic-ephemeral-playground-portal-ui-anonymous-entry`

Acceptance criterion: AC: "on 409: renders the 'session full' message", "on 410: redirects to the tombstone page", "does not fire POST if viewState is already joining."

## Gap type
complementary coverage

## Suggested test
Test asserts that if a 410 races a user double-click on the join button,
only the first request fires AND the user is redirected (not re-rendered into
the picker with an error).

## Test location (suggested)
`frontend/src/lib/screens/JoinerPicker.test.ts`

## Implementation

Add one test to the `'Guards'` describe section of
`frontend/src/lib/screens/JoinerPicker.test.ts`:

```typescript
it('on 410 racing a double-click: fires POST only once and redirects to tombstone', async () => {
  let resolve410: (v: unknown) => void = () => {};
  mockPOST.mockReturnValue(new Promise((r) => { resolve410 = r; }));

  render(JoinerPicker, { props: DEFAULT_PROPS });
  const form = document.querySelector('form')!;

  // Double-click: second submit fires before first resolves
  void fireEvent.submit(form);
  void fireEvent.submit(form);

  // Guard: only one POST despite two submits
  await waitFor(() => {
    expect(mockPOST).toHaveBeenCalledTimes(1);
  });

  // Resolve in-flight request with 410
  resolve410({
    data: undefined,
    error: { error: 'playground.session_ended', message: 'session ended' },
    response: { status: 410 },
  });

  // Assert: navigates to tombstone, no error UI rendered
  await waitFor(() => {
    expect(mockNavigate).toHaveBeenCalledWith('/playground/s/sess-pg-1/ended');
  });
  expect(screen.queryByRole('alert')).toBeNull();
});
```

This test combines the existing double-submit guard test pattern with the
existing 410-redirect path to cover the specific race described in the story
AC. The `viewState === 'joining'` guard prevents the second POST; the 410
branch calls `navigate()` and returns without setting an error state.

File: `frontend/src/lib/screens/JoinerPicker.test.ts` — the Guards section
starts around line 331 in the existing file.
