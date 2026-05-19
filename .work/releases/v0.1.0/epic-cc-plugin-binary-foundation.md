---
id: epic-cc-plugin-binary-foundation
kind: feature
stage: done
tags: [plugin, security]
parent: epic-cc-plugin
depends_on: []
release_binding: v0.1.0
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

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Subcommand router**: `github.com/urfave/cli/v3` v3.x. Mature,
  clean POSIX flag handling, native subcommand tree, `--help`
  generation built in. Lighter than cobra; sufficient for our flag
  surface. Pin to a specific minor.
- **Hook IO scaffold**: a tiny helper at `cmd/jamsesh/hookio/`
  (`Run[I, O any](decode func(json.RawMessage)(I, error), handle func(ctx, I)(O, error))`)
  that reads stdin once, calls handler, writes stdout once. Hook
  subcommands plug in by type-parameterizing.
- **Local state package**: `cmd/jamsesh/state/` with `Read(file)
  ([]byte, error)`, `Write(file string, data []byte, mode os.FileMode)
  error` (atomic via temp + rename), plus typed wrappers
  `ReadToken() (string, error)`, `WriteToken(string) error`, etc.
  `${CLAUDE_PLUGIN_DATA}` resolved from env at startup.
- **OAuth client flow ordering**: browser flow is the default;
  `--device-code` is the explicit alternative. Both share the
  PKCE + state primitive (a small `oauthcommon` helper).
- **PKCE implementation**: `code_verifier = base64url(crypto/rand 32 bytes)`,
  `code_challenge = base64url(sha256(code_verifier))`,
  `code_challenge_method = "S256"`.
- **Device code polling cadence**: per RFC 8628, the portal
  returns `interval` and `expires_in` in the device-authorization
  response. Client respects `interval` (default 5s if absent),
  honors `slow_down` to back off (add 5s per error), gives up at
  `expires_in`.
- **Token-refresh single-flight**: `golang.org/x/sync/singleflight`
  group keyed by `refresh_token` (or static key "refresh" — since
  there's at most one current refresh token, either works; static
  key is simpler).
- **Portal API client**: hand-rolled minimal Go HTTP client (not
  oapi-codegen client) because the CC plugin binary calls only a
  small subset of endpoints: `/api/auth/refresh`, MCP via
  headersHelper, and the digest/turn-end endpoints later. A typed
  oapi-codegen client adds dep weight for limited value. Client
  exposes `Get/Post[T any](path string, body any) (T, error)` with
  automatic 401 → refresh → retry-once.
- **Portal URL resolution**: env `JAMSESH_PORTAL_URL` overrides
  `${CLAUDE_PLUGIN_DATA}/portal_url` overrides default
  `https://jamsesh.example.com` (placeholder — operator configures).
- **Module structure**: a single `cmd/jamsesh/main.go` registers
  all subcommands; supporting packages live at
  `cmd/jamsesh/{state,oauth,portalclient,hookio}/`. They are
  cmd-local (not under `internal/`) because they're plugin-binary-
  specific.
- **Story decomposition**: 3 stories.
  1. `router-state-mcp` — subcommand router scaffold, hook IO
     helper, state package, `jamsesh mcp-headers` subcommand,
     `jamsesh --version`. depends_on: []
  2. `oauth-browser-and-device` — `jamsesh auth` subcommand with
     local-listener + device-code modes, PKCE + state primitives.
     depends_on: [router-state-mcp]
  3. `portal-client-and-refresh` — portal client + single-flight
     refresh helper. depends_on: [router-state-mcp]

## Architectural choice

**One Go binary at `cmd/jamsesh/`. Subcommands registered as urfave/cli
commands. Supporting packages are cmd-local. Each subcommand is small,
testable in isolation, and side-effect-confined to local state +
HTTP calls to the portal.**

The binary is intentionally narrow: it doesn't include any "jamsesh"
business logic. It's a thin client that holds tokens and translates
CC hook contracts into portal API calls.

## Implementation Units

### Unit 1: Subcommand router + main.go

**File**: `cmd/jamsesh/main.go`
**Story**: `epic-cc-plugin-binary-foundation-router-state-mcp`

```go
package main

import (
    "context"
    "os"
    "os/signal"

    "github.com/urfave/cli/v3"

    "jamsesh/cmd/jamsesh/auth"
    "jamsesh/cmd/jamsesh/mcpheaders"
)

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    app := &cli.Command{
        Name:    "jamsesh",
        Usage:   "Local client for the jamsesh portal",
        Version: buildinfo.Version,
        Commands: []*cli.Command{
            auth.Command(),
            mcpheaders.Command(),
            // session-commands + hooks land in sibling features
        },
    }
    if err := app.Run(ctx, os.Args); err != nil {
        // stderr; exit 1
    }
}
```

### Unit 2: Local state package

**File**: `cmd/jamsesh/state/state.go`

```go
package state

import (
    "errors"
    "io/fs"
    "os"
    "path/filepath"
)

// PluginDataDir returns the value of CLAUDE_PLUGIN_DATA, or an error
// if unset. Documented contract: this binary requires the CC plugin
// runtime to set the env var.
func PluginDataDir() (string, error)

// Read returns the contents of <PluginDataDir>/<name> or os.ErrNotExist.
func Read(name string) ([]byte, error)

// Write atomically writes contents to <PluginDataDir>/<name> with the
// given mode. Uses temp file + rename for atomicity.
func Write(name string, data []byte, mode fs.FileMode) error

// Convenience typed wrappers:
func ReadToken() (string, error)         // reads "token", trims whitespace
func WriteToken(t string) error          // mode 0600
func ReadRefreshToken() (string, error)  // reads "refresh_token"
func WriteRefreshToken(t string) error   // mode 0600
func ReadPortalURL() (string, error)     // env JAMSESH_PORTAL_URL → file → default
```

Test: round-trip writes, atomic-write crash semantics (interrupted
temp file is invisible), mode enforcement.

### Unit 3: Hook IO scaffold

**File**: `cmd/jamsesh/hookio/hookio.go`

```go
package hookio

import (
    "context"
    "encoding/json"
    "io"
)

// Run reads JSON from in, decodes into I, calls handle, encodes the
// resulting O to out. Errors written to out as
// {"error": "...", "message": "..."} (matching the portal's envelope
// shape for consistency across CC's surface).
func Run[I, O any](ctx context.Context, in io.Reader, out io.Writer, handle func(context.Context, I) (O, error)) error
```

Used by hook subcommands (lands in the `hooks` feature).

### Unit 4: mcp-headers subcommand

**File**: `cmd/jamsesh/mcpheaders/mcpheaders.go`

```go
package mcpheaders

import (
    "context"
    "encoding/json"
    "fmt"
    "os"

    "github.com/urfave/cli/v3"

    "jamsesh/cmd/jamsesh/state"
)

func Command() *cli.Command {
    return &cli.Command{
        Name:  "mcp-headers",
        Usage: "Output the Authorization header for CC's MCP client",
        Action: func(ctx context.Context, _ *cli.Command) error {
            tok, err := state.ReadToken()
            if err != nil {
                fmt.Fprintln(os.Stderr, "no token; run `jamsesh auth` first")
                os.Exit(2)
            }
            return json.NewEncoder(os.Stdout).Encode(map[string]string{
                "Authorization": "Bearer " + tok,
            })
        },
    }
}
```

### Unit 5: OAuth flow (browser local-listener)

**File**: `cmd/jamsesh/auth/browser.go`
**Story**: `epic-cc-plugin-binary-foundation-oauth-browser-and-device`

Pattern:
1. Generate PKCE pair + state nonce
2. Start `httptest.NewServer` (or stdlib) on ephemeral port; record port
3. Build authorize URL with `client_id`, `code_challenge`,
   `code_challenge_method=S256`, `state`, `redirect_uri=http://127.0.0.1:<port>/cb`
4. `os.OpenFile`-or-`exec.Command` to open the URL in the user's
   default browser (use `github.com/pkg/browser` for cross-platform
   handling — or vendor a tiny helper inline)
5. Listener handler validates `state`, extracts `code`, signals main
   goroutine via channel
6. Main exchanges code at `/api/auth/oauth/github/callback`-like
   endpoint (TBD when auth-flows ships; for now POST to a placeholder
   `/api/auth/code` endpoint and document the assumption)
7. On success: write access + refresh tokens to local state
8. Shut down listener; print success

### Unit 6: OAuth flow (device-code)

**File**: `cmd/jamsesh/auth/device.go`

Per RFC 8628. Pattern:
1. POST to `/api/auth/device/authorize` with `client_id` + `scope`
2. Receive `device_code`, `user_code`, `verification_uri`, `interval`,
   `expires_in`
3. Print user-facing instructions: "Go to <uri> and enter code <user_code>"
4. Poll POST `/api/auth/token` with `grant_type=urn:ietf:params:oauth:grant-type:device_code`
   + `device_code` + `client_id` every `interval` seconds
5. Handle errors: `authorization_pending` → continue; `slow_down` →
   `interval += 5s` and continue; `expired_token` / `access_denied` →
   exit; success → write tokens
6. Give up at `expires_in`

### Unit 7: Portal API client

**File**: `cmd/jamsesh/portalclient/client.go`
**Story**: `epic-cc-plugin-binary-foundation-portal-client-and-refresh`

```go
package portalclient

import (
    "context"
    "encoding/json"
    "net/http"
)

type Client struct {
    BaseURL string
    HTTP    *http.Client
    // injectable refresh function (the refresher.Refresh)
    Refresh func(ctx context.Context) error
}

// Do executes req with Authorization attached, returns response or
// error. On 401, calls c.Refresh() then retries once.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error)

// GetJSON / PostJSON helpers that wrap Do + JSON encoding.
func GetJSON[T any](ctx context.Context, c *Client, path string) (T, error)
func PostJSON[T any](ctx context.Context, c *Client, path string, body any) (T, error)
```

### Unit 8: Token refresh + single-flight

**File**: `cmd/jamsesh/portalclient/refresh.go`

```go
package portalclient

import (
    "context"
    "sync"

    "golang.org/x/sync/singleflight"

    "jamsesh/cmd/jamsesh/state"
)

type Refresher struct {
    BaseURL string
    HTTP    *http.Client
    group   singleflight.Group
    mu      sync.Mutex
}

// Refresh fetches new access + refresh tokens, writes them to local
// state, returns. Concurrent calls collapse via singleflight.
func (r *Refresher) Refresh(ctx context.Context) error
```

Test: spawn N goroutines calling Refresh simultaneously, assert the
HTTP POST is called exactly once.

## Story decomposition

3 stories:

1. `cc-plugin-binary-foundation-router-state-mcp` — Units 1-4. depends_on: []
2. `cc-plugin-binary-foundation-oauth-browser-and-device` — Units 5-6. depends_on: [router-state-mcp]
3. `cc-plugin-binary-foundation-portal-client-and-refresh` — Units 7-8. depends_on: [router-state-mcp]

Stories 2 and 3 can run in parallel after 1.

## Implementation Order

1. router-state-mcp
2. (in parallel) oauth-browser-and-device + portal-client-and-refresh

## Testing

- `state/state_test.go` — read/write round-trip; atomic write semantics
  (use a t.TempDir as fake CLAUDE_PLUGIN_DATA); mode enforcement
- `hookio/hookio_test.go` — JSON in → handler → JSON out happy path;
  malformed JSON → error envelope
- `mcpheaders/mcpheaders_test.go` — token presence → JSON output;
  missing token → exit 2 with stderr message
- `auth/browser_test.go` — PKCE pair generation; state validation;
  listener handles the callback correctly (use a mock browser-open by
  injecting an `openURL func(url string)` field)
- `auth/device_test.go` — polling honors `interval`; `slow_down`
  backs off; `expired_token` exits cleanly
- `portalclient/client_test.go` — 401 → refresh → retry; 401 after
  refresh fails → error propagates
- `portalclient/refresh_test.go` — N concurrent Refresh calls → one
  HTTP POST

## go.mod additions

- `github.com/urfave/cli/v3@latest`
- `golang.org/x/sync@latest` (for singleflight)
- Optional: `github.com/pkg/browser@latest` (or inline helper)

## Risks

- **Cross-platform browser open**: `xdg-open` (Linux), `open` (macOS),
  `start` (Windows). Use `github.com/pkg/browser` to abstract; falls
  back to printing the URL if no opener available.
- **CLAUDE_PLUGIN_DATA contract**: this binary depends on CC setting
  the env var. If unset, `state.PluginDataDir()` returns an error and
  every subcommand fails gracefully with a clear message.
- **Token leak risk**: never log tokens; never write to anything but
  the mode-0600 files. Add a `go vet` linter or a small custom check
  in CI that scans for accidental `log.*token` patterns.
- **Endpoint paths assume future feature work**: the OAuth flow
  endpoints (`/api/auth/oauth/github/callback`, `/api/auth/device/authorize`)
  land in `epic-portal-foundation-auth-flows`. Until that ships, this
  binary's auth subcommand can't complete a real flow; documented in
  story bodies.

## Implementation summary

All 3 child stories at done. The `jamsesh` Go binary foundation: subcommand router, local state package (atomic writes, mode 0600), hookio scaffold, mcp-headers subcommand, both OAuth flows with PKCE+state, portal HTTP client with single-flight refresh.

### Verification
- `go build ./cmd/jamsesh` clean
- `go test ./cmd/jamsesh/...` green
- `./jamsesh --help` lists subcommands

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Capability complete. Foundation supports sibling features (hooks, session-commands, packaging) as designed. OAuth endpoint placeholders documented for auth-flows landing.
