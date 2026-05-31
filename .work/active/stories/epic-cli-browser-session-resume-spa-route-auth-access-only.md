---
id: epic-cli-browser-session-resume-spa-route-auth-access-only
kind: story
stage: implementing
tags: [ui]
parent: epic-cli-browser-session-resume-spa-route
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# `auth.setAccessOnly` — durable access-only adoption

Implements **Unit 1** of `epic-cli-browser-session-resume-spa-route`. See feature body.

## Scope

- `frontend/src/lib/auth.svelte.ts`: add `setAccessOnly(access: string): void` to
  the `auth` wrapper-object rune store. Sets `_token` (+ persists to
  `jamsesh.token`); CLEARS `_refresh` and removes the `jamsesh.refresh`
  localStorage key; clears any cached current-user/orgs and `_loadingMe` so the
  next `/me` runs fresh as the adopted account. (The resume exchange returns an
  access-only durable bearer — no refresh — so we must not leave a stale refresh
  or cached user behind.)
- `frontend/src/lib/auth.test.ts`: tests.

## Acceptance criteria

- [ ] After `setAccessOnly(x)`: `auth.token === x` (persisted to `jamsesh.token`);
      `auth.refresh === null`; `jamsesh.refresh` removed from localStorage.
- [ ] Cached current-user/orgs + `_loadingMe` cleared (next `/me` fresh).
- [ ] Follows the `wrapper-object-rune-store` pattern (no raw `$state` export).
- [ ] `npm run -C frontend test` (or the project's vitest cmd) passes; typecheck clean.

## Notes

If the SPA build/test writes to `/tmp` (tmpfs full), point TMPDIR at /home.
