import { test, expect } from "@playwright/test";

// Invariant: when the portal is booted with JAMSESH_LANDING_VARIANT=project,
// an anonymous visitor at / sees the ProjectLanding screen rather than being
// redirected to /login.  This is the whole-stack smoke test for the
// landing-variant capability:
//
//   operator sets JAMSESH_LANDING_VARIANT=project
//   → portal serves it in GET /api/portal/info
//   → SPA fetches it on boot
//   → App.svelte routes anonymous / to <ProjectLanding/>
//   → DOM has the hero h1 and the sign-in link
//
// Runtime requirement: PORTAL_URL must point at a portal that was started
// with JAMSESH_LANDING_VARIANT=project.  Without that env var the portal
// returns landingVariant="default" and App.svelte redirects / → /login,
// which would cause both tests below to fail.  Set the env var before
// starting the portal container:
//
//   docker run … -e JAMSESH_LANDING_VARIANT=project jamsesh/portal:e2e
//
// In CI the Go-orchestrated fixture layer is responsible for setting
// JAMSESH_LANDING_VARIANT=project via Options.ExtraEnv before handing
// PORTAL_URL to Playwright (see tests/e2e/fixtures/portal/portal.go).
// That wiring lands in the ci-workflow story; for now it is a manual step.
//
// Selector rationale:
//   heading level 1 — ProjectLanding.svelte hero renders
//     <h1>Your team on a call.<br>Your agents <span>in the loop.</span></h1>
//   The regex /your team on a call/i matches the visible text content.
//
//   "Sign in" link — the topbar end-section renders
//     <a href="/login" class="signin">Sign in →</a>
//   getByRole('link', { name: /sign in/i }) matches via accessible name.
//   The arrow character (→) is part of the text but the regex omits it so
//   the assertion stays readable and decoupled from cosmetic copy changes.

test("anonymous root with landing_variant=project shows ProjectLanding hero heading", async ({
  page,
}) => {
  // PORTAL_URL must point at a portal started with JAMSESH_LANDING_VARIANT=project.
  await page.goto("/");

  // ProjectLanding hero h1: "Your team on a call. Your agents in the loop."
  await expect(
    page.getByRole("heading", { name: /your team on a call/i, level: 1 }),
  ).toBeVisible({ timeout: 5_000 });
});

test("anonymous root with landing_variant=project shows sign-in link", async ({
  page,
}) => {
  await page.goto("/");

  // Topbar renders <a href="/login" class="signin">Sign in →</a>
  await expect(
    page.getByRole("link", { name: /sign in/i }),
  ).toBeVisible({ timeout: 5_000 });
});
