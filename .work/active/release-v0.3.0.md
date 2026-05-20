---
id: release-v0.3.0
kind: release
stage: quality-gate
tags: []
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
---

# Release v0.3.0

## Bound items

- `spa-logged-in-landing-and-org-bootstrap` (feature, [frontend, ui]) — SPA logged-in landing and org bootstrap
  - `spa-logged-in-landing-auth-store-orgs-cache` (story) — Auth store extension + bootstrap effect
  - `spa-logged-in-landing-home-screen` (story) — Home screen + router wiring
  - `spa-logged-in-landing-authed-redirect-fixes` (story) — Authed-redirect fixes

## Gate runs

- **gate-security** (2026-05-20) — 5 findings (0 critical, 0 high, 1 medium, 4 low). 1 active story, 4 backlog.
- **gate-tests** (2026-05-20) — 11 findings (0 critical, 4 high, 4 medium, 2 low, 1 informational). 8 active stories, 2 backlog. No test-integrity violations.
- **gate-cruft** (2026-05-20) — 6 findings (1 high, 4 medium, 1 low). 5 active stories, 1 backlog.
- **gate-docs** (2026-05-20) — 2 findings (2 high, 0 medium). 1 foundation-doc-assertion (UX.md), 1 pattern-skill-staleness. 2 active stories.
- **gate-patterns** (2026-05-20) — 6 patterns extracted, 0 inconsistencies. Tracking item `gate-patterns-v0.3.0` at stage:done.

## Readiness check (2026-05-20)

NOT READY. All 5 gates have run. 16 active items + 7 backlog items remain
with `release_binding: v0.3.0`.

### Active items blocking ship (must reach `stage: done`)

**security** (1)
- `gate-security-refresh-token-localstorage-exposure` (drafting / Medium)

**tests** (8)
- `gate-tests-router-root-route-home` (implementing / High)
- `gate-tests-signout-resets-loadingme` (implementing / High)
- `gate-tests-app-authed-on-login-redirect` (implementing / High)
- `gate-tests-app-bootstrap-effect` (implementing / High)
- `gate-tests-org-row-preventdefault` (drafting / Medium)
- `gate-tests-oauthcallback-loadme-rejection` (drafting / Medium)
- `gate-tests-addorg-reactivity` (drafting / Medium)
- `gate-tests-loadcurrentuser-null-token-noop` (drafting / Medium)

**cruft** (5)
- `gate-cruft-home-test-redundant-setorgs` (implementing / High)
- `gate-cruft-app-stale-later-story-comment` (drafting / Medium)
- `gate-cruft-oauthcallback-test-dead-isauth-mock` (drafting / Medium)
- `gate-cruft-login-resumesession-unused-state` (drafting / Medium)
- `gate-cruft-login-test-unused-spyon-location` (drafting / Medium)

**docs** (2)
- `gate-docs-ux-md-home-landing-surface` (implementing / High)
- `gate-docs-openapi-fetch-middleware-pattern-citation` (implementing / High)

### Backlog items bound to v0.3.0 (deferred Lows — see note)

- `gate-security-signout-no-backend-revoke`
- `gate-security-authorize-url-no-scheme-host-validation`
- `gate-security-oauth-state-no-client-binding`
- `gate-security-401-blanket-signout`
- `gate-tests-picker-submit-name-trim`
- `gate-tests-unknown-role-titlecase`
- `gate-cruft-router-mock-dead-current-field`

These are Low-severity findings filed to backlog by their respective
gate skills (per spec). They carry `release_binding: v0.3.0` to record
where they surfaced, but the gate skill specs flag them as "not
stage-managed". Treat as informational: either drain them inline or
clear `release_binding` to defer to a later release.

### How to drive these to done

- `/agile-workflow:implement-orchestrator` — drain the queue, the
  `gate-cruft-*` items in particular parallelize well as they're
  mechanical.
- `/agile-workflow:implement` on a specific id for inline work.
- Re-run `/agile-workflow:release-deploy v0.3.0` once all bound items
  are at `stage: done` — the orchestrator is idempotent and picks up
  where it left off.
