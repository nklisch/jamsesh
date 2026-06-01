---
id: release-v0.5.1
kind: release
stage: released
tags: []
parent: null
depends_on: []
release_binding: v0.5.1
gate_origin: null
created: 2026-06-01
updated: 2026-06-01
---

# Release v0.5.1

## Bound items

- feature-playground-anon-session-access (feature)
- story-playground-anon-access-file-tree-403 (story)
- story-playground-anon-access-refresh-bounce (story)
- story-playground-anon-access-ws-live-updates (story)

## Gate runs

- Full release gates skipped by maintainer request for this patch release.
- Lightweight verification only:
  - `go test ./internal/portal/sessions`
  - `npm test -- --run src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/components/ArtifactPane.test.ts src/App.test.ts src/lib/screens/SessionViewShell.test.ts src/lib/ws.test.ts`
  - `npm run check` (0 errors, 1 pre-existing Svelte warning in `ModeSwitchDialog.svelte`)

## Release summary

- Shipped: 2026-06-01
- Mapping: tag-based
- Version commit: pending (`release-prep: v0.5.1`)
- Tag: pending `v0.5.1`
- Items shipped: 4 work items plus this release record
- Gates run: skipped by maintainer request
- Gate outcomes: not applicable for this patch release
