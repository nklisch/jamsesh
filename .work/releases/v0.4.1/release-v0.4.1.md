---
id: release-v0.4.1
kind: release
stage: released
tags: []
parent: null
depends_on: []
release_binding: v0.4.1
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# Release v0.4.1

Patch release shipping the portal visitor-entry SPA landing surface plus
small follow-ups since v0.4.0: a CLI playground share-url fix, a
status-nickname empty-playground fix, a data-dir env rename, and a
no-op architectural-note triage on the parseInviteEmails dedupe.

## Bound items

### Feature: portal visitor-entry pages (3)

- `feature-portal-visitor-entry-pages` — `ui, portal`
  - `story-portal-visitor-entry-pages-info-endpoint` — `portal, infra`
  - `story-portal-visitor-entry-pages-spa-landing` — `ui, portal`

### Stories — bug fixes (2)

- `story-fix-cli-playground-share-url` — `bug, cli, plugin`
- `story-status-nickname-empty-playground` — `bug, cli, plugin`

### Stories — refactor / docs (1)

- `story-data-dir-env-rename` — `refactor, plugin, documentation`

### Stories — triage no-op (1)

- `gate-tests-parseinviteemails-dedupe-location-architectural-note` — `testing, plugin`

## Gate runs

- **gate-security** (2026-05-25) — 5 findings filed (0 critical, 0 high, 1 medium, 4 low). All unbound from v0.4.1 (deferred per user direction — patch-release pragmatic).
- **gate-tests** (2026-05-25) — 9 findings filed (2 critical, 2 high, 3 medium, 2 low). 1 Critical drained inline (`gate-tests-cli-jam-playground-flag-e2e-extractor-stale-url` — fixed stale share-URL extractor). Remaining 8 unbound from v0.4.1 (deferred per user direction).
- **gate-cruft**, **gate-docs**, **gate-patterns** — skipped per user direction (patch-release scope).

## Drained inline this cycle

- `gate-tests-cli-jam-playground-flag-e2e-extractor-stale-url` (Critical) —
  `tests/e2e/golden/cli_jam_playground_flag_test.go` `extractPlaygroundSessionID`
  re-anchored on the new `/playground/s/<id>/join` URL shape that
  `story-fix-cli-playground-share-url` introduced.

## Deferred to future releases (filed, unbound)

Gate findings tracked as items but not blocking this patch. See
`.work/active/stories/gate-{security,tests}-*.md` and
`.work/backlog/gate-{security,tests}-*.md`.

## Shipped (2026-05-25)

**Mapping**: tag-based (annotated `v0.4.1`, pushed to `origin/main`).

**Release commit**: `6a97c5a` (release-prep: v0.4.1)
**Release tag**: `v0.4.1`

**Bound items shipped**: 8
- 1 feature + 5 stories (the original v0.4.1 bundle)
- 1 inline-drained Critical gate-tests story
- 1 no-op architectural-note story (from prior cycle, kept in bundle)

**Gate finding totals**: 14 filed (gate-security: 5; gate-tests: 9). 1
drained inline; 13 unbound and deferred to future release(s).
cruft/docs/patterns gates skipped per user direction.
