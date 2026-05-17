---
id: epic-e2e-tests-failure-mode-spa-error-states
kind: story
stage: done
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

## Implementation notes

### Tests written (6 active, 2 skipped)

| # | Test name | Selector | Source component |
|---|-----------|----------|-----------------|
| 1 | `unauthenticated visit to protected route redirects to login` | `getByPlaceholder("you@example.com")` + URL assertion | App.svelte auth guard + Login.svelte |
| 2 | `no-token visit to protected route lands on login` | same as #1, with explicit localStorage.removeItem | same |
| 3 | `magic-link request failure shows error state` | `getByRole("heading", { name: "Something went wrong" })` + getByText(error copy) | Login.svelte mode === 'magic-link-error' |
| 4 | `try-again from magic-link error returns to login form` | `getByRole("button", { name: "Try again" })` → email input re-appears | Login.svelte |
| 5 | `unknown route renders page-not-found heading` | `getByRole("heading", { name: "Page not found" })` | NotFound.svelte |
| 6 | `session-list shows load error on 403 response` | `getByText("Failed to load sessions.")` | SessionList.svelte loadError paragraph |

### Skipped tests and reasons

- **`stale bearer token on API call triggers 401 sign-out and login redirect`** — Skipped. `auth.svelte.ts` stores tokens under `jamsesh.token`. The App.svelte auth guard only checks token presence (non-null), not validity. A stale non-null token passes the guard; the SPA reaches the backend and gets a 401, but no 401-interceptor in `frontend/src/lib/api/client.ts` currently calls `auth.signOut()`. Re-enable once the API client has a 401 handler.

- **`network-loss state shows reconnecting indicator in session view`** — Skipped. `ws.svelte.ts` has no reconnect logic and no UI indicator for a dropped WebSocket. The `close` event handler only removes the socket from the map. Re-enable once reconnect + a stable status-element (`role="status"` with `/reconnecting/i` text) lands in `ws.svelte.ts`.

### Key codebase findings

- Token localStorage key is `jamsesh.token` (not `access_token`) — from `auth.svelte.ts` `TOKEN_KEY` constant.
- The SPA router (`router.svelte.ts`) has no `/auth/magic-link` route; visiting that path renders NotFound.
- `Login.svelte` error mode renders `<h1>Something went wrong</h1>` and a "Try again" ghost button — no `role="alert"`, so heading text is the stable selector.
- `SessionList.svelte` surfaces load errors as a `<p class="error-msg">` paragraph; text content `"Failed to load sessions."` is the stable selector.
- Tests #3, #4, and #6 use `page.route()` to intercept API calls, so they run without a live portal.

## Review (2026-05-17)

**Verdict**: Approve with comments

**Blockers**: none
**Important**:
- The two skipped tests surface real production-side SPA gaps. Filed as backlog: `spa-api-client-401-interceptor` (stale-token handling) and `spa-websocket-reconnect-logic` (WS reconnect + UI indicator). Both tests are ready to un-skip once the corresponding SPA features land.

**Nits**:
- Token localStorage key discovered to be `jamsesh.token` not `access_token` — agent correctly inspected the SPA. Good catch.

**Notes**: 6 active tests pin to stable selectors (heading text, button role+name, placeholder); all use semantic Playwright matchers (`getByRole`, `getByText`, `getByPlaceholder`). The `page.route()`-based API stubbing in tests 3/4/6 lets them run without a live portal, which is the right boundary for SPA-error-state coverage.
