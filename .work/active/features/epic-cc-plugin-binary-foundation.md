---
id: epic-cc-plugin-binary-foundation
kind: feature
stage: drafting
tags: [plugin, security]
parent: epic-cc-plugin
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# CC Plugin — Binary Foundation

## Brief

The `jamsesh` Go binary's foundation layer: subcommand router, JSON-in/
JSON-out hook IO scaffold, local state read/write helpers, OAuth client
flows (both local-listener and `--device-code`), token refresh, and the
`mcp-headers` subcommand CC's MCP client consumes at connection time.

Every subsequent binary subcommand (session-commands, hooks) sits on
top of this foundation.

**Subcommands delivered by this feature**:

- `jamsesh auth` — runs the OAuth client flow against the configured
  portal URL. Default mode: open a browser, run a one-shot HTTP listener
  on an ephemeral port for the OAuth callback, exchange the code for
  access + refresh tokens, write to local state, shut down listener.
  `--device-code` mode: portal returns a short user code + verification
  URL; user enters the code on any browser-capable device; binary polls
  the portal until confirmation.
- `jamsesh mcp-headers` — invoked by CC's MCP client via headersHelper
  at connection open and on token-expired retry. Reads
  `${CLAUDE_PLUGIN_DATA}/token` synchronously, outputs
  `{"Authorization": "Bearer <token>"}` as JSON on stdout. Never
  refreshes — refresh is asynchronous and triggered by 401s elsewhere.

**Internal scaffolding** (exposed as Go packages other features consume):

- **Subcommand router**: parses argv, dispatches to registered
  subcommand handlers; standard `--help`, `--json` flags.
- **Hook IO scaffold**: reads JSON from stdin into a typed struct,
  writes JSON to stdout. Each hook subcommand registers an input type
  and an output type; the scaffold handles parse / dispatch / encode /
  error formatting consistently.
- **Local state package**: read/write helpers for
  `${CLAUDE_PLUGIN_DATA}/`. Single-value files (`token`, `refresh_token`,
  `portal_url`) — plain-text, one line, file mode 0600 for credential
  files. Per-session structured state under `sessions/<session-id>/`
  as small JSON files (`ref`, `instance_id`, `last_seen_seq`, `refs/<peer>`).
  All reads return typed structs; all writes are atomic (write to temp +
  rename).
- **Token refresh helper**: invoked by any portal-API caller on 401.
  Posts to `/api/auth/refresh` with the refresh token, writes new
  tokens to local state, returns. Single-flight (concurrent 401s
  collapse to one refresh).
- **Portal API client**: thin REST client (typed via the spec-first
  generated contracts where applicable — clients consume
  `openapi-typescript` for TS, the Go binary may directly call REST
  endpoints with hand-rolled clients OR consume the same
  oapi-codegen-generated Go client; design pass decides). Includes
  automatic 401 → refresh → retry logic.

**OAuth security** (locked at epic-design):

- PKCE (S256) + state parameter on both flows. Cheap, universal best
  practice, forward-compatible with provider changes.
- Tokens are never logged. Token files are mode 0600.
- Refresh tokens rotate on every refresh.

Does NOT include any hook implementations (`hooks` feature). Does NOT
include `join`, `status`, `fork`, `mode` (`session-commands`). Does NOT
include the plugin manifest or skills (`packaging`).

## Epic context

- Parent epic: `epic-cc-plugin`
- Position in epic: foundation. Session-commands and hooks both
  consume the local state, portal API client, and JSON IO scaffold.

## Foundation references

- `docs/SPEC.md` — Local client, Auth model (OAuth flow, magic-link —
  this feature implements the client side)
- `docs/ARCHITECTURE.md` — The `jamsesh` binary, Local state layout
- `docs/SECURITY.md` — User authentication (OAuth + magic-link flows
  this binary participates in), Token lifetime and renewal,
  Self-host security posture
- `docs/PROTOCOL.md` — REST API > Auth section

## Inherited epic design decisions

- **OAuth flows**: local-listener default + `--device-code` flag.
- **OAuth security primitives**: PKCE (S256) + state parameter.
- **Local state format**: plain text per-file for single values, JSON
  for structured state. Token files mode 0600. Atomic writes.
- **headersHelper shape**: synchronous read + JSON output. Async
  refresh elsewhere.
- **Token refresh on 401**: handled silently in the background; if
  refresh fails, the next hook surfaces "session token expired" via
  `additionalContext`.

## Decomposition risks

- Device-code OAuth flow has subtle timing concerns: polling cadence,
  code expiry, user-experience messaging during the wait. Mitigation:
  design pass references RFC 8628 (OAuth Device Authorization Grant)
  and locks the polling cadence.
- `headersHelper` timing: if CC caches headers per connection and the
  token refreshes mid-session, MCP calls keep going with stale
  Authorization until reconnect. Document this assumption; CC's MCP
  client spec is the source of truth on 401-retry behavior.
- Single-flight token refresh is the right primitive but easy to
  miscode. Design pass produces a small concurrency test (multiple
  goroutines triggering refresh simultaneously → one refresh).

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
