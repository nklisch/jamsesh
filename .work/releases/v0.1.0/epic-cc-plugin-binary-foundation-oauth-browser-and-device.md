---
id: epic-cc-plugin-binary-foundation-oauth-browser-and-device
kind: story
stage: done
tags: [plugin, security]
parent: epic-cc-plugin-binary-foundation
depends_on: [epic-cc-plugin-binary-foundation-router-state-mcp]
release_binding: v0.1.0
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

- [x] `jamsesh auth` (default) launches the browser flow:
      starts local listener on ephemeral port, opens browser to the
      portal's authorization URL, awaits callback, exchanges code,
      writes tokens
- [x] `jamsesh auth --device-code` runs the RFC 8628 device-code flow
- [x] PKCE: `code_verifier` is 32 random bytes base64url-encoded;
      `code_challenge` is `base64url(sha256(code_verifier))`;
      `code_challenge_method=S256`
- [x] State nonce is verified on callback — mismatch returns an error
      without exchanging the code
- [x] Device polling honors `interval` returned by the
      device-authorization response; `slow_down` adds 5s per error;
      `expired_token` exits cleanly
- [x] Tokens written to `${CLAUDE_PLUGIN_DATA}/token` (access) and
      `${CLAUDE_PLUGIN_DATA}/refresh_token` at mode 0600
- [x] Tests use a mock `openURL` injection (no real browser opens)
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

## Implementation notes

### Files delivered

- `cmd/jamsesh/auth/pkce.go` — `GeneratePKCE()` / `GenerateState()`. Verifier
  is 32 crypto-random bytes, base64url (no padding). Challenge is
  `base64url(sha256(verifier))`. State is 16 random bytes base64url.
- `cmd/jamsesh/auth/browser.go` — `browserFlow()`. Starts an ephemeral
  `net.Listen("tcp", "127.0.0.1:0")` server, builds the authorization URL
  with all required PKCE + state params, calls `openURL`, waits for the
  callback on `/cb`, validates state, exchanges the code at
  `/api/auth/code` (placeholder — maps to the future portal endpoint), and
  writes tokens via `state.WriteToken` / `state.WriteRefreshToken`.
- `cmd/jamsesh/auth/device.go` — `deviceFlow()`. POSTs to
  `/api/auth/device/authorize`, prints user instructions, polls
  `/api/auth/token` honoring `interval`; `slow_down` adds 5 s;
  `expired_token` / `access_denied` return errors; success writes tokens.
  The `sleep` parameter is injectable for deterministic testing.
- `cmd/jamsesh/auth/auth.go` — `Command()` accepts `...Option` (functional
  options). `WithOpenURL(fn)` replaces the default platform opener for
  tests. The `--device-code` flag dispatches to `deviceFlow`; default
  dispatches to `browserFlow`.

### Browser opener choice

We inline a tiny cross-platform opener (`defaultOpenURL`) rather than
adding `github.com/pkg/browser` as a dependency. The logic is identical
to what that package does (xdg-open / open / rundll32). If the command
fails, the URL is printed to stderr so the user can open it manually —
graceful degradation, no hard failure.

### Portal endpoint status

The token-exchange endpoint called by the browser flow (`/api/auth/code`)
and the device-flow endpoints (`/api/auth/device/authorize`,
`/api/auth/token`) do not exist on the portal yet — they land in
`epic-portal-foundation-auth-flows`. Running `jamsesh auth` against a real
portal today will fail with 404 at the exchange step. All tests exercise
the binary's HTTP call shapes against `httptest.NewServer` mock portals,
which are green.

### Test coverage

- `pkce_test.go` — verifier length (32 bytes), challenge derivation,
  uniqueness across calls; state nonce length and uniqueness.
- `browser_test.go` — happy-path full flow (mock portal + injected opener);
  state-mismatch rejection (400, error propagation); auth URL parameter
  shape; token-exchange request shape.
- `device_test.go` — happy-path (immediate success); authorization_pending
  (2 pending → success, 3 polls); slow_down interval growth (2s → 7s);
  expired_token clean exit; access_denied error; context cancellation;
  device-authorization request shape; token poll request shape.

### go.mod

No new direct dependencies added. `golang.org/x/sync` and
`github.com/urfave/cli/v3` were already present from the router-state-mcp
story.

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: PKCE + state nonce correct. RFC 8628 polling honors interval and slow_down. Injected openURL + httptest portals enable hermetic testing. Inlining the browser opener (no github.com/pkg/browser dep) is a reasonable trade.
