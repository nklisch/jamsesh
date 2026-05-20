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
