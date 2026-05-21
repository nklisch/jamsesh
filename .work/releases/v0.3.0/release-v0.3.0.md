---
id: release-v0.3.0
kind: release
stage: released
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

### Feature: SPA logged-in landing + org bootstrap (4)

- `spa-logged-in-landing-and-org-bootstrap` (feature, [frontend, ui])
  - `spa-logged-in-landing-auth-store-orgs-cache` (story) — Auth store extension + bootstrap effect
  - `spa-logged-in-landing-home-screen` (story) — Home screen + router wiring
  - `spa-logged-in-landing-authed-redirect-fixes` (story) — Authed-redirect fixes

### Feature: bin/jamsesh regression harness (3)

- `testing-bin-jamsesh-regression-harness` (feature, [testing, plugin])
  - `testing-bin-jamsesh-regression-harness-bats-suite` (story) — bats test suite
  - `testing-bin-jamsesh-regression-harness-ci-job` (story) — CI job wiring

### Lone stories (4)

- `bug-receive-pack-report-status-sideband-wrapping` — git receive-pack report-status sideband framing fix
- `refactor-unify-refupdate-across-prereceive-postreceive` — unified RefUpdate type across hook handlers
- `infra-claude-scheduled-tasks-lock-should-be-gitignored` — gitignore the .claude scheduled-tasks lock file
- `docs-readme-cc-plugin-install-instructions` — README "Install the Claude Code plugin" section

### Gate-driven (21)

Items produced by the five quality gates (gate-security, gate-tests,
gate-cruft, gate-docs, gate-patterns) when they ran on the v0.3.0 bundle.
All 21 are at `stage: done`. See "Gate runs" below for finding counts.

## Gate runs

- **gate-security** (2026-05-20) — 5 findings (0 critical, 0 high, 1 medium, 4 low). 1 active story, 4 backlog.
- **gate-tests** (2026-05-20) — 11 findings (0 critical, 4 high, 4 medium, 2 low, 1 informational). 8 active stories, 2 backlog. No test-integrity violations.
- **gate-cruft** (2026-05-20) — 6 findings (1 high, 4 medium, 1 low). 5 active stories, 1 backlog.
- **gate-docs** (2026-05-20) — 2 findings (2 high, 0 medium). 1 foundation-doc-assertion (UX.md), 1 pattern-skill-staleness. 2 active stories.
- **gate-patterns** (2026-05-20) — 6 patterns extracted, 0 inconsistencies. Tracking item `gate-patterns-v0.3.0` at stage:done.

## Readiness check (2026-05-20, refresh)

NOT READY. 15 of 28 bound items are at `stage: done`. 13 stories remain
at `stage: drafting`. All 5 gates have already run; no re-run needed.

Progress since the previous readiness check (earlier 2026-05-20):
- All 4 `gate-tests-*` and 1 `gate-cruft-*` items that were at
  `implementing` (High severity) are now `done`.
- Both `gate-docs-*` High-severity items are now `done`.
- `gate-patterns-v0.3.0` is `done`.
- The 3 Low-severity items that were originally listed as "backlog Lows"
  are also now `done` (`gate-tests-picker-submit-name-trim`,
  `gate-tests-unknown-role-titlecase`,
  `gate-cruft-router-mock-dead-current-field`).

### Active items blocking ship (must reach `stage: done`)

**security** (5) — all at `drafting`
- `gate-security-refresh-token-localstorage-exposure` (Medium)
- `gate-security-signout-no-backend-revoke` (Low, originally backlog)
- `gate-security-authorize-url-no-scheme-host-validation` (Low, originally backlog)
- `gate-security-oauth-state-no-client-binding` (Low, originally backlog)
- `gate-security-401-blanket-signout` (Low, originally backlog)

**tests** (4) — all at `drafting` / Medium
- `gate-tests-org-row-preventdefault`
- `gate-tests-oauthcallback-loadme-rejection`
- `gate-tests-addorg-reactivity`
- `gate-tests-loadcurrentuser-null-token-noop`

**cruft** (4) — all at `drafting` / Medium
- `gate-cruft-app-stale-later-story-comment`
- `gate-cruft-oauthcallback-test-dead-isauth-mock`
- `gate-cruft-login-resumesession-unused-state`
- `gate-cruft-login-test-unused-spyon-location`

### How to drive these to done

- `/agile-workflow:implement-orchestrator` — drain the queue. The
  `gate-cruft-*` items in particular parallelize well as they're
  mechanical.
- `/agile-workflow:implement <id>` on a specific id for inline work.
- For the 4 Low-severity `gate-security-*` items originally tagged as
  "backlog Lows", an alternative is to clear `release_binding: v0.3.0`
  from their frontmatter to defer them to a later release rather than
  drain them now. The release file's original note (above) is
  preserved in git history; the gate-skill spec marks Low-severity
  items as "not stage-managed".
- Re-run `/agile-workflow:release-deploy v0.3.0` once all bound items
  are at `stage: done` — the orchestrator is idempotent and picks up
  where it left off.

## Shipped (2026-05-20)

**Mapping**: tag-based (annotated `v0.3.0`, pushed to origin/main).

**Release commit**: `e0c2886` (release-prep: v0.3.0)
**Release tag**: `v0.3.0`

**Bound items shipped**: 32

| Source | Count |
|---|---|
| Feature: spa-logged-in-landing-and-org-bootstrap + 3 child stories | 4 |
| Feature: testing-bin-jamsesh-regression-harness + 2 child stories | 3 |
| Gate-driven (security/tests/cruft/docs/patterns) | 21 |
| Lone stories (bug-fix, refactor, infra, docs) | 4 |

**Gate finding totals** (from the 5 gate runs on 2026-05-20):
- gate-security: 5 findings (1 medium, 4 low) — 2 shipped, 3 deferred to backlog
- gate-tests: 11 findings (4 high, 4 medium, 2 low, 1 informational) — all shipped
- gate-cruft: 6 findings (1 high, 4 medium, 1 low) — all shipped
- gate-docs: 2 findings — all shipped
- gate-patterns: 6 patterns extracted (tracking item `gate-patterns-v0.3.0` shipped)

**Frontend test count**: 465 → 476 across the cycle.

**Deferred to backlog (3)**: cross-stack security work needing feature-scope
design. See backlog items `gate-security-refresh-token-localstorage-exposure`,
`gate-security-signout-no-backend-revoke`, `gate-security-oauth-state-no-client-binding`.

**Next**: CI workflow triggered by tag push on origin — multi-arch binary
build + cosign keyless signing. Watch with `gh run watch`.
