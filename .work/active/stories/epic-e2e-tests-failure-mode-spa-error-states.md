---
id: epic-e2e-tests-failure-mode-spa-error-states
kind: story
stage: implementing
tags: [e2e-test, testing, ui]
parent: epic-e2e-tests-failure-mode
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Failure — SPA error states

## Scope

One Playwright spec `tests/e2e/playwright/error-states.spec.ts`
covering user-visible error states in the Svelte SPA.

### States covered

- Expired access-token banner (or redirect to login) when the SPA's
  WebSocket / REST calls return 401
- Malformed magic-link URL: visiting
  `/auth/magic-link?token=garbage` displays a clear error state
- Missing org permission: navigating to a session you're not a member
  of displays the appropriate error state (or redirects)
- Network-loss state: the SPA's WebSocket fails to connect — does the
  UI surface a reconnecting indicator?

## Files to create / modify

- `tests/e2e/playwright/error-states.spec.ts` (NEW) — Playwright tests

## Acceptance criteria

- [ ] Each test pins to a stable user-visible DOM element (text,
      role, test-id) — not generic body elements
- [ ] Tests are independent — each can run in isolation
- [ ] Failed runs produce a Playwright trace for debugging
- [ ] No `setTimeout` / `page.waitForTimeout` — always wait on
      observable state

## Notes for the implementer

- The SPA's error-state UI is in `frontend/src/lib/` — inspect the
  components to find stable selectors. The Login.svelte component has
  a "check your inbox" state; check for similar error states.
- For the expired-token test: simulate the expired-token scenario by
  setting `localStorage.access_token` to a known-invalid value before
  navigating to a protected route. Assert the redirect / banner
  appears.
- For the malformed magic-link test: navigate directly to
  `/auth/magic-link?token=garbage` and assert the error state renders
  (or that the user is redirected somewhere reasonable).
- The "missing org permission" and "network-loss" tests may require
  features that aren't yet implemented in the SPA. If a state doesn't
  exist, file a follow-on for the SPA feature and skip the
  corresponding test in this spec (with a clear `test.skip()` and a
  comment pointing to the follow-on).

## Notes on selector stability

Login.svelte uses `placeholder="you@example.com"` (no test-id). For
error states, look for either:
- Test-id attributes (`data-testid="error-banner"`)
- Stable role + name selectors (`getByRole("alert")`)
- Heading text (`getByRole("heading", { name: /expired/i })`)

Document the chosen selector and rationale at the top of each test.
