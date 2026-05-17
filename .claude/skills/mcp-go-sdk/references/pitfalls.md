# Known pitfalls and rough edges in `modelcontextprotocol/go-sdk` v1.x

Read this when debugging unexpected behavior, planning version bumps, or
designing the spike phase of `epic-portal-api-mcp-endpoint`.

## v1.6.0 behavior changes (worth knowing)

1. **Cross-origin protection OFF by default.** Was ON in v1.4.1 / v1.5.0
   for browser-exposed endpoints. To restore: either set
   `StreamableHTTPOptions.CrossOriginProtection = &http.CrossOriginProtection{}`
   (now deprecated) or wrap externally:
   ```go
   prot := http.NewCrossOriginProtection()
   handler = prot.Handler(handler)
   ```
   Or env compat: `MCPGODEBUG=enableoriginverification=1` (removed in v1.8.0).
   jamsesh `/mcp` is agent-facing, not browser-facing — low priority.

2. **`SetError` preserves existing Content.** Was overwrite before.
   Compat: `MCPGODEBUG=seterroroverwrite=1`. Affects only direct uses
   of `(*CallToolResult).SetError`; the `ToolHandlerFor` error-return
   path is unaffected.

## Auth / token gotchas

3. **`TokenInfo.Expiration` MUST be non-zero** — middleware otherwise
   responds 401 "token missing expiration". If your tokens are opaque
   and you don't track expiry, set a sentinel far-future time.

4. **Always set `TokenInfo.UserID`.** Non-empty UserID enables session-
   hijacking protection: subsequent requests on the same
   `Mcp-Session-Id` from a different user get 403. Without it, anyone
   with the session id can hijack the conversation.

5. **`auth.RequireBearerToken` returns 401 on missing/malformed bearer.**
   Format must be `Authorization: Bearer <token>` (case-insensitive
   "bearer"; exactly 2 whitespace-separated fields).

6. **Don't do auth inside `getServer`.** Returning nil produces a 400
   "no server available" — opaque to clients. The
   `auth.RequireBearerToken` middleware returns proper 401 with
   `WWW-Authenticate: Bearer resource_metadata="..."` per RFC 9728.

## Schema / tool registration

7. **Schema draft pinned to 2020-12.** The SDK uses
   `github.com/google/jsonschema-go`. Hand-rolled `Tool.InputSchema`
   outside this draft is rejected at registration (panic).

8. **`AddTool` panics on:**
   - Invalid tool name (must match the JSONRPC name validation)
   - Missing `InputSchema` (when used with the non-generic
     `(*Server).AddTool`)
   - In-type that isn't a struct or map (JSON Schema requires "object")

9. **Use `jsonschema:` struct tags for field descriptions.** Example:
   ```go
   SessionID string `json:"session_id" jsonschema:"jamsesh session id (uuid)"`
   ```
   Not `description:` — that's a different convention.

## Transport / session

10. **Stateful is the default.** `Stateless: true` disables sessions
    (each request is independent, GET/DELETE return 405). Use stateful
    for jamsesh — agents reuse sessions across multi-turn calls.

11. **`SessionTimeout` is the only built-in idle cleanup.** No client-
    disconnect callback exists. Set ~30 min for jamsesh.

12. **DNS-rebinding protection ON by default since v1.4.1.** Localhost
    listeners (127.0.0.1, ::1) reject non-localhost `Host` headers with
    403. Disable via `StreamableHTTPOptions.DisableLocalhostProtection`
    or `MCPGODEBUG=disablelocalhostprotection=1` (removed in v1.8.0).
    Public-bound listeners are unaffected.

13. **Keepalive ping bug fixed in late v1.5/v1.6.** Earlier versions
    silently closed sessions when peer didn't implement `ping`. Set
    `ServerOptions.KeepAlive` carefully on older versions; in v1.6 the
    method-not-found response is tolerated.

## Versioning / compatibility

14. **Go 1.25 minimum since v1.4.1.**

15. **Pin the exact version in `go.mod`.** Minor releases ship behavior
    changes (cross-origin defaults, SetError semantics). Upgrades
    should be deliberate, read release notes first.

16. **MCPGODEBUG env compat flags are temporary.** Most removed in
    v1.8.0 or v1.9.0:
    - `seterroroverwrite` (v1.8.0)
    - `enableoriginverification` (v1.8.0)
    - `disablelocalhostprotection` (v1.8.0)
    - `allowsessionsinstateless` (v1.9.0)

17. **Prior breaking changes to watch for** if migrating from <v1.4:
    - Distinguished StreamID type removed
    - GetSessionID moved onto `ServerOptions`

## Protocol spec

18. **v1.6.0 defaults `latestProtocolVersion = "2025-11-25"`** and
    supports 2025-06-18 and 2025-03-26 for back-compat. Spec version
    is negotiated at `initialize`; the epic's mention of "2025-06-18"
    is the prior baseline — clients on newer or older spec versions
    both work via negotiation.

19. **`Mcp-Protocol-Version` header is mandatory** after `initialize`.
    Mismatch responds 400 "Bad Request".

## Fallback library

20. **`mark3labs/mcp-go` v0.54.0 (May 2026) is mature** but still 0.x
    — API is not stable, expect rewrite at the
    `StreamableHTTPServer`/options layer if jamsesh needs to switch.
    Use only if a hard v1.6 bug blocks us.
