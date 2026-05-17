import { defineConfig, devices } from "@playwright/test";

const portalURL = process.env["PORTAL_URL"] ?? "http://localhost:8443";

export default defineConfig({
  testDir: "./",
  fullyParallel: true,
  forbidOnly: !!process.env["CI"],
  retries: process.env["CI"] ? 1 : 0,
  workers: process.env["CI"] ? 1 : undefined,
  reporter: [
    [process.env["CI"] ? "github" : "list"],
    ["html", { outputFolder: "playwright-report", open: "never" }],
  ],
  use: {
    baseURL: portalURL,
    trace: "retain-on-failure",
    headless: true,
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
  ],
});
