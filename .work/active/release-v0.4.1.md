---
id: release-v0.4.1
kind: release
stage: quality-gate
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

<populated in Phase 4>
