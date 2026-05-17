import { test, expect } from "@playwright/test";

// Invariant: a visitor who types their email into the magic-link form and
// clicks "Send link" sees a "Check your inbox" confirmation state in response.
// The transition happens client-side on a 200 response from
// POST /api/auth/magic-link/request. This spec verifies the UI behaviour from
// form entry through to the confirmed state.
//
// Runtime requirement: PORTAL_URL must point at a running portal with MailHog
// wired as the SMTP backend (set in playwright.config.ts via the PORTAL_URL
// env var). In CI this is satisfied by the Go-driven Testcontainers stack; for
// local ad-hoc runs, start the portal stack first.

test("magic-link form transitions to check-your-inbox state after submission", async ({
  page,
}) => {
  await page.goto("/login");

  // Fill in the magic-link email input.
  const emailInput = page.getByPlaceholder("you@example.com");
  await expect(emailInput).toBeVisible({ timeout: 5_000 });
  await emailInput.fill("playwright-test@example.com");

  // Click the "Send link" button to submit the form.
  await page.getByRole("button", { name: "Send link" }).click();

  // Login.svelte sets mode = 'magic-link-sent' on a 200 response, which
  // renders an <h1>Check your inbox</h1> heading.
  await expect(page.getByRole("heading", { name: "Check your inbox" })).toBeVisible({
    timeout: 5_000,
  });
});

test("check-your-inbox state shows the submitted email address", async ({
  page,
}) => {
  const testEmail = "confirmation-display@example.com";

  await page.goto("/login");

  const emailInput = page.getByPlaceholder("you@example.com");
  await expect(emailInput).toBeVisible({ timeout: 5_000 });
  await emailInput.fill(testEmail);
  await page.getByRole("button", { name: "Send link" }).click();

  // The confirmation state renders the email address the link was sent to.
  await expect(page.getByText(testEmail)).toBeVisible({ timeout: 5_000 });
});

test("check-your-inbox state has a link to retry with a different email", async ({
  page,
}) => {
  await page.goto("/login");

  const emailInput = page.getByPlaceholder("you@example.com");
  await expect(emailInput).toBeVisible({ timeout: 5_000 });
  await emailInput.fill("retry-test@example.com");
  await page.getByRole("button", { name: "Send link" }).click();

  await expect(page.getByRole("heading", { name: "Check your inbox" })).toBeVisible({
    timeout: 5_000,
  });

  // Clicking "Try a different email" returns to the choose form.
  await page.getByRole("button", { name: "Try a different email" }).click();
  await expect(page.getByPlaceholder("you@example.com")).toBeVisible({
    timeout: 3_000,
  });
});
