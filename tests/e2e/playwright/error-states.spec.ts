import { test, expect } from "@playwright/test";

// ─────────────────────────────────────────────────────────────────────────────
// SPA error-state coverage
//
// Each test pins to a stable user-visible DOM element (text/role), not CSS
// classes. Selectors are documented with their source component and rationale.
//
// Token-storage note: auth.svelte.ts persists the bearer token under
// "jamsesh.token" (not "access_token"). The auth guard in App.svelte treats any
// non-null value for that key as authenticated. Tests that need an
// unauthenticated starting state must leave the key absent or null.
// ─────────────────────────────────────────────────────────────────────────────

// ─── 1. Unauthenticated visit to protected route → redirect to /login ────────
//
// Invariant: navigating to any protected route with no auth token in
// localStorage causes the App.svelte auth guard to redirect to /login and
// render the magic-link email input.
//
// Selector rationale: Login.svelte renders
//   <Input type="email" placeholder="you@example.com" />
// getByPlaceholder is semantic and stable — it breaks only on intentional copy
// changes.
test("unauthenticated visit to protected route redirects to login", async ({
  page,
}) => {
  // No token seeding — fresh context starts with empty localStorage.
  await page.goto("/orgs/some-org-id/sessions");

  // App.svelte's auth guard calls navigate('/login') when isAuthenticated is
  // false. Expect the login form to appear within 5 s.
  await expect(page).toHaveURL(/\/login/, { timeout: 5_000 });
  await expect(page.getByPlaceholder("you@example.com")).toBeVisible({
    timeout: 5_000,
  });
});

// ─── 2. Expired/invalid token → login redirect ───────────────────────────────
//
// Invariant: seeding localStorage with a non-null but invalid bearer token
// (so isAuthenticated is true) and then navigating to a protected route that
// requires a backend call does NOT redirect on its own — the guard only checks
// token presence. The session-list page will attempt a GET and surface an error.
//
// This test covers the simpler, guaranteed case: when no token is present at
// all, the guard redirects. The scenario where a stale token reaches the backend
// and the server returns 401 (triggering a sign-out) depends on a 401-handler in
// the API client that is not yet implemented; that test is skipped below.
test("no-token visit to protected route lands on login", async ({
  page,
  context,
}) => {
  // Explicitly clear the token key (belt-and-suspenders for isolation).
  await context.addInitScript(() => {
    localStorage.removeItem("jamsesh.token");
  });

  await page.goto("/orgs/another-org/sessions");

  await expect(page).toHaveURL(/\/login/, { timeout: 5_000 });
  await expect(page.getByPlaceholder("you@example.com")).toBeVisible({
    timeout: 5_000,
  });
});

// ─── 3. Magic-link request failure → "Something went wrong" error state ───────
//
// Invariant: when POST /api/auth/magic-link/request returns a non-2xx response,
// Login.svelte sets mode = 'magic-link-error' and renders
//   <h1>Something went wrong</h1>
// with the error message "Could not send magic link. Please try again."
//
// We intercept the network request so the test runs without a live portal.
//
// Selector rationale: the heading text is the primary signal; the error message
// is a secondary assertion. Both come directly from Login.svelte template
// (mode === 'magic-link-error' branch).
test("magic-link request failure shows error state", async ({ page }) => {
  // Intercept the magic-link request endpoint and return a 500.
  await page.route("/api/auth/magic-link/request", (route) => {
    void route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({ error: "internal server error" }),
    });
  });

  await page.goto("/login");

  const emailInput = page.getByPlaceholder("you@example.com");
  await expect(emailInput).toBeVisible({ timeout: 5_000 });
  await emailInput.fill("error-test@example.com");

  await page.getByRole("button", { name: "Send link" }).click();

  // Login.svelte renders <h1>Something went wrong</h1> in mode === 'magic-link-error'.
  await expect(
    page.getByRole("heading", { name: "Something went wrong" }),
  ).toBeVisible({ timeout: 5_000 });

  // Secondary assertion: the error message text.
  await expect(
    page.getByText("Could not send magic link. Please try again."),
  ).toBeVisible({ timeout: 5_000 });
});

// ─── 4. "Try again" from error state returns to the login form ────────────────
//
// Invariant: after the "Something went wrong" error state appears, clicking
// "Try again" returns to the 'choose' mode and re-renders the email input.
test("try-again from magic-link error returns to login form", async ({
  page,
}) => {
  await page.route("/api/auth/magic-link/request", (route) => {
    void route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "service unavailable" }),
    });
  });

  await page.goto("/login");

  const emailInput = page.getByPlaceholder("you@example.com");
  await expect(emailInput).toBeVisible({ timeout: 5_000 });
  await emailInput.fill("retry@example.com");
  await page.getByRole("button", { name: "Send link" }).click();

  await expect(
    page.getByRole("heading", { name: "Something went wrong" }),
  ).toBeVisible({ timeout: 5_000 });

  // Click the "Try again" ghost button rendered in mode === 'magic-link-error'.
  await page.getByRole("button", { name: "Try again" }).click();

  // Should return to the 'choose' mode — email input re-appears.
  await expect(page.getByPlaceholder("you@example.com")).toBeVisible({
    timeout: 3_000,
  });
});

// ─── 5. Malformed / unknown route renders "Page not found" ───────────────────
//
// Invariant: navigating to a URL that matches no route in router.svelte.ts
// renders NotFound.svelte with <h1>Page not found</h1>.
//
// Note on /auth/magic-link: the SPA's client-side router (router.svelte.ts)
// does not define a route for /auth/magic-link. The backend handles the
// redirect and the SPA never sees that path in practice. Visiting it directly
// in a test context hits the 'not-found' branch. The relevant error state for
// an expired magic-link token is instead surfaced via the backend 4xx response
// after the redirect — that path requires a live portal and is covered by the
// integration test suite.
//
// Selector rationale: NotFound.svelte renders an <h1>Page not found</h1>
// heading. Role + name is stable and meaningful.
test("unknown route renders page-not-found heading", async ({ page }) => {
  await page.goto("/auth/magic-link?token=garbage-not-a-real-token");

  // The SPA's not-found branch renders this heading via NotFound.svelte.
  await expect(
    page.getByRole("heading", { name: "Page not found" }),
  ).toBeVisible({ timeout: 5_000 });
});

// ─── 6. Missing org permission (session-list load error) ─────────────────────
//
// Invariant: navigating to an org's sessions page with a valid token but a
// 403 response from the backend causes SessionList.svelte to set
// loadError = 'Failed to load sessions.' and render that text on the page.
//
// We seed a plausible (but invalid) token so the auth guard does not redirect,
// and intercept the sessions API call to return 403.
//
// Selector rationale: SessionList.svelte renders the error as a <p> element
// with the exact text "Failed to load sessions." getByText is a stable semantic
// selector — it breaks only if the copy changes intentionally.
test("session-list shows load error on 403 response", async ({
  page,
  context,
}) => {
  // Seed a non-null token so the auth guard in App.svelte does not redirect.
  await context.addInitScript(() => {
    localStorage.setItem("jamsesh.token", "fake-but-present-token-for-403-test");
  });

  // Intercept the sessions API endpoint and return 403.
  await page.route(/\/api\/orgs\/[^/]+\/sessions$/, (route) => {
    void route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({ error: "forbidden" }),
    });
  });

  await page.goto("/orgs/restricted-org/sessions");

  // SessionList.svelte renders loadError as a paragraph when data load fails.
  await expect(page.getByText("Failed to load sessions.")).toBeVisible({
    timeout: 5_000,
  });
});

// ─── 7. Expired bearer token triggers 401 sign-out ───────────────────────────
//
// The API client's 401-interceptor (frontend/src/lib/api/client.ts) routes
// every 401 through auth.signOut(), which clears localStorage and navigates
// to /login. Seed a known-invalid token, navigate to a protected route, let
// the backend return 401, and assert a redirect to /login.
test(
  "stale bearer token on API call triggers 401 sign-out and login redirect",
  async ({ page, context }) => {
    await context.addInitScript(() => {
      localStorage.setItem("jamsesh.token", "expired-fake-token-123");
    });
    await page.goto("/orgs/some-org-id/sessions");
    await expect(page).toHaveURL(/\/login/, { timeout: 5_000 });
    await expect(page.getByPlaceholder("you@example.com")).toBeVisible({
      timeout: 5_000,
    });
  },
);

// ─── 8. Skipped: network-loss WebSocket reconnect indicator ──────────────────
//
// Pending: ws.svelte.ts does not implement reconnect logic or surface a
// "reconnecting" UI indicator. The socket's 'close' event handler currently
// only removes the entry from the sockets map (no retry, no UI state).
//
// Re-enable this test once:
//   - ws.svelte.ts implements exponential-backoff reconnect
//   - A stable "reconnecting" DOM element exists (e.g. role="status" with
//     text /reconnecting/i, or a data-testid="ws-reconnecting" banner)
test.skip(
  "network-loss state shows reconnecting indicator in session view",
  async ({ page, context }) => {
    // Pending: SPA WebSocket reconnect UX (see frontend/src/lib/ws.svelte.ts).
    // Re-enable once a stable reconnecting-state selector exists.
    await context.addInitScript(() => {
      localStorage.setItem("jamsesh.token", "valid-enough-token");
    });
    await page.goto("/orgs/test-org/sessions/test-session");
    // Simulate network failure — drop all WS connections.
    await page.route("**/ws/**", (route) => void route.abort("connectionrefused"));
    await expect(
      page.getByRole("status", { name: /reconnecting/i }),
    ).toBeVisible({ timeout: 10_000 });
  },
);
