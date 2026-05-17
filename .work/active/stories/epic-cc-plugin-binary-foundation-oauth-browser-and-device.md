---
id: epic-cc-plugin-binary-foundation-oauth-browser-and-device
kind: story
stage: implementing
tags: [plugin, security]
parent: epic-cc-plugin-binary-foundation
depends_on: [epic-cc-plugin-binary-foundation-router-state-mcp]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# CC Binary Foundation — OAuth Browser and Device-Code Flows

## Scope

Implement the `jamsesh auth` subcommand with both the default browser
local-listener flow and the `--device-code` alternative. PKCE (S256) +
state nonce on both. On success, write tokens to local state via the
`state` package.

## Units delivered

- `cmd/jamsesh/auth/auth.go` — the `Command()` exporter, root dispatcher between browser/device modes
- `cmd/jamsesh/auth/pkce.go` — PKCE pair generator + state nonce
- `cmd/jamsesh/auth/browser.go` — local-listener browser flow
- `cmd/jamsesh/auth/device.go` — RFC 8628 device-code flow
- Tests for each
- Add `github.com/pkg/browser@latest` to go.mod (or inline opener helper for darwin/linux/windows)

## Acceptance Criteria

- [ ] `jamsesh auth` (default) launches the browser flow:
      starts local listener on ephemeral port, opens browser to the
      portal's authorization URL, awaits callback, exchanges code,
      writes tokens
- [ ] `jamsesh auth --device-code` runs the RFC 8628 device-code flow
- [ ] PKCE: `code_verifier` is 32 random bytes base64url-encoded;
      `code_challenge` is `base64url(sha256(code_verifier))`;
      `code_challenge_method=S256`
- [ ] State nonce is verified on callback — mismatch returns an error
      without exchanging the code
- [ ] Device polling honors `interval` returned by the
      device-authorization response; `slow_down` adds 5s per error;
      `expired_token` exits cleanly
- [ ] Tokens written to `${CLAUDE_PLUGIN_DATA}/token` (access) and
      `${CLAUDE_PLUGIN_DATA}/refresh_token` at mode 0600
- [ ] Tests use a mock `openURL` injection (no real browser opens)
      and a mock portal HTTP server (`httptest.NewServer`) to exercise
      the full flow

## Notes

- The portal's `/api/auth/oauth/github/start`,
  `/api/auth/oauth/github/callback`, `/api/auth/device/authorize`, and
  `/api/auth/token` endpoints land in `epic-portal-foundation-auth-flows`.
  Until that ships, this binary's auth subcommand cannot complete a
  real flow against a real portal. **Tests use mock portals.**
- The auth subcommand registered in the previous story's main.go now
  has its real Action.
- For RFC 8628 polling: track elapsed time against `expires_in`; if
  exceeded, exit with a "device code expired" message.
