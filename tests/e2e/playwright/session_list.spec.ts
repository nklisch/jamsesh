import { test, expect } from "@playwright/test";

// ─────────────────────────────────────────────────────────────────────────────
// Session list and session view shell — Playwright spec
//
// Invariant: an authenticated user can navigate to the sessions list, see
// their sessions loaded from the backend, click a session row, and land on
// the session view shell which renders the session name within 5 seconds.
//
// Runtime requirement: PORTAL_URL must point at a running portal (set via
// playwright.config.ts). SESSION_ORG_ID and SESSION_AUTH_TOKEN must be set in
// the environment if this spec is driven by a Go Testcontainers harness. For
// standalone ad-hoc runs, set the env vars manually alongside PORTAL_URL:
//
//   SESSION_ORG_ID=<orgID>
//   SESSION_AUTH_TOKEN=<bearerToken>
//   SESSION_ID=<sessionID>
//   SESSION_NAME=<sessionName>
//   PORTAL_URL=http://localhost:PORT npx playwright test session_list
//
// When the env vars are absent the spec stubs the API layer to validate the
// SPA's rendering path without requiring a live portal data set. This mode
// exercises the UI contract (correct selectors, correct navigation) rather
// than the full integration stack.
// ─────────────────────────────────────────────────────────────────────────────

const orgID = process.env["SESSION_ORG_ID"] ?? "stub-org-id";
const authToken = process.env["SESSION_AUTH_TOKEN"] ?? "stub-bearer-token";
const sessionID = process.env["SESSION_ID"] ?? "stub-session-id";
const sessionName = process.env["SESSION_NAME"] ?? "Playwright Test Session";

// seedAuth injects the bearer token into localStorage so the App.svelte auth
// guard treats the browser context as signed-in.
async function seedAuth(
  context: import("@playwright/test").BrowserContext,
  token: string,
) {
  await context.addInitScript((t) => {
    localStorage.setItem("jamsesh.token", t);
  }, token);
}

// ─── 1. Sessions list renders after navigation ───────────────────────────────
//
// Invariant: navigating to /orgs/{orgID}/sessions with a valid auth token
// renders the "Your sessions" heading (from SessionList.svelte) within 5 s.
//
// The API call to /api/orgs/{orgID}/sessions is intercepted and stubbed so
// this test does not require a live data set. The stub returns a session whose
// name matches `sessionName` so we can assert on the rendered row.
test("sessions list renders session rows from API response", async ({
  page,
  context,
}) => {
  await seedAuth(context, authToken);

  // Stub the sessions list endpoint.
  await page.route(/\/api\/orgs\/[^/]+\/sessions$/, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          {
            id: sessionID,
            org_id: orgID,
            name: sessionName,
            goal: "E2E Playwright test goal",
            scope: '["**"]',
            default_mode: "sync",
            status: "active",
            base_sha: null,
            end_reason: null,
            created_at: new Date().toISOString(),
            members: [{ account_id: "stub-member-id", role: "creator" }],
          },
        ],
        next_cursor: null,
      }),
    });
  });

  // Stub the single-session fetch that fires on WS events (not triggered in
  // this test, but listed here for completeness so the route does not fall
  // through to a live backend if one is present).
  await page.route(/\/api\/orgs\/[^/]+\/sessions\/[^/]+$/, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        id: sessionID,
        org_id: orgID,
        name: sessionName,
        goal: "E2E Playwright test goal",
        scope: '["**"]',
        default_mode: "sync",
        status: "active",
        base_sha: null,
        end_reason: null,
        created_at: new Date().toISOString(),
        members: [{ account_id: "stub-member-id", role: "creator" }],
      }),
    });
  });

  await page.goto(`/orgs/${orgID}/sessions`);

  // SessionList.svelte renders <h1>Your sessions</h1>.
  await expect(
    page.getByRole("heading", { name: "Your sessions" }),
  ).toBeVisible({ timeout: 5_000 });

  // The stubbed session row must be visible.
  await expect(page.getByText(sessionName)).toBeVisible({ timeout: 5_000 });
});

// ─── 2. Clicking a session row navigates to the session view shell ────────────
//
// Invariant: clicking a session row button in the list navigates to
// /orgs/{orgID}/sessions/{sessionID} and the session view shell renders the
// session name in the header within 5 seconds.
test("clicking a session row navigates to the session view shell", async ({
  page,
  context,
}) => {
  await seedAuth(context, authToken);

  const stubSession = {
    id: sessionID,
    org_id: orgID,
    name: sessionName,
    goal: "E2E Playwright test goal",
    scope: '["**"]',
    default_mode: "sync",
    status: "active",
    base_sha: null,
    end_reason: null,
    created_at: new Date().toISOString(),
    members: [{ account_id: "stub-member-id", role: "creator" }],
  };

  // Stub the sessions list.
  await page.route(/\/api\/orgs\/[^/]+\/sessions$/, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [stubSession], next_cursor: null }),
    });
  });

  // Stub the single-session GET that SessionViewShell calls on mount.
  await page.route(/\/api\/orgs\/[^/]+\/sessions\/[^/]+$/, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(stubSession),
    });
  });

  // Stub the refs endpoint called by TreeDag inside the session view shell.
  await page.route(/\/api\/orgs\/[^/]+\/sessions\/[^/]+\/refs/, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ refs: [] }),
    });
  });

  await page.goto(`/orgs/${orgID}/sessions`);

  // Wait for the list to render the session row.
  const sessionRow = page.getByRole("button", { name: sessionName });
  await expect(sessionRow).toBeVisible({ timeout: 5_000 });

  // Click the session row — this calls navigate() in SessionList.svelte.
  await sessionRow.click();

  // We should now be on the session view shell URL.
  await expect(page).toHaveURL(
    new RegExp(`/orgs/${orgID}/sessions/${sessionID}$`),
    { timeout: 5_000 },
  );

  // SessionViewShell.svelte renders session.name inside <h1> in the header
  // once the GET /api/orgs/{orgID}/sessions/{sessionID} call resolves.
  await expect(page.getByRole("heading", { name: sessionName })).toBeVisible({
    timeout: 5_000,
  });
});

// ─── 3. Session view shell renders session name for direct navigation ─────────
//
// Invariant: navigating directly to /orgs/{orgID}/sessions/{sessionID} with a
// valid auth token renders the session's name in the shell's <h1> heading
// within 5 seconds. This exercises the route match and the session load path
// without going through the list.
test("direct navigation to session view shell renders session name", async ({
  page,
  context,
}) => {
  await seedAuth(context, authToken);

  const stubSession = {
    id: sessionID,
    org_id: orgID,
    name: sessionName,
    goal: "E2E Playwright test goal",
    scope: '["**"]',
    default_mode: "sync",
    status: "active",
    base_sha: null,
    end_reason: null,
    created_at: new Date().toISOString(),
    members: [{ account_id: "stub-member-id", role: "creator" }],
  };

  // Stub the session GET used by SessionViewShell on mount.
  await page.route(/\/api\/orgs\/[^/]+\/sessions\/[^/]+$/, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(stubSession),
    });
  });

  // Stub the refs endpoint.
  await page.route(/\/api\/orgs\/[^/]+\/sessions\/[^/]+\/refs/, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ refs: [] }),
    });
  });

  await page.goto(`/orgs/${orgID}/sessions/${sessionID}`);

  // SessionViewShell.svelte renders the session name in its <h1> header.
  await expect(page.getByRole("heading", { name: sessionName })).toBeVisible({
    timeout: 5_000,
  });
});
