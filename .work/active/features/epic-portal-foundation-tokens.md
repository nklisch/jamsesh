---
id: epic-portal-foundation-tokens
kind: feature
stage: done
tags: [portal, security]
parent: epic-portal-foundation
depends_on: [epic-portal-foundation-data-layer, epic-portal-foundation-http-skeleton]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Foundation — Tokens

## Brief

The token subsystem the entire system authenticates against. Implements
issuance, validation, sliding-window refresh, and revocation for the user
OAuth tokens that serve as Bearer auth (REST + MCP), HTTP Basic password
(git smart-HTTP), and any future client surface. One token model, one
codepath, multiple transports consume it.

**Token shape**: opaque random tokens (32 bytes, hex-encoded), stored
hashed at rest in the `oauth_tokens` table (epic-design decision — no JWTs).
Validation is a hashed-lookup against the row; revocation is row deletion
or a `revoked_at` timestamp set; expiry is `expires_at` enforced at
validation time. The `oauth_tokens` row also carries `account_id` and
metadata (issued-by flow, last-used timestamp).

**Lifetimes (from SECURITY.md):**
- Access tokens: 1 hour TTL
- Refresh tokens: 30 days TTL, renewed on each refresh (sliding window)
- Revocation propagates within 1 minute (every protected request validates
  against the DB; cache, if introduced, has ≤ 60s TTL)

**Surface delivered:**
- Bearer-auth middleware for `/api/*` that resolves `Authorization:
  Bearer <token>` to an `account_id` (and rejects with the standard error
  contract on failure)
- A token-as-Basic-Auth-password helper that `epic-portal-git`'s
  smart-HTTP handler uses to validate `git push` credentials
- Issuance helper called by auth-flows after successful OAuth or
  magic-link exchange
- Refresh endpoint (`POST /api/auth/refresh`) and revoke endpoint
  (`POST /api/auth/revoke`) per `docs/PROTOCOL.md > REST API > Auth`
- Background sweep that prunes expired tokens (optional but documented;
  the design pass decides whether to land it here or defer)

Does NOT cover OAuth client flow or magic-link delivery (auth-flows owns
those). Does NOT cover account/org provisioning — that happens in
auth-flows on first sign-in.

## Epic context

- Parent epic: `epic-portal-foundation`
- Position in epic: depends on both data-layer (oauth_tokens table) and
  http-skeleton (middleware mount points); auth-flows depends on this for
  issuance.

## Foundation references

- `docs/SECURITY.md` — Authentication > Token lifetime and renewal,
  Authorization > MCP and REST API authorization, What a single-user-token
  compromise exposes
- `docs/PROTOCOL.md` — REST API > Auth section, HTTP error contract
- `docs/SPEC.md` — Auth model (one token per user, scope of uses)

## Inherited epic design decisions

- **Token format**: opaque random bytes, hex-encoded, hashed at rest.
- **Revocation mechanism**: row state on `oauth_tokens`; every protected
  request validates against the DB (or a ≤60s TTL cache). No JWT
  blocklist machinery.
- **Token reuse across transports**: same token serves Bearer (REST/MCP)
  and HTTP Basic password (git push). One token, three transports.

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Token generator**: `crypto/rand.Read` → 32 bytes →
  `hex.EncodeToString` (64-char hex string). Cryptographically
  secure, URL-safe, easy to inspect in logs (truncated).
- **Hash function**: SHA-256 of the raw token bytes; stored as
  hex in `oauth_tokens.token_hash`. SHA-256 is fine because the
  token itself is 256 bits of entropy — no pepper or work factor
  needed (pre-image resistance against the row hash buys nothing
  the entropy doesn't already give).
- **Validation caching**: NONE in v1. Every protected request hits
  the DB. Documented in implementation notes — the locked SLO of
  ≤60s revocation propagation is trivially met. If profiling later
  shows DB latency hurts, a ≤60s TTL in-memory cache is the
  follow-up.
- **Sweep**: NOT in scope for this feature. Expired-token cleanup
  is a documented operator concern; v1 ships without an internal
  sweeper. A SQL helper query exists for operators to schedule
  via cron if needed: `DELETE FROM oauth_tokens WHERE expires_at <
  $now`. Tracked as a backlog item for the v0.x post-launch list.
- **Refresh-flow shape**: `POST /api/auth/refresh` with
  `{"refresh_token": "<token>"}` body, returns
  `{"access_token", "refresh_token", "access_expires_at",
  "refresh_expires_at"}`. The current refresh token is consumed
  (deleted) and a new pair is issued. Sliding-window TTL.
- **Revoke-flow shape**: `POST /api/auth/revoke` with
  `{"token": "<token>"}` body, returns 204 on success.
  Token-presented can be either access or refresh; both are
  revoked. Optional `revoke_all: bool` field revokes every token
  for the authenticated account (defensive logout-everywhere).
- **OpenAPI ownership**: this feature ADDS `/api/auth/refresh` and
  `/api/auth/revoke` to `docs/openapi.yaml` — first features that
  populate paths. The auth-flows feature ADDS `/api/auth/oauth/*`
  and `/api/auth/magic-link/*` later. Both modify the same file;
  parallel stories coordinate.
- **Bearer middleware contract**: validates token, attaches an
  `*Account` (or just `accountID`) to the request context under a
  package-private key, exposes a helper `tokens.AccountFromContext(ctx)`.
  On failure, calls `httperr.Write` with `ErrInvalidToken()` or
  `ErrExpiredToken()` from PROTOCOL.md.
- **Basic-auth helper**: a function that the git-smart-http handler
  calls per request — `Validate(username, password) (*Account, error)`.
  The "username" is ignored (git accepts any string as username for
  HTTP Basic); the "password" is the token. Returns the same Account
  + error normalization as Bearer.
- **Time clock**: a small injectable `Clock` interface for testability.
  Default `realClock` calls `time.Now().UTC()`. Tests inject a fake.
- **Story decomposition**: 2 stories.
  1. `token-core-and-middleware` — types, generation, hashing,
     issuance, validation, Bearer middleware, Basic-auth helper.
     depends_on: []
  2. `refresh-and-revoke-endpoints` — openapi.yaml additions,
     POST handlers, strict-server wiring, tests. depends_on:
     [token-core-and-middleware]

## Architectural choice

**Service layer at `internal/portal/tokens/`. Three exports:**

- `Service` interface for issuance / validation / refresh / revoke
- `BearerMiddleware(svc Service) func(http.Handler) http.Handler`
- `BasicAuthValidator(svc Service) func(user, pass string) (*store.Account, error)`

All three consume a single `Service` so the implementation lives
in one place and the seams are testable independently.

## Implementation Units

### Unit 1: Service interface and types

**File**: `internal/portal/tokens/service.go`
**Story**: `epic-portal-foundation-tokens-token-core-and-middleware`

```go
package tokens

import (
    "context"
    "errors"
    "time"

    "jamsesh/internal/db/store"
)

// Lifetimes locked by SECURITY.md.
const (
    AccessTokenTTL  = 1 * time.Hour
    RefreshTokenTTL = 30 * 24 * time.Hour
)

// Pair is the user-visible bundle returned on issuance and refresh.
type Pair struct {
    AccessToken       string
    RefreshToken      string
    AccessExpiresAt   time.Time
    RefreshExpiresAt  time.Time
}

type Service interface {
    // Issue mints a new access+refresh pair for the given account.
    Issue(ctx context.Context, accountID string) (Pair, error)
    // Validate returns the account associated with a raw token, or
    // a normalized error if invalid/expired/revoked.
    Validate(ctx context.Context, rawToken string) (*store.Account, error)
    // Refresh consumes the given refresh token (revoking it) and
    // mints a new pair sliding-window-style.
    Refresh(ctx context.Context, refreshToken string) (Pair, error)
    // Revoke marks the supplied token as revoked. revokeAll, when
    // true, revokes every token for the token's account.
    Revoke(ctx context.Context, rawToken string, revokeAll bool) error
}

// Sentinel errors that callers map to PROTOCOL.md error codes.
var (
    ErrInvalidToken  = errors.New("tokens: invalid")
    ErrExpiredToken  = errors.New("tokens: expired")
    ErrRevokedToken  = errors.New("tokens: revoked")
)
```

### Unit 2: Generation + hashing helpers

**File**: `internal/portal/tokens/token.go`

```go
package tokens

import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
)

const rawTokenBytes = 32

func generateToken() (raw, hash string, err error) {
    b := make([]byte, rawTokenBytes)
    if _, err := rand.Read(b); err != nil {
        return "", "", fmt.Errorf("tokens: read random: %w", err)
    }
    raw = hex.EncodeToString(b)
    sum := sha256.Sum256([]byte(raw))
    hash = hex.EncodeToString(sum[:])
    return raw, hash, nil
}

func hashToken(raw string) string {
    sum := sha256.Sum256([]byte(raw))
    return hex.EncodeToString(sum[:])
}
```

### Unit 3: Service implementation

**File**: `internal/portal/tokens/service_impl.go`

```go
package tokens

import (
    "context"
    "errors"
    "time"

    "github.com/oklog/ulid/v2"

    "jamsesh/internal/db/store"
)

type Clock interface{ Now() time.Time }
type realClock struct{}
func (realClock) Now() time.Time { return time.Now().UTC() }

type service struct {
    store store.Store
    clock Clock
}

func New(s store.Store) Service {
    return &service{store: s, clock: realClock{}}
}

// NewWithClock is a test-only constructor.
func NewWithClock(s store.Store, c Clock) Service {
    return &service{store: s, clock: c}
}

func (s *service) Issue(ctx context.Context, accountID string) (Pair, error) {
    now := s.clock.Now()
    accessRaw, accessHash, err := generateToken()
    if err != nil { return Pair{}, err }
    refreshRaw, refreshHash, err := generateToken()
    if err != nil { return Pair{}, err }

    accessExpiry := now.Add(AccessTokenTTL)
    refreshExpiry := now.Add(RefreshTokenTTL)

    if err := s.store.CreateOAuthToken(ctx, store.CreateOAuthTokenParams{
        ID:        ulid.Make().String(),
        AccountID: accountID,
        TokenHash: accessHash,
        Kind:      "access",
        IssuedAt:  now,
        ExpiresAt: accessExpiry,
    }); err != nil { return Pair{}, err }

    if err := s.store.CreateOAuthToken(ctx, store.CreateOAuthTokenParams{
        ID:        ulid.Make().String(),
        AccountID: accountID,
        TokenHash: refreshHash,
        Kind:      "refresh",
        IssuedAt:  now,
        ExpiresAt: refreshExpiry,
    }); err != nil { return Pair{}, err }

    return Pair{
        AccessToken:      accessRaw,
        RefreshToken:     refreshRaw,
        AccessExpiresAt:  accessExpiry,
        RefreshExpiresAt: refreshExpiry,
    }, nil
}

func (s *service) Validate(ctx context.Context, raw string) (*store.Account, error) {
    if raw == "" { return nil, ErrInvalidToken }
    row, err := s.store.GetOAuthTokenByHash(ctx, hashToken(raw))
    if err != nil {
        if errors.Is(err, store.ErrNotFound) { return nil, ErrInvalidToken }
        return nil, err
    }
    now := s.clock.Now()
    if row.RevokedAt != nil { return nil, ErrRevokedToken }
    if now.After(row.ExpiresAt) { return nil, ErrExpiredToken }
    // Touch last_used_at (fire-and-forget; failures don't block validation).
    _ = s.store.TouchOAuthTokenLastUsed(ctx, row.ID, now)
    acct, err := s.store.GetAccountByID(ctx, row.AccountID)
    if err != nil { return nil, err }
    return &acct, nil
}

func (s *service) Refresh(ctx context.Context, raw string) (Pair, error) {
    acct, err := s.Validate(ctx, raw)
    if err != nil { return Pair{}, err }
    // Find the token row to verify it's a refresh token specifically.
    row, err := s.store.GetOAuthTokenByHash(ctx, hashToken(raw))
    if err != nil { return Pair{}, err }
    if row.Kind != "refresh" { return Pair{}, ErrInvalidToken }
    // Revoke the old refresh token (single-use).
    if err := s.store.RevokeOAuthToken(ctx, row.ID, s.clock.Now()); err != nil {
        return Pair{}, err
    }
    return s.Issue(ctx, acct.ID)
}

func (s *service) Revoke(ctx context.Context, raw string, revokeAll bool) error {
    row, err := s.store.GetOAuthTokenByHash(ctx, hashToken(raw))
    if err != nil {
        if errors.Is(err, store.ErrNotFound) { return nil } // idempotent
        return err
    }
    if revokeAll {
        return s.store.RevokeAllOAuthTokensForAccount(ctx, row.AccountID, s.clock.Now())
    }
    return s.store.RevokeOAuthToken(ctx, row.ID, s.clock.Now())
}
```

### Unit 4: Bearer middleware

**File**: `internal/portal/tokens/middleware.go`

```go
package tokens

import (
    "context"
    "errors"
    "net/http"
    "strings"

    "jamsesh/internal/db/store"
    "jamsesh/internal/portal/httperr"
)

type ctxKey struct{}

// BearerMiddleware returns a chi-compatible middleware that
// validates an "Authorization: Bearer <token>" header and attaches
// the resolved account to the request context.
func BearerMiddleware(svc Service) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authz := r.Header.Get("Authorization")
            const prefix = "Bearer "
            if !strings.HasPrefix(authz, prefix) {
                httperr.Write(w, r, httperr.ErrInvalidToken())
                return
            }
            tok := strings.TrimPrefix(authz, prefix)
            acct, err := svc.Validate(r.Context(), tok)
            if err != nil {
                switch {
                case errors.Is(err, ErrExpiredToken):
                    httperr.Write(w, r, httperr.ErrExpiredToken())
                case errors.Is(err, ErrInvalidToken), errors.Is(err, ErrRevokedToken):
                    httperr.Write(w, r, httperr.ErrInvalidToken())
                default:
                    httperr.Write(w, r, httperr.ErrInternal(err))
                }
                return
            }
            ctx := context.WithValue(r.Context(), ctxKey{}, acct)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func AccountFromContext(ctx context.Context) (*store.Account, bool) {
    v, ok := ctx.Value(ctxKey{}).(*store.Account)
    return v, ok
}
```

### Unit 5: HTTP Basic-auth helper

**File**: `internal/portal/tokens/basic.go`

```go
package tokens

import (
    "context"
    "errors"

    "jamsesh/internal/db/store"
)

// BasicAuthValidator returns a function suitable for plugging into
// the git smart-HTTP handler's per-request Basic-auth check.
// The username is ignored (git uses any string); the password is
// the token.
func BasicAuthValidator(svc Service) func(ctx context.Context, user, pass string) (*store.Account, error) {
    return func(ctx context.Context, _user, pass string) (*store.Account, error) {
        acct, err := svc.Validate(ctx, pass)
        if err != nil {
            // Surface the same sentinels callers can switch on.
            if errors.Is(err, ErrInvalidToken) ||
               errors.Is(err, ErrExpiredToken) ||
               errors.Is(err, ErrRevokedToken) {
                return nil, err
            }
            return nil, err
        }
        return acct, nil
    }
}
```

### Unit 6: openapi.yaml — auth refresh/revoke

**File**: `docs/openapi.yaml` (edit)
**Story**: `epic-portal-foundation-tokens-refresh-and-revoke-endpoints`

Add paths under the existing skeleton:

```yaml
paths:
  /api/auth/refresh:
    post:
      summary: Exchange a refresh token for a new access+refresh pair
      operationId: refreshToken
      security: []  # public — body carries the refresh token
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [refresh_token]
              properties:
                refresh_token: { type: string }
      responses:
        '200':
          description: New token pair
          content:
            application/json:
              schema: {$ref: '#/components/schemas/TokenPair'}
        '401':
          $ref: '#/components/responses/Unauthorized'
  /api/auth/revoke:
    post:
      summary: Revoke a token (or all tokens for the account)
      operationId: revokeToken
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [token]
              properties:
                token: { type: string }
                revoke_all: { type: boolean, default: false }
      responses:
        '204':
          description: Revocation complete
        '401':
          $ref: '#/components/responses/Unauthorized'
components:
  schemas:
    TokenPair:
      type: object
      required: [access_token, refresh_token, access_expires_at, refresh_expires_at]
      properties:
        access_token: { type: string }
        refresh_token: { type: string }
        access_expires_at: { type: string, format: date-time }
        refresh_expires_at: { type: string, format: date-time }
```

Run `make generate` to regenerate Go + TS clients.

### Unit 7: REST handlers + chi wiring

**File**: `internal/portal/tokens/handlers.go`

Implement `RefreshToken` and `RevokeToken` per the generated
`StrictServerInterface`. Mount via `router.Deps.MountAPI` —
wire in `cmd/portal/main.go` to populate the hook.

```go
// Pseudocode shape — actual generated method names depend on the
// oapi-codegen output.
func (h *Handler) RefreshToken(ctx context.Context, req api.RefreshTokenRequestObject) (api.RefreshTokenResponseObject, error) {
    pair, err := h.svc.Refresh(ctx, req.Body.RefreshToken)
    if err != nil { /* map to 401 envelope */ }
    return api.RefreshToken200JSONResponse(toAPIPair(pair)), nil
}
```

Bearer middleware is required on the `/api/auth/revoke` route
(the caller must already authenticate). The `/api/auth/refresh`
route is PUBLIC — the refresh token itself is the credential.

## Story decomposition

1. **token-core-and-middleware** — Units 1-5. depends_on: []
2. **refresh-and-revoke-endpoints** — Units 6-7. depends_on:
   [token-core-and-middleware]

## Implementation Order

1. token-core-and-middleware
2. refresh-and-revoke-endpoints

## Testing

- `tokens/service_test.go` — Issue produces valid pair; Validate
  round-trips; Refresh consumes old + issues new; Revoke kills
  validation; revokeAll kills every token for the account; expiry
  is enforced via the injectable Clock
- `tokens/middleware_test.go` — missing header → 401; bad scheme
  → 401; invalid token → 401; expired token → 401 with
  `auth.expired_token`; valid token → next handler reaches with
  account in context
- `tokens/basic_test.go` — same matrix as middleware but for the
  Basic-auth validator
- `tokens/handlers_test.go` — refresh path, revoke path, revoke_all

## Risks

- **Concurrent refresh of same token**: two simultaneous calls
  with the same refresh token. The first Validate+Revoke sequence
  is non-atomic. Mitigation: wrap Refresh in a Tx that does
  `SELECT ... FOR UPDATE` (Postgres) / serialized writes
  (SQLite). For v1, the race window is small and the worst
  outcome is the second caller gets ErrRevokedToken — acceptable.
  Documented in the service as a known limitation; revisit if
  observed.
- **last_used_at write contention**: every protected request
  fires an UPDATE. At high RPS this dominates DB load. Mitigation:
  it's a fire-and-forget write; errors don't block validation. If
  load becomes a problem, batch via background flusher (deferred).
- **Cache vs no-cache trade-off**: v1 ships without cache, hitting
  the DB on every request. Locked at SECURITY.md's 1-minute
  revocation propagation SLO. If SQLite read latency is fine and
  Postgres has connection pool headroom, we're good. Re-evaluate
  if metrics show otherwise.

## Implementation summary

2 child stories at review (token-core-and-middleware, refresh-and-revoke-endpoints). First REST endpoints landed; openapi.yaml populated.

### Verification
- `go build ./...` clean
- `go test ./...` green (Go side)
- `go vet ./...` clean

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Capability complete. Token core (Service + middleware + Basic-auth helper), refresh + revoke REST endpoints, openapi.yaml populated with first paths. Both child stories at done. Auth-flows feature can now call svc.Issue after OAuth/magic-link exchange; git smart-HTTP can call BasicAuthValidator for push auth.
