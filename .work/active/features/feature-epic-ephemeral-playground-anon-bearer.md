---
id: feature-epic-ephemeral-playground-anon-bearer
kind: feature
stage: review
tags: [portal, security]
parent: epic-ephemeral-playground
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Anonymous session-scoped bearer tokens

## Brief

Extends the existing `oauth_tokens` table to carry anonymous
session-scoped bearers — the auth substrate that makes ephemeral
playground identities work without touching the OAuth flow. Adds an
`anonymous_session_bearer` value to the existing `kind` column and a
nullable `session_id` foreign key. Anonymous identities also get an
`accounts` row marked `is_anonymous: true` so the existing
`session_members.account_id` FK and `RequireSessionMember` middleware
work unchanged. The bearer plugs into the bearer middleware, MCP's
`verifyToken`, and the git Basic-auth resolver without per-call-site
branching — handlers don't differentiate identity kind, only membership.

The token service grows one new method:
`IssueAnonymousSessionBearer(ctx, sessionID, nickname) (string, error)`.
Validation reuses `tokens.Validate` unchanged. Revocation happens
implicitly during session destruction (the destruction sweep sets
`oauth_tokens.revoked_at` for every bearer with the session_id before
deleting the session row).

This feature is auth-substrate only — it does NOT create playground
sessions or issue bearers in response to API calls. That's
`session-lifecycle`. This feature ships the primitive; the lifecycle
feature wires it up.

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 1 foundation** — no dependencies; required by
  `session-lifecycle` (wave 2) for the bearer-issuance step in
  playground session creation and joiner accept.

## Foundation references
- `docs/SPEC.md` § Auth model — the anonymous-bearer bullet added at
  scope time describes the contract this feature implements
- `docs/ARCHITECTURE.md` § Data layer — `oauth_tokens` and `accounts`
  table shapes; this feature is the first migration touching them since
  the epic's scope work
- `docs/SECURITY.md` — anon-bearer threat model addendum (token leak
  scope is session-bounded, no cross-session blast radius) is owned by
  this feature's design pass

## Mockups
No UI surface — substrate-only feature. The parent epic's flow mocks
cover everything user-visible.

## Design decisions

Locked at `--only-questions` time. Feature-design Phase 5 inherits these
as fixed input.

- **Bearer expiry mechanism**: belt-and-suspenders. Bearer carries
  `expires_at` synced to the session's hard-cap deadline (24h from
  creation by default; in lockstep with the session's
  `JAMSESH_PLAYGROUND_HARD_CAP` setting). `tokens.Validate` rejects
  expired bearers automatically via existing logic. The destruction
  sweep ALSO sets `revoked_at` on every bearer for the destroyed
  session as a second layer. If the sweep is delayed, TTL still kicks
  in; if TTL math drifts, sweep cleans up. Either failure mode is
  benign — both have to fail simultaneously for bearers to outlive
  their session.

- **Anonymous `accounts` row shape**: one row per anonymous session
  participant. Schema: `accounts.id` = `anon_<random>`, `is_anonymous:
  true` (new boolean column), `email: NULL`, `display_name` =
  server-minted nickname. 1:1 with `session_members.account_id` —
  every account-joined query, commit-attribution path, and addressed-
  comment lookup works unchanged. Storage cost is bounded by max
  concurrent participants × active session count (negligible).

- **Nickname storage**: `accounts.display_name`. Single source of
  truth; reuses the existing column populated for durable accounts.
  Joiner-rename at the nickname-picker surface is one UPDATE. Both
  the presence panel and `@<nickname>` addressing lookups read from
  the same column for both durable and anonymous identities — no
  identity-kind branching needed.

- **Anonymous account cleanup on session destruction**: cascade-delete
  with the session. The destruction routine sequence becomes:
  1. Revoke all bearers for the session (set `oauth_tokens.revoked_at`)
  2. Delete `comments` and `conflict_events` for the session
  3. Delete the `sessions` row (FK CASCADE handles `session_members`,
     `events`, `presence`)
  4. Delete the anonymous `accounts` rows that joined this session
     (identified by `is_anonymous: true` and a JOIN against the
     just-deleted `session_members` rows captured pre-delete)
  5. Delete the bare repo on disk under `<storage>/orgs/playground/sessions/<id>.git`
  
  Step 4's identification depends on capturing the to-delete account
  IDs before step 3 (since the FK cascade removes the
  `session_members` rows by then). Implementation note: the
  destruction routine collects `account_id` list at the top of the
  transaction, applies cascades, then deletes the captured account
  IDs. Anonymous accounts never participate in another session
  (per-session-row decision above), so the cleanup is safe and
  consistent with the strict-ephemeral commitment.

  **Note: the destruction routine itself is owned by the
  `session-lifecycle` feature.** This feature provides the substrate
  primitives (the schema + the issuance method) and verifies that the
  cascade-delete sequence works correctly via integration tests; the
  orchestration of when destruction fires lives in session-lifecycle.

## Architectural choice

**Extend existing `tokens.Service` interface with one new method;
preserve `Validate`'s signature so every existing caller (REST bearer
middleware, MCP `verifyToken`, git Basic-auth resolver) works
unchanged.** Anonymous accounts are real `accounts` rows with
`is_anonymous: true` and a synthetic email; anonymous bearers are real
`oauth_tokens` rows with a new `kind` value and a nullable `session_id`
FK. The data layer enforces cascade behavior; the service layer is a
thin issuance helper.

Why over alternatives:
- **Parallel `anonymous_tokens` table**: forks `Validate` into two
  resolution paths, requires middleware updates everywhere. Anti-pattern
  per the locked design decision (single resolution path).
- **NULL `account_id` on `oauth_tokens` + NULL `session_members.account_id`**:
  breaks every account-joined query in the codebase. Anti-pattern per the
  locked design decision.
- **Reuse existing `IssueShortLived`**: doesn't support session_id binding
  or anonymous-account creation in one transaction. Adding parameters to
  it would muddy the existing OAuth flow's contract. New method is cleaner.

The synthetic-email approach (`anon-<random>@playground.local`) handles
the `accounts.email NOT NULL UNIQUE` constraint without a schema
rebuild. SQLite's `ALTER TABLE` is restrictive (can't drop NOT NULL
without a table-rebuild dance); synthetic emails sidestep the dance
entirely while preserving the UNIQUE invariant (random suffix guarantees
no collisions).

## Implementation units

This is a single tight-cohesion substrate change. **No child stories
spawned** — the design body IS the implementation guide for the single
implementing agent. The work is sequenced (schema → sqlc regen →
service method → tests → docs), each step depending on the prior.

### Unit 1: Goose migration (per dialect)
**File**: `internal/db/migrations/sqlite/NNNN_anonymous_bearers.sql`
       + `internal/db/migrations/postgres/NNNN_anonymous_bearers.sql`
(NNNN = next sequence number after the latest existing migration)

**SQLite** (table-rebuild dance for the `oauth_tokens.kind` CHECK
constraint update; `accounts.is_anonymous` is a simple ALTER ADD):

```sql
-- +goose Up
ALTER TABLE accounts
  ADD COLUMN is_anonymous INTEGER NOT NULL DEFAULT 0;

-- Rebuild oauth_tokens to update CHECK constraint and add session_id FK.
-- SQLite can't ALTER a CHECK constraint or add a FK without rebuilding.
CREATE TABLE oauth_tokens_new (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('access','refresh','anonymous_session_bearer')),
    session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
    issued_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    last_used_at DATETIME,
    revoked_at DATETIME
);

INSERT INTO oauth_tokens_new (id, account_id, token_hash, kind, session_id,
                              issued_at, expires_at, last_used_at, revoked_at)
SELECT id, account_id, token_hash, kind, NULL,
       issued_at, expires_at, last_used_at, revoked_at
  FROM oauth_tokens;

DROP TABLE oauth_tokens;
ALTER TABLE oauth_tokens_new RENAME TO oauth_tokens;

CREATE INDEX oauth_tokens_account_idx ON oauth_tokens(account_id);
CREATE INDEX oauth_tokens_session_idx ON oauth_tokens(session_id)
  WHERE session_id IS NOT NULL;

-- +goose Down
-- (Reverse migration: drop is_anonymous, rebuild oauth_tokens without
--  session_id and without the new kind value. Implementer writes the
--  symmetric down migration.)
```

**Postgres** (cleaner — ALTER TABLE supports the operations directly):

```sql
-- +goose Up
ALTER TABLE accounts
  ADD COLUMN is_anonymous BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE oauth_tokens
  ADD COLUMN session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE;

ALTER TABLE oauth_tokens
  DROP CONSTRAINT oauth_tokens_kind_check,
  ADD CONSTRAINT oauth_tokens_kind_check
    CHECK (kind IN ('access', 'refresh', 'anonymous_session_bearer'));

CREATE INDEX oauth_tokens_session_idx ON oauth_tokens(session_id)
  WHERE session_id IS NOT NULL;

-- +goose Down
-- (Reverse migration: drop session_id column, restore original kind
--  CHECK, drop is_anonymous column. Implementer writes the symmetric.)
```

**Important — also update the source-of-truth schema files** that sqlc
reads from:
- `db/schema/sqlite.sql` — update `accounts` and `oauth_tokens` table
  definitions to reflect the post-migration state
- `db/schema/postgres.sql` — same

sqlc generates Go types from `db/schema/`, not from migrations. If the
schema files drift from the migrations, generated types will be wrong.

**Acceptance criteria**:
- [ ] Goose `up` migration applies cleanly against fresh + existing DB on both dialects
- [ ] Goose `down` migration reverses cleanly (verify in a test)
- [ ] `db/schema/{sqlite,postgres}.sql` updated to match
- [ ] sqlc regenerates without errors after schema update
- [ ] Existing OAuth token rows survive the migration (data-preservation
      test: insert a token, run migration, verify token still validates)

---

### Unit 2: sqlc query for anonymous account creation
**File**: `db/queries/sqlite/accounts.sql` + `db/queries/postgres/accounts.sql`
(dual-dialect mirror per `dual-dialect-mirror-queries` pattern)

```sql
-- name: CreateAnonymousAccount :one
-- Creates an anonymous account for a playground session participant.
-- The synthetic email satisfies the NOT NULL UNIQUE constraint without
-- requiring schema relaxation; the @playground.local suffix and the
-- random ID prefix guarantee uniqueness.
INSERT INTO accounts (id, email, display_name, github_user_id, created_at, is_anonymous)
VALUES (?, ?, ?, NULL, ?, 1)
RETURNING *;
```

(Postgres variant uses `$1, $2, $3, $4` placeholders + `TRUE` instead of `1`.)

The caller (token service) generates:
- `id` = `"anon_" + cryptorand.URLSafe(16)` (matches existing account-ID
  format conventions; `cmd/jamsesh/sessioncmd/` or similar may already
  have a helper)
- `email` = `id + "@playground.local"` (e.g.,
  `anon_abc123def456@playground.local`)
- `display_name` = the server-minted nickname (e.g., `amber-otter`)
- `created_at` = `time.Now().UTC()` (or via the per-package clock
  interface per the project's `per-package-clock-interface` pattern)

**Acceptance criteria**:
- [ ] sqlc generates `CreateAnonymousAccount` method on the Querier
      interface in both dialects
- [ ] Method round-trips correctly via the `stores(t)` test harness
- [ ] Generated `Account` struct exposes `IsAnonymous bool` field
- [ ] `GetAccountByID` (existing) returns anonymous accounts unchanged
      with `IsAnonymous: true`

---

### Unit 3: sqlc query for anonymous bearer creation
**File**: `db/queries/sqlite/oauth_tokens.sql` + postgres mirror

```sql
-- name: CreateAnonymousBearer :one
-- Inserts an anonymous-session-scoped bearer row. The session_id FK
-- ensures the bearer is destroyed when the session is destroyed
-- (ON DELETE CASCADE). expires_at is set by the caller, typically to
-- the session's hard-cap deadline.
INSERT INTO oauth_tokens (id, account_id, token_hash, kind, session_id,
                          issued_at, expires_at)
VALUES (?, ?, ?, 'anonymous_session_bearer', ?, ?, ?)
RETURNING *;

-- name: RevokeBearersForSession :exec
-- Marks every bearer (any kind) associated with a session as revoked.
-- Used by the session destruction routine in session-lifecycle feature
-- as the first step of the cascade (revoke bearers → delete dependent
-- rows → delete session row → cascade).
UPDATE oauth_tokens
   SET revoked_at = ?
 WHERE session_id = ?
   AND revoked_at IS NULL;
```

**Acceptance criteria**:
- [ ] sqlc generates both methods in both dialects
- [ ] `CreateAnonymousBearer` inserts a row with kind='anonymous_session_bearer'
- [ ] `RevokeBearersForSession` is idempotent (running it twice with the
      same `revoked_at` clock value: second call updates 0 rows)
- [ ] Cascade test: deleting the parent `sessions` row also deletes the
      `oauth_tokens` row (verified via integration test through the
      store layer, both dialects)

---

### Unit 4: `tokens.Service.IssueAnonymousSessionBearer` method
**File**: `internal/portal/tokens/service.go` (interface) +
         `internal/portal/tokens/service_impl.go` (implementation)

**Interface addition** (`service.go`):

```go
type Service interface {
    // ... existing methods unchanged ...
    Issue(ctx context.Context, accountID string) (Pair, error)
    IssueShortLived(ctx context.Context, accountID string, ttl time.Duration) (string, time.Time, error)
    Validate(ctx context.Context, raw string) (*store.Account, error)
    Refresh(ctx context.Context, raw string) (Pair, error)
    Revoke(ctx context.Context, ...) error

    // IssueAnonymousSessionBearer creates a fresh anonymous account row
    // (with synthetic email) and a session-scoped bearer for it. The
    // bearer's expires_at is set to now+ttl; the caller (typically the
    // playground session-creation handler) computes ttl from the session's
    // hard-cap deadline.
    //
    // The returned rawToken is the unhashed token to return to the
    // client; the database stores only its hash. The accountID is the
    // generated anon_<random> id; the caller persists session_members
    // and any other per-session state.
    IssueAnonymousSessionBearer(ctx context.Context, sessionID, nickname string, ttl time.Duration) (rawToken, accountID string, expiresAt time.Time, err error)
}
```

**Implementation** (`service_impl.go`):

```go
func (s *service) IssueAnonymousSessionBearer(ctx context.Context, sessionID, nickname string, ttl time.Duration) (string, string, time.Time, error) {
    if nickname == "" {
        return "", "", time.Time{}, errors.New("nickname must not be empty")
    }
    if sessionID == "" {
        return "", "", time.Time{}, errors.New("sessionID must not be empty")
    }

    accountID := "anon_" + randID(16) // crypto/rand-backed; matches existing ID conventions
    email := accountID + "@playground.local"
    now := s.clock.Now().UTC()

    // Transactional: account row + bearer row in one TX. If either
    // fails, both roll back. (Use the store's WithTx pattern per
    // `tx-emit-then-fanout` skill — verify whether tokens.Service
    // already has a WithTx wrapper; if not, this is a thin wrapper
    // around store.WithTx.)
    var rawToken string
    var expiresAt time.Time

    err := s.store.WithTx(ctx, func(q store.Querier) error {
        _, err := q.CreateAnonymousAccount(ctx, store.CreateAnonymousAccountParams{
            ID:          accountID,
            Email:       email,
            DisplayName: nickname,
            CreatedAt:   now,
        })
        if err != nil { return fmt.Errorf("create anon account: %w", err) }

        rawToken = generateToken() // reuse the existing token generator
        hash := hashToken(rawToken)
        expiresAt = now.Add(ttl)

        _, err = q.CreateAnonymousBearer(ctx, store.CreateAnonymousBearerParams{
            ID:        "tok_" + randID(16),
            AccountID: accountID,
            TokenHash: hash,
            SessionID: sql.NullString{String: sessionID, Valid: true},
            IssuedAt:  now,
            ExpiresAt: expiresAt,
        })
        if err != nil { return fmt.Errorf("create anon bearer: %w", err) }
        return nil
    })
    if err != nil { return "", "", time.Time{}, err }

    return rawToken, accountID, expiresAt, nil
}
```

**Implementation notes**:
- `randID(n)` and `generateToken()`/`hashToken()` already exist in the
  package (used by `Issue` and `IssueShortLived`). Reuse.
- `s.clock` is the per-package clock interface (per the project's
  `per-package-clock-interface` pattern); tests inject a fake clock to
  pin `now` for deterministic expiration assertions.
- The bearer creation uses the same `generateToken` + `hashToken` pair
  the existing `Issue` flow uses, so `Validate` resolves anonymous
  bearers via the same hash lookup. No `Validate` changes needed.
- `store.WithTx` — verify the wrapper signature in
  `internal/db/store/store.go`. Pattern is established for other
  multi-row operations.

**Acceptance criteria**:
- [ ] `IssueAnonymousSessionBearer("sess1", "amber-otter", 24h)` returns
      a non-empty rawToken, an `anon_*` accountID, an expiresAt 24h in
      the future
- [ ] After issuance, `Validate(ctx, rawToken)` returns the new
      `*store.Account` with `IsAnonymous: true`, `DisplayName: "amber-otter"`
- [ ] After issuance, `Validate(ctx, rawToken)` updates `last_used_at`
      (existing behavior, regression-tested)
- [ ] After TTL expires, `Validate(ctx, rawToken)` returns the
      `ErrTokenExpired` typed error (existing behavior — no anon-specific
      branch)
- [ ] After explicit revocation (set `revoked_at` directly via store),
      `Validate(ctx, rawToken)` returns the `ErrTokenRevoked` typed error
- [ ] Transactional rollback: if account creation succeeds but bearer
      creation fails (e.g., via a wrapping store injecting an error), no
      account row is left behind
- [ ] Empty nickname or sessionID returns a clear error pre-transaction
      (no DB calls made)

---

### Unit 5: BasicAuth resolver compatibility check (regression test)
**File**: `internal/portal/tokens/basic_test.go` (extend)

The existing `BasicAuthValidator` (in `basic.go`) calls `Validate`
under the hood. Anonymous bearers go through the same path. Add a
regression test confirming:

- An anonymous bearer issued via `IssueAnonymousSessionBearer` is
  accepted by `BasicAuthValidator` (used for git smart-HTTP auth)
- The returned `*store.Account` has `IsAnonymous: true`

This is a 1-test regression to catch any future refactor that
accidentally branches on identity kind in the BasicAuth path.

**Acceptance criteria**:
- [ ] Test asserts an anonymous bearer authenticates a git push
      (constructed via httptest, see existing basic_test.go patterns)

---

### Unit 6: Bearer middleware compatibility check (regression test)
**File**: `internal/portal/tokens/middleware_test.go` (extend)

Same shape as Unit 5, for `BearerMiddleware`:

- Issue an anonymous bearer via `IssueAnonymousSessionBearer`
- Make a request with `Authorization: Bearer <rawToken>` through the
  middleware
- Assert: the middleware injects the anonymous `*store.Account` into
  context; downstream handler can read it via
  `tokens.AccountFromContext(ctx)`

**Acceptance criteria**:
- [ ] Test asserts an anonymous bearer authenticates a REST request
- [ ] `AccountFromContext` returns the anonymous account with
      `IsAnonymous: true`

---

### Unit 7: Documentation roll-forward
**Files**: `docs/SECURITY.md`, `docs/PROTOCOL.md`

**`docs/SECURITY.md`** — add a section under the auth-model area:

```markdown
## Anonymous session-scoped bearers

When playground sessions are enabled (`JAMSESH_PLAYGROUND_ENABLED=true`),
participants are issued **anonymous session-scoped bearers**: a new
`accounts` row marked `is_anonymous: true` (with a synthetic
`anon-<random>@playground.local` email) and an `oauth_tokens` row with
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
```

**`docs/PROTOCOL.md`** — add a paragraph in the addressing section
noting that anonymous handles use the same `@<nickname>` form as
durable accounts. The addressing layer doesn't distinguish identity
kind — it looks up by `accounts.display_name` for the session, which
works the same for both.

**Acceptance criteria**:
- [ ] `docs/SECURITY.md` has an "Anonymous session-scoped bearers"
      section under the auth-model area
- [ ] `docs/PROTOCOL.md` mentions anonymous handles in the addressing
      section (one paragraph)
- [ ] Both docs read cleanly as part of their surrounding sections
      (the rolling-foundation principle: present tense, no "previously"
      language)

---

## Implementation order

Sequential — each step depends on the prior:

1. **Unit 1**: Goose migration (both dialects) + `db/schema/*.sql` update
2. **Unit 2 + 3**: sqlc queries (run `sqlc generate` after schema update)
3. **Unit 4**: `tokens.Service.IssueAnonymousSessionBearer` (depends on
   Units 2 + 3's generated Querier methods)
4. **Unit 5 + 6**: Regression tests for BasicAuth + Bearer middleware
   (depend on Unit 4's issuance method to mint test bearers)
5. **Unit 7**: Documentation roll-forward (can be done anytime;
   recommended last so it reflects the actually-implemented behavior)

No fan-out — single implementing agent walks the sequence.

## Testing

All tests live alongside production code per the project's idiom
(`internal/portal/tokens/*_test.go`, `internal/db/store/*_test.go`).
Multi-dialect via the `stores(t)` harness per the project's
`dual-dialect-mirror-queries` pattern.

**Unit tests**:
- `internal/db/store/anonymous_account_test.go` (new) — round-trip
  CreateAnonymousAccount + GetAccountByID via `stores(t)`
- `internal/db/store/anonymous_bearer_test.go` (new) — CreateAnonymousBearer
  + cascade-delete via session deletion
- `internal/portal/tokens/anon_bearer_test.go` (new) — full
  IssueAnonymousSessionBearer flow: returns valid rawToken, persists
  rows, Validate accepts the new bearer, expired bearers rejected,
  revoked bearers rejected
- `internal/portal/tokens/basic_test.go` (extend) — Unit 5 regression
- `internal/portal/tokens/middleware_test.go` (extend) — Unit 6 regression

**Migration test**:
- `internal/db/migrate_test.go` (extend) — apply Up, then Down, then Up
  again. Verify existing data survives. Run on both dialects (Postgres
  conditional on `JAMSESH_TEST_PG_DSN` env var).

## Risks

- **SQLite table-rebuild dance for the kind CHECK constraint**: SQLite
  doesn't support ALTER CHECK; the rebuild is well-trodden but easy to
  get wrong (forget to recreate indexes, drop the data accidentally,
  miss a column). Mitigation: the migration test (Unit 1's
  data-preservation acceptance criterion) verifies survival of pre-existing
  OAuth tokens by round-tripping a sample row through migrate-up.

- **`db/schema/*.sql` drift from migrations**: sqlc reads from
  `db/schema/`, not from migrations. If the schema file is updated but
  a migration step is missing (or vice versa), generated types diverge
  from runtime schema. Mitigation: the migration's acceptance criterion
  explicitly calls out updating both; verify by running `sqlc generate`
  + `make build` + a smoke test of `IssueAnonymousSessionBearer` post-
  migration.

- **Postgres `pg_advisory_lock` interaction with the migration**: the
  migration takes the advisory lock per `migrate.go`'s contract.
  Rebuilding `oauth_tokens` (if Postgres also needs a rebuild, though
  the design uses ALTER for Postgres) under the lock is safe but slow
  on tables with many rows. Mitigation: Postgres path uses ALTER not
  rebuild, so this risk is SQLite-only. SQLite production instances are
  typically small-tenant (per the deployment-shape constraint), so a
  full table rewrite is cheap (sub-second for <10k rows).

- **Synthetic email collision with a real user**: a user can't sign up
  with `@playground.local` because email validation in the existing
  magic-link flow likely accepts any well-formed email. If someone
  legitimately tries to use `someone@playground.local`, they collide
  with an anonymous account synthetic. Mitigation: register
  `playground.local` as a reserved suffix in the magic-link
  email-validation path (small handler change). Track as a discovered
  side-quest — log in the implementation notes when the discovery
  hits in `internal/portal/auth/magic_link.go`.

- **Transaction rollback on bearer-creation failure**: if the account
  row is committed but the bearer row fails to insert, we leave a stale
  anonymous account. The `WithTx` wrapper handles this — confirm the
  package uses `store.WithTx` for the issuance flow (not separate calls).
  Acceptance criterion in Unit 4 covers this.

- **`IsAnonymous` field on `Account` struct affects every caller**:
  adding a field to the sqlc-generated `Account` struct doesn't break
  existing callers (zero value is `false` for booleans). But any code
  that constructs `Account` literals in tests will need to add the new
  field. Sweep test files for `Account{...}` literals during
  implementation; existing patterns mostly use factory functions
  (`mustCreateAccount`) which absorb the change automatically.

## Implementation notes

All 7 units implemented. The implementation differed from the design guide in
the following noteworthy ways:

**SQLite unicode arrow truncation bug**: SQL comments containing the unicode
arrow character `→` (U+2192) caused sqlc to truncate the query body —
`AND revoked_at IS NULL` became `AND revoked_at I`. Replaced all unicode
arrows in SQL comments with plain ASCII equivalents. This is an undocumented
sqlc limitation; worth noting for any future SQL with non-ASCII comment
characters.

**Inline stub for handlerauth test**: `internal/portal/handlerauth/handlerauth_test.go`
contains a `stubStore` that satisfies `store.Store`. Adding `CreateAnonymousAccount`,
`CreateAnonymousBearer`, and `RevokeBearersForSession` as panic stubs was
required to keep the compile-time interface check passing. This is test debt
debt inherent to the blanket-stub pattern — each new store method requires
touching every existing stub.

**`@playground.local` suffix not reserved in magic-link validation**: confirmed
during Unit 7 that `internal/portal/auth/magic_link.go` accepts any
well-formed email with no domain-suffix filtering. A user could register
`anon_<anything>@playground.local` via magic-link and potentially collide
with a synthetic anonymous-account email. This is a real (if low-probability)
issue. Parked as follow-up item (see backlog).

**WithTx parameter type**: the feature design guide showed `func(store.Querier)
error` as the WithTx callback type but the actual interface is
`func(store.TxStore) error`. Used `TxStore` throughout.

**randID implementation**: no existing randID helper was found in the tokens
package. Implemented `randID(n int) (string, error)` using `crypto/rand` +
`encoding/hex` as a private helper, consistent with the token generation
approach already used for `generateToken()`.
