---
id: epic-cc-plugin-binary-foundation-portal-client-and-refresh
kind: story
tags: [plugin]
stage: implementing
parent: epic-cc-plugin-binary-foundation
depends_on: [epic-cc-plugin-binary-foundation-router-state-mcp]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# CC Binary Foundation — Portal API Client and Token Refresh

## Scope

A small typed HTTP client for the subset of portal endpoints the
plugin binary calls, plus a single-flight token-refresh helper that
runs silently in the background on 401s.

## Units delivered

- `cmd/jamsesh/portalclient/client.go` — `Client` struct with
  `Do(ctx, req)`, `GetJSON[T]`, `PostJSON[T]`; auto-refresh-on-401
- `cmd/jamsesh/portalclient/refresh.go` — `Refresher` with
  `singleflight.Group`-backed `Refresh(ctx) error`
- Tests for both
- Add `golang.org/x/sync@latest` to go.mod

## Acceptance Criteria

- [ ] `Client.Do` attaches `Authorization: Bearer <token>` from local
      state on every request
- [ ] 401 response triggers `c.Refresh()` then a single retry;
      second 401 returns the error
- [ ] Concurrent `Refresher.Refresh()` calls collapse to one HTTP
      POST (verified by a test spawning N goroutines and counting
      hits against an `httptest.NewServer`)
- [ ] Refresh writes the new access + refresh tokens to local state
      atomically (no partial-write race)
- [ ] On refresh failure (network error, refresh-token revoked), the
      error propagates back to `Do` and the second retry is skipped

## Notes

- The portal's `/api/auth/refresh` endpoint landed with
  `epic-portal-foundation-tokens-refresh-and-revoke-endpoints` — the
  contract is `POST {refresh_token: string}` → 200 with `TokenPair`.
  Use the contract directly; don't import the generated types from
  the portal's `internal/api/openapi` package (that's portal-internal).
- The `Refresher` is injected into the `Client` via a field; the
  binary's main composes them at startup.
- This story has NO depends_on on `oauth-browser-and-device` — it can
  ship before users can complete an OAuth flow, since the client is
  exercised in tests against a mock portal.
