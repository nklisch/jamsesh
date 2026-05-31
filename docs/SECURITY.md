# Security

Trust boundaries, authentication, authorization, and what the portal can
and cannot see.

## Trust boundary

The single sentence that defines jamsesh's security posture:

> The portal is jam-scoped. It never touches the source repo. The source
> repo is sacrosanct.

Concretely, this means:

- The portal holds no credentials that grant access to anything other than
  the in-flight session content it hosts.
- The portal makes no API calls against any source forge (GitHub, GitLab,
  etc.).
- The portal pushes nothing to the source repo. All source-repo writes are
  performed by the human's own machine using their own credentials.
- The portal's blast radius under any breach is the in-flight session
  content only — never the user's source repo, never their other projects,
  never their git credentials.

## Authentication

### User authentication

Two supported flows. Both result in a long-lived user OAuth bearer token
stored locally by Claude Code.

**OAuth flow (recommended for hosted and browser-capable self-host):**

1. User runs `jamsesh auth` or triggers OAuth via CC's `/mcp` command.
2. CC opens a browser to the portal's OAuth authorization endpoint.
3. User authenticates to the portal (which itself may use GitHub OAuth or
   email magic link to verify identity — portal's choice).
4. Portal redirects back to CC's local callback with an authorization code.
5. CC exchanges the code for an access token + refresh token.
6. Tokens stored in CC's secure credential store (system keychain on
   supported platforms, encrypted file otherwise).

**Magic-link flow (for headless self-host):**

1. User runs `jamsesh auth --email <addr>`.
2. Portal emails a magic link.
3. User opens the link on any browser-capable device; portal binds the
   pending CLI session to the magic-link confirmation.
4. CLI polls for token; receives it on confirmation.

### Token lifetime and renewal

- Access tokens: 1 hour TTL.
- Refresh tokens: 30 days TTL, renewed on use (sliding window).
- Refresh tokens can be revoked from the portal admin UI.
- Token revocation propagates within 1 minute (active sessions verify on
  every protected request).

### Service-to-service authentication

There is no service-to-service authentication. The portal is a single
process; the local binary is a single process. All cross-component auth is
the user's token.

## Authorization

All authorization is enforced server-side. Client-side checks (in the local
binary or the agent's prompts) are advisory only.

### MCP and REST API authorization

Every tool call and API request carries the user's bearer token. The portal:

1. Validates the token (signature, not expired, not revoked).
2. Resolves the user.
3. For session-scoped requests, validates the user is a member of the
   `session_id` named in the request.
4. For role-restricted operations (admin endpoints, member removal, session
   abandon), validates the user holds the required role.

### Git push authorization

Every push goes through `pre-receive`, which:

1. Validates HTTP Basic auth (password = user OAuth token).
2. Resolves the user.
3. For each ref being updated:
   - Verifies the ref name is in the user's namespace
     (`jam/<session>/<user>/*`), or is the user's own first-push of `base`
     during session creation.
   - Verifies the user is a current member of the session.
   - Rejects pushes to `base` (after creation), `draft`, or other users'
     namespaces.
   - Rejects force-pushes on any shared ref.
4. For each commit being pushed:
   - Verifies required commit trailers (`Jam-Session`, `Jam-Turn`,
     `Jam-Author`) are present.
   - Verifies all changed paths fall within the session's writable scope.

Failures return git-protocol errors with structured rejection messages
listing the offending refs/commits/paths.

### Auto-merger authorization

The auto-merger runs server-side with privileged write access to the session
repo. It can write to `draft` (no other party can). Its writes are bounded
to merge commits whose parents are commits already in the repo via legitimate
user pushes — it never invents content. The auto-merger does not act on
addressed comments; it only processes pushed commits.

## Trust model for participants

### Honest participants

The expected case. Members of a session can:
- Read everything in the session.
- Write to their own namespace.
- Fork from any commit (creates a ref under their own namespace).
- Comment anywhere with any addressing.
- Hit finalize (non-destructive).

### Mistaken or buggy participants

A misbehaving plugin or an agent running raw bash commands cannot:
- Push to refs outside its namespace (rejected by pre-receive).
- Push commits without required trailers (rejected).
- Push commits touching paths outside writable scope (rejected).
- Force-push shared refs (rejected).
- Bypass pre-receive (it's server-side; the smart-HTTP handler always invokes it).

A misbehaving plugin can post unexpected comments via MCP. This is bounded
by the comment-quantity rate limits and is auditable.

### Adversarial participants

A member with valid credentials can:
- Comment provocatively (auditable; resolvable; revocable via member removal)
- Fork excessively (creates clutter; auditable; can be addressed by removal)
- Use up portal resources within rate limits

A member cannot:
- Push to other members' namespaces.
- Tamper with auto-merger logic (server-side, signed by us).
- Read sessions they're not a member of.
- Access other orgs' data.

The creator can remove an adversarial member at any time, which:
- Revokes the member's session-scoped authorization immediately.
- Marks their refs read-only (preserves attribution and history).
- Removes them from comment-addressing autocompletes.

### Network adversaries

All client-to-portal communication is HTTPS. Tokens are bearer; loss of a
token means loss of authentication, mitigated by refresh-token revocation and
short access-token TTL.

The portal does not currently use mTLS for self-hosted deployments. Operators
who require mTLS can place the portal behind a reverse proxy that terminates
TLS with client certificates.

## What a portal breach exposes

In the worst case (full database read + bare-repo filesystem read):

- All session content (commits, file contents, draft history) for active and
  unarchived sessions.
- All comments, conflict events, presence data.
- OAuth tokens, including refresh tokens (granting continued access until
  detected and revoked).
- Email addresses and display names of all portal accounts.
- Git author identities.

The breach does NOT expose:
- Any source-repo content beyond what was brought into a jam session.
- Any user's git credentials for their source remote.
- Any forge access tokens.
- Sessions that have been archived past their retention window (bare repos
  and social state are deleted).

## What a single-user-token compromise exposes

If one user's OAuth token leaks:

- The attacker can act as that user for the token's lifetime (max 1 hour for
  access tokens, until next refresh for refresh tokens).
- The attacker can push to that user's namespaces in all sessions the user is
  a member of.
- The attacker can read all those sessions.
- The attacker cannot access sessions the user is not a member of.
- The attacker cannot escalate to source-repo access (no source-repo
  credentials in the portal).

Mitigation:
- Refresh-token revocation cuts off further token issuance within 1 minute.
- Active access tokens expire within 1 hour, limiting the post-revocation
  window.

## Client-side token storage and XSS residual risk

The portal SPA stores two categories of token on the client:

**Durable OAuth tokens (refresh tokens persisted to `localStorage`):**
`frontend/src/lib/auth.svelte.ts` writes both the access token and the
30-day refresh token to `localStorage` under the keys `jamsesh.token` and
`jamsesh.refresh`. Any XSS vulnerability in the SPA — including a future
inline-script regression in the Svelte bundle — can exfiltrate both halves of
the token pair. The access token has a short TTL (≤1 hour); the refresh token
is long-lived and would allow an attacker to issue fresh access tokens until
the refresh token is explicitly revoked.

Residual risk accepted: moving refresh tokens to an `httpOnly Secure` cookie
requires rework of the token-refresh API contract and is deferred to a future
hardening pass. The current `script-src 'self'` CSP directive blocks inline
script injection from known-XSS vectors; a CSP report endpoint (see below)
surfaces regressions.

**Playground anonymous bearers (in-memory only, NOT in `localStorage`):**
The `_playgroundContext` rune in `auth.svelte.ts` holds the anonymous bearer
for the current playground session in module-level `$state` — it is never
written to `localStorage` or any persistent storage. A page reload loses the
playground context (by design). XSS can still exfiltrate the in-memory bearer
during the page session, but the blast radius is limited to the single
playground session (see [Anonymous session-scoped bearers](#anonymous-session-scoped-bearers)).

**CLI resume-exchange client-side safeguards:** The resume exchange flow
(`ResumeExchange.svelte`) implements three independent client-side defenses
that complement the server-side single-use enforcement:

1. **Fragment strip in `onMount`, before exchange and navigation.** On mount,
   `history.replaceState` removes `#rt=<token>` from the address bar — after the
   initial render but before the exchange fetch is dispatched and before any
   further SPA navigation. The token does not appear in DevTools "current URL",
   subsequent `window.location.href` reads, or browser history entries created
   after the strip. (The strip cannot happen before the first render because it
   runs in `onMount`; it does happen before any meaningful action the component
   takes on the token.)
2. **Bare fetch, `credentials: 'omit'`.** The exchange POST uses a hand-rolled
   `fetch(…, {credentials:'omit', headers:{'content-type':'application/json'},
   body})` rather than the shared openapi-fetch client. The shared client's
   `bearerMiddleware` would attach any currently authenticated user's access
   token as an `Authorization` header. The resume token is the sole credential
   for the exchange and must arrive without interference from ambient browser
   auth state.
3. **Token and bearer never enter reactive state.** The raw resume token is
   read from the hash into a local variable and never assigned to any `$state`
   rune. The returned bearer is stored in a plain non-reactive variable during
   the confirm flow and is discarded immediately after adoption (or on decline).
   Neither value is ever passed to a template rendering expression, preventing
   accidental DOM exposure, screenshot capture, or logging via reactivity traces.

The `requiresAuth: false` flag on both resume routes (`/playground/s/{id}/resume`
and `/orgs/{orgId}/sessions/{sessionId}/resume`) ensures the SPA auth gate does
not redirect the user to `/login` before the fragment can be read and stripped.

**CSP regression detection:** A `Content-Security-Policy-Report-Only` header
with `report-uri /_csp-report` is emitted alongside the enforced CSP so
inline-script policy violations surface in server logs. The `/_csp-report`
route is a placeholder; see backlog item
`bug-csp-report-endpoint-not-wired` for wiring an actual report receiver.

## Supply chain and integrity

- The `jamsesh` binary is built reproducibly from public source and
  distributed as GitHub release assets with cryptographic checksums. The
  plugin's `plugins/jamsesh/bin/jamsesh` wrapper verifies sha256 against the signed
  `checksums.txt` before exec, and additionally validates the cosign
  sigstore bundle when `cosign` is on the user's PATH.
- The portal binary likewise.
- Releases are signed with Sigstore cosign in keyless mode (GitHub OIDC).
  Signatures are verified at fetch time by the plugin wrapper (`plugins/jamsesh/bin/jamsesh`)
  and at install time by the self-host install flows using
  `--certificate-identity-regexp` pinned to the jamsesh release workflow and
  `--certificate-oidc-issuer https://token.actions.githubusercontent.com`.
- The keyless-signing trust anchor is the release workflow's identity. A
  compromise of `.github/workflows/release.yml` on the `main` branch would
  produce "valid" signatures, so the workflow file and the `main` branch
  carry branch-protection rules requiring code-owner review for any change.
- Dependencies are pinned; security advisories are watched and patched
  promptly.

## Self-host security posture

Self-host operators are responsible for:
- TLS termination (recommended via reverse proxy with Let's Encrypt or
  similar).
- Database backup and disaster recovery.
- Network access controls (the portal binds HTTPS by default; firewall rules
  for who can reach it).
- OAuth callback URL configuration.
- Patching the portal binary as security updates ship.
- **Finalize fetch tokens passed via Authorization header, not git URL** —
  when the jamsesh plugin fetches session refs during finalize-run, it mints
  an ephemeral fetch token and passes it as an HTTP `Authorization: Bearer`
  header via `git -c http.extraHeader=...`. The token is **never** embedded
  in the git remote URL. This means:
  - The token does not persist into `.git/config` after the clone/fetch.
  - The token does not appear in `git remote -v` output.
  - The token is not logged by git's own credential helper chain.
  Operators can confirm this behavior matches their threat model by auditing
  the `http.extraHeader` env var path (the portal's
  `POST .../finalize/fetch-token` response carries a plain `remote_url` with
  no userinfo segment, and a separate `token` field). Proxy access logs
  will show the Authorization header value on requests to `/git/...` during
  finalize; ensure those logs are scoped appropriately given the 5-minute TTL.
- **Object storage IAM** — when clustered mode and object-storage are enabled,
  the operator must configure a service principal or IAM role with
  bucket-scoped read/write/list/delete permissions on the bucket named in
  `JAMSESH_OBJECT_STORAGE_URL`. Workload identity is preferred (GKE Workload
  Identity, AKS Workload Identity, EKS IRSA) because it avoids static
  credentials. Static credentials (`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`
  or equivalent) are acceptable for non-cloud providers (Cloudflare R2,
  Backblaze B2, MinIO). Scope credentials to the minimum required permissions:
  `PutObject`, `GetObject`, `DeleteObject`, `ListBucket` (S3 names; equivalent
  on GCS and Azure). Do not grant cross-bucket or account-wide permissions.

The portal is designed to be safe in a hostile network with default
configuration (HTTPS-only, token-authenticated, no anonymous endpoints
except auth initiation).

## Anonymous session-scoped bearers

When playground sessions are enabled (`JAMSESH_PLAYGROUND_ENABLED=true`),
participants are issued **anonymous session-scoped bearers**: a new
`accounts` row marked `is_anonymous: true` (with a synthetic
`anon_<random>@playground.local` email) and an `oauth_tokens` row with
`kind=anonymous_session_bearer` and a `session_id` FK pinning the bearer
to one session.

**Threat model:**
- **Token leak blast radius**: a leaked anonymous bearer authenticates
  only the session it was issued for. No cross-session privilege; no
  org-scope access (the playground org's `org_members` table is never
  populated with anonymous accounts).
- **Bearer lifetime**: two independent expiry mechanisms — the `expires_at`
  column (synced to the session's hard-cap deadline, e.g., 24h) and the
  destruction-sweep revocation (sets `revoked_at` at session end). Either
  failing means the bearer naturally expires within the session window.
- **Reuse after destruction**: impossible — both the `oauth_tokens` row
  AND the underlying `accounts` row are cascade-deleted with the session
  (per the session-lifecycle feature's destruction routine).
- **Hijacking mid-session**: a hijacked bearer can act as the original
  participant within the session — same blast radius as a session
  hijack of a durable bearer. No additional mitigation in v1; consider
  bearer rotation in a future hardening pass if abuse data justifies.

Anonymous accounts never participate in another session (per-session-row
identity); they never appear in `org_members`; they cannot be promoted
to durable accounts (the "claim-to-durable" path is explicitly deferred
per SPEC.md's deferred-features list).

## Abuse model for playground sessions

Playground sessions are open to anyone who can reach the portal. The abuse
surface is different from durable sessions (which require portal accounts and
invite-based membership). The mitigations below are independent layers; each
addresses a distinct vector.

### Per-IP rate limit rationale

`POST /api/playground/sessions` is the only unauthenticated session-creation
surface. Without a rate limit, a single actor can spin up sessions at whatever
speed the portal allows, exhausting database rows, bare-repo storage, and
destruction-worker capacity.

The default cap of **3 sessions per hour per source IP** is chosen to balance:
- **Spam prevention**: a burst of 3 is sufficient for a developer to quickly
  evaluate the playground (create, share, tear down, retry) without hitting
  the limit in normal use.
- **Abuse cost**: exceeding 3/hour requires rotating IPs; the attacker's cost
  scales faster than the operator's mitigation cost.

The limit is enforced by a per-IP token bucket in the existing
`internal/portal/ratelimit` infrastructure, mounted as middleware on the
playground create route only. Join requests are not rate-limited — they require
a valid session ID (not guessable at scale) and are bounded by the participant
cap.

Operators tune this with `JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR`.

### Content-size cap

Each playground session enforces a maximum accumulated content size (default
**50 MiB**, `JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES`). The cap serves two
independent goals:

1. **Abuse prevention**: without a cap, a single session can push large
   packfiles and consume disproportionate portal storage, affecting other
   sessions and the destruction worker's cleanup time.
2. **Storage-cost guard**: operators running hosted playground surfaces need
   predictable per-session storage bounds to cost-model the feature. 50 MiB
   covers realistic evaluation workloads (a full source tree with history)
   while keeping worst-case storage at a tractable per-session ceiling.

The cap is enforced at `pre-receive` by `CheckPlaygroundCaps`, which fires
after all other validation succeeds. Pushes that would cause the session's
total object storage to exceed the threshold are rejected with
`remote: ERROR: playground.size_exceeded`. The check compares the incoming
packfile's object count against the current session storage total reported by
the storage backend — no rolling-window accounting required.

### Joiner overflow as DoS prevention

The **5-participant cap** (`JAMSESH_PLAYGROUND_MAX_PARTICIPANTS`) prevents a
single playground session from consuming unbounded server resources through
join flood. Each participant gets a database row, a session-members row, a
bearer token, and a presence WebSocket connection. Without a cap, a publicly
shared join URL becomes a resource-exhaustion vector.

When the cap is reached, `POST /api/playground/sessions/{id}/join` returns
`409 playground.session_full`. This is a hard error with a retry hint and a
CTA to start a fresh session — no waitlist, no spectator tier, no partial
admission. The simplicity is intentional: low-ceremony design means fewer
states that can be abused.

The participant cap is checked inside the join transaction, not as a pre-check,
to avoid TOCTOU races at the boundary.

### Cross-reference: anonymous bearer blast radius

The threat model for a leaked or hijacked anonymous session bearer — blast
radius, lifetime, and reuse-after-destruction protections — is covered in the
[Anonymous session-scoped bearers](#anonymous-session-scoped-bearers) section
above. That section is owned by the anon-bearer feature's documentation pass;
this section does not duplicate it.

## Audit trail

Everything is auditable:
- All commits exist in git history with structured trailers identifying the
  author and turn.
- All comments are stored with author, timestamp, addressing.
- All conflict events have full provenance (which commits, which ancestor).
- All auto-merger actions are commits with `Auto-Merger: true` trailer.
- All admin actions (member removal, session abandon, scope changes) are
  recorded in the event log.
- Token issuance and revocation are logged.

Auditors can reconstruct who did what, when, and why from a combination of
the git history and the portal event log.
