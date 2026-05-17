import { test, expect } from "@playwright/test";

// Invariant: opening the portal root in a fresh browser session redirects to
// /login and renders the magic-link email input within 5 seconds.
//
// Selector rationale: Login.svelte renders
//   <Input type="email" placeholder="you@example.com" />
// which compiles to <input type="email" placeholder="you@example.com">.
// getByPlaceholder is a semantic, stable handle — it breaks only if the
// placeholder text changes, which is a deliberate UI choice and a valid
// regression signal.
//
// If the portal root (/) is not yet routed to the login screen, visiting
// /login directly is an acceptable fallback and is noted below.
test("login screen renders the magic-link email input", async ({ page }) => {
  // The SPA auth guard redirects unauthenticated visitors at / to /login.
  await page.goto("/");

  // Wait for the email input field rendered by the magic-link form on Login.svelte.
  await expect(page.getByPlaceholder("you@example.com")).toBeVisible({
    timeout: 5_000,
  });
});

test("magic-link form accepts email input", async ({ page }) => {
  await page.goto("/login");

  const emailInput = page.getByPlaceholder("you@example.com");
  await expect(emailInput).toBeVisible({ timeout: 5_000 });

  // Confirm the field is interactive: typing works without throwing.
  await emailInput.fill("smoke@example.com");
  await expect(emailInput).toHaveValue("smoke@example.com");
});
