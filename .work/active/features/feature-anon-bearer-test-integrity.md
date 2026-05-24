---
id: feature-anon-bearer-test-integrity
kind: feature
stage: review
tags: [testing, tokens, migrations]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Anon-bearer test integrity

## Brief

Two tests under `feature-epic-ephemeral-playground-anon-bearer`
whose names lie about what they assert. Surfaced from review of
that feature (2026-05-23) under the test-integrity discipline in
CLAUDE.md: "A failing test that documents why it fails ... is more
honest than a green test that lies." Both green tests pass while
silently failing to exercise the acceptance criteria they're named
after.

## Why a feature

Both children share the same theme (mis-named tests on the same
feature) and need the same kind of work: re-name the existing tests
to describe what they actually assert, then add new tests that
exercise the original acceptance criteria. One feature gives the
work a coherent verdict and a single PR.

The migration story also lands a reusable `migrateDown` test helper
that future migration tests can use.

## Child stories

- `story-anon-bearer-test-integrity-transactional-rollback` — rename
  the misleading test to `_EmptySessionID_NoDBCalls` and add a real
  transactional-rollback test using the embedded-store-with-override
  pattern.
- `story-anon-bearer-test-integrity-migration-updownup` — extend the
  existing test body with the actual Up→Down→Up cycle the name
  promises; introduce a test-only `migrateDown(t, db, dialect, version)`
  helper in the same file.

The two stories are **independent** (different packages, different
files, different invariants under test). No `depends_on` chain — they
should land in parallel.

## Design decisions

- **`MigrateDown` helper home**: test-only helper in `internal/db/migrate_test.go`. Unexported, takes `testing.TB`, replicates the provider-creation block from `MigrateUp(ctx, db, dialect)` then calls `provider.DownTo(ctx, version int64)`. Cannot be called from production code. Confirms the feature body's recommendation. Future migration tests in the same file get reuse for free; cross-package reuse can come later if a second migration test needs it.
- **Rollback test interposition pattern**: embed real `store.Store`, override only `WithTx` to pass a wrapped `TxStore` whose `CreateAnonymousBearer` returns the injected error. Go's struct embedding handles the other ~20 sub-interface methods automatically. Lightest implementation — no full TxStore decorator, no driver-level shim. The pattern is introduced fresh in `internal/portal/tokens/anon_bearer_test.go`; future tests in the package needing similar interception can copy it.
- **Rename strategy for both misleading tests**: rename existing tests to describe what they actually do AND add new properly-named tests:
  - `TestIssueAnonymousSessionBearer_TransactionalRollback` → `TestIssueAnonymousSessionBearer_EmptySessionID_NoDBCalls` (preserves the no-DB-calls assertion that is its real value); add new `TestIssueAnonymousSessionBearer_TransactionalRollback` that exercises the bearer-insert-error rollback path with the embedded-store pattern.
  - `TestMigrate00016_AnonymousBearers_UpDownUp` → keep the name AND implement the real Up-Down-Up cycle in its body (the existing body is mostly post-Up shape assertions; extend with Down step + re-Up step). The existing assertions document the post-Up state, which is still valuable; no rename needed if the body actually does the cycle.
  - (The migration test essentially absorbs option-3 wording for itself, since its current body is salvageable. The bearer test follows the rename-and-add path.)

## Architectural choice

This is **test-quality work, not an architectural shift.** No new
packages, no new public surface, no new abstractions in production
code. The decisions are all about how to interpose on existing
production surfaces (`store.Store`, `MigrateUp`) from tests without
spreading test scaffolding into production code or growing the public
API.

Two test-local techniques carry the work:

1. **Struct-embedding override** for `store.Store` interception. Go's
   embedding means a wrapper struct that embeds the real `store.Store`
   automatically satisfies all ~20 sub-interfaces; tests only need to
   write the method they want to redirect. Same for `store.TxStore`.
   This stays test-local in `anon_bearer_test.go`. No production
   `MockStore` is added.
2. **Test-only `migrateDown` helper** that re-derives a goose
   `Provider` from the embedded migration FS and calls `DownTo`. Lives
   in `migrate_test.go`, unexported, takes `testing.TB`. Production
   `db.MigrateUp` is unchanged — the helper just mirrors its
   provider-construction logic, scoped to the test binary.

Rejected: exposing a public `MigrateDown` (invites prod misuse);
writing a full `TxStore` decorator (24 stub methods, churn for one
test); injecting an error at the SQL driver level (driver shims are
fragile and dialect-coupled).

## Implementation Units

### Unit 1: `migrateDown` test helper

**File**: `internal/db/migrate_test.go`
**Story**: `story-anon-bearer-test-integrity-migration-updownup`

```go
// migrateDown rolls back the database to (but not including) the given
// goose version. Test-only: takes testing.TB so it can only be called
// from a test binary. Replicates the provider construction in
// MigrateUp; calls Provider.DownTo, which goose documents as
// "rolls back all migrations down to, but not including, the
// specified version".
//
// To revert migration 00016, call migrateDown(t, db, "sqlite", 16) —
// goose DownTo(N) leaves the DB at version N-1.
func migrateDown(t testing.TB, ctx context.Context, db *sql.DB, dialect string, version int64) {
    t.Helper()

    var rawFS embed.FS
    var subDir string
    var gooseDialect goose.Dialect

    switch dialect {
    case "sqlite":
        rawFS = sqliteMigrations
        subDir = "migrations/sqlite"
        gooseDialect = goose.DialectSQLite3
    case "postgres":
        rawFS = postgresMigrations
        subDir = "migrations/postgres"
        gooseDialect = goose.DialectPostgres
    default:
        t.Fatalf("migrateDown: unknown dialect %q", dialect)
    }

    fsys, err := fs.Sub(rawFS, subDir)
    if err != nil {
        t.Fatalf("migrateDown: embed sub-FS (%s): %v", dialect, err)
    }
    provider, err := goose.NewProvider(gooseDialect, db, fsys)
    if err != nil {
        t.Fatalf("migrateDown: goose provider init (%s): %v", dialect, err)
    }
    if _, err := provider.DownTo(ctx, version); err != nil {
        t.Fatalf("migrateDown: provider.DownTo(%d): %v", version, err)
    }
}
```

**Implementation notes**:
- `sqliteMigrations` and `postgresMigrations` are package-level
  `embed.FS` values declared in `migrate.go`; tests in the same
  package can reference them directly.
- Provider construction is intentionally duplicated rather than
  factored out from `MigrateUp` — extracting a shared helper would
  expose internals just for one test. Re-derive locally; if a second
  test wants it, factor then.
- The helper uses `t.Fatalf` for all failure modes so callers get a
  fail-fast signature; there's no benefit returning an error a test
  would just `t.Fatal` on anyway.

**Acceptance Criteria**:
- [ ] Calling `migrateDown(t, ctx, db, "sqlite", 16)` after `MigrateUp`
      leaves the DB at goose version 15 (verifiable via
      `SELECT MAX(version_id) FROM goose_db_version`).
- [ ] Bad dialect (`"oracle"`) fails the test rather than no-op.
- [ ] Helper unavailable to production code — `internal/db/*.go`
      (non-`_test`) does not reference it.

---

### Unit 2: Extend `TestMigrate00016_AnonymousBearers_UpDownUp` body

**File**: `internal/db/migrate_test.go`
**Story**: `story-anon-bearer-test-integrity-migration-updownup`

After the existing post-Up data-insert block (which currently asserts
both `is_anonymous` on accounts and the new `anonymous_session_bearer`
kind work), the body extends with:

```go
// --- Down: roll back to before 00016 ---
migrateDown(t, ctx, db, "sqlite", 16)

// is_anonymous column removed.
_, err = db.ExecContext(ctx,
    `INSERT INTO accounts (id, email, display_name, created_at, is_anonymous)
     VALUES ('acc-test-002', 'test2@example.com', 'Test2', datetime('now'), 0)`)
if err == nil {
    t.Error("after Down: is_anonymous column should be gone but INSERT succeeded")
}

// session_id column removed from oauth_tokens; INSERT including it should fail.
_, err = db.ExecContext(ctx,
    `INSERT INTO oauth_tokens (id, account_id, token_hash, kind, session_id, issued_at, expires_at)
     VALUES ('tok-003', 'acc-test-001', 'hash-003', 'access', 'sess-001', datetime('now'), datetime('now', '+1 hour'))`)
if err == nil {
    t.Error("after Down: session_id column should be gone but INSERT succeeded")
}

// anonymous_session_bearer rows deleted (CHECK now rejects the kind).
var anonCount int
if err := db.QueryRowContext(ctx,
    `SELECT COUNT(*) FROM oauth_tokens WHERE kind='anonymous_session_bearer'`,
).Scan(&anonCount); err != nil {
    t.Fatalf("count anonymous_session_bearer after Down: %v", err)
}
if anonCount != 0 {
    t.Errorf("after Down: anonymous_session_bearer rows: want 0, got %d", anonCount)
}

// Pre-migration rows survived the Down.
var preCount int
if err := db.QueryRowContext(ctx,
    `SELECT COUNT(*) FROM oauth_tokens WHERE id='tok-001'`,
).Scan(&preCount); err != nil {
    t.Fatalf("count pre-migration token after Down: %v", err)
}
if preCount != 1 {
    t.Errorf("after Down: tok-001: want 1 row, got %d", preCount)
}

// --- Re-Up: reapply 00016 ---
if err := MigrateUp(ctx, db, "sqlite"); err != nil {
    t.Fatalf("MigrateUp (re-apply after Down): %v", err)
}

// is_anonymous column back; session_id column back; new kind accepted.
if _, err := db.ExecContext(ctx,
    `INSERT INTO oauth_tokens (id, account_id, token_hash, kind, session_id, issued_at, expires_at)
     VALUES ('tok-004', 'acc-test-001', 'hash-004', 'anonymous_session_bearer', 'sess-001', datetime('now'), datetime('now', '+1 hour'))`,
); err != nil {
    t.Errorf("after re-Up: anonymous_session_bearer insert: %v", err)
}
```

**Implementation notes**:
- The existing body's setup (org, session, pre-migration token, new-kind
  token) all stays — it's the precondition for the Down step.
- Order matters: assert Down deleted `anonymous_session_bearer` rows
  **before** running re-Up, otherwise the assertion proves nothing.
- "Insert should fail" checks rely on SQLite returning an error for
  unknown column names. We assert `err == nil` is the failure case
  rather than inspecting the error message (resilient to driver
  message changes).
- Postgres counterpart NOT included here. The 00016 Postgres Down is a
  straightforward `DROP COLUMN`/`ALTER TABLE`; if/when a SQLite-Postgres
  Down-parity test is needed, file a follow-up story.

**Acceptance Criteria**:
- [ ] Test name `TestMigrate00016_AnonymousBearers_UpDownUp` matches
      behaviour: body invokes Up, Down, then Up again.
- [ ] Down phase deletes all `anonymous_session_bearer` rows.
- [ ] Down phase removes `is_anonymous` from `accounts` and `session_id`
      from `oauth_tokens` (verified by INSERT failure).
- [ ] Pre-migration `access` tokens survive Down without data loss.
- [ ] Re-Up succeeds and the post-migration shape is restored.
- [ ] Test passes against SQLite (`go test ./internal/db/...`).

---

### Unit 3: Rename misnamed bearer test, add real rollback test

**File**: `internal/portal/tokens/anon_bearer_test.go`
**Story**: `story-anon-bearer-test-integrity-transactional-rollback`

Rename the existing test (lines 209-230) from `_TransactionalRollback`
to `_EmptySessionID_NoDBCalls`. The body stays as-is — it does
correctly assert the no-DB-calls behaviour:

```go
func TestIssueAnonymousSessionBearer_EmptySessionID_NoDBCalls(t *testing.T) {
    // Verifies that an empty sessionID is rejected by the pre-tx validation
    // guard, so no DB calls are made and no account row is created.
    // (Originally named _TransactionalRollback, which was a lie — empty
    // sessionID never reaches WithTx, so there's no transaction to roll
    // back. The real rollback test is below.)
    ctx := context.Background()
    s := openStore(t)
    svc := tokens.New(s)

    _, _, _, err := svc.IssueAnonymousSessionBearer(ctx, "", "fern-moth", 24*time.Hour)
    if err == nil {
        t.Fatal("expected error for empty sessionID, got nil")
    }
    rows, listErr := s.ListOAuthTokensForAccount(ctx, "anon_anything")
    if listErr != nil {
        t.Fatalf("ListOAuthTokensForAccount: %v", listErr)
    }
    if len(rows) != 0 {
        t.Errorf("expected 0 token rows after failed issue, got %d", len(rows))
    }
}
```

Note this introduces a name collision risk with the existing
`TestIssueAnonymousSessionBearer_EmptySessionID_Rejected` at line 243.
Resolution: the `_Rejected` test asserts the error surface; the new
`_NoDBCalls` test additionally asserts no rows were written. Two
distinct invariants, two distinct tests, both kept.

Then add the new properly-named test:

```go
// txStoreOverride embeds the real TxStore and overrides only
// CreateAnonymousBearer to return the injected error. Go's struct
// embedding satisfies all other ~20 sub-interface methods through
// the embedded *real* TxStore (the one passed by WithTx).
type txStoreOverride struct {
    store.TxStore
    bearerErr error
}

func (o *txStoreOverride) CreateAnonymousBearer(ctx context.Context, arg store.CreateAnonymousBearerParams) (store.OAuthToken, error) {
    return store.OAuthToken{}, o.bearerErr
}

// storeOverride embeds the real Store and overrides only WithTx to
// wrap the TxStore passed to fn. Same embedding trick — every other
// Store method delegates to the embedded real Store.
type storeOverride struct {
    store.Store
    bearerErr error
}

func (o *storeOverride) WithTx(ctx context.Context, fn func(store.TxStore) error) error {
    return o.Store.WithTx(ctx, func(tx store.TxStore) error {
        return fn(&txStoreOverride{TxStore: tx, bearerErr: o.bearerErr})
    })
}

func TestIssueAnonymousSessionBearer_TransactionalRollback(t *testing.T) {
    // Verifies that if bearer creation fails inside WithTx, the account
    // row created earlier in the same transaction is rolled back. Uses
    // the storeOverride pattern to inject a CreateAnonymousBearer error
    // while letting CreateAnonymousAccount proceed normally.
    ctx := context.Background()
    realStore, sessID := openStoreWithSession(t)

    injectErr := errors.New("synthetic bearer-insert failure")
    overlay := &storeOverride{Store: realStore, bearerErr: injectErr}
    svc := tokens.New(overlay)

    _, _, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "fern-moth", 24*time.Hour)
    if err == nil {
        t.Fatal("expected bearer-insert error, got nil")
    }
    if !errors.Is(err, injectErr) {
        t.Errorf("expected wrapped %v, got %v", injectErr, err)
    }

    // Confirm no anonymous account row was committed despite
    // CreateAnonymousAccount succeeding within the transaction.
    accounts, err := realStore.ListAnonymousAccounts(ctx)
    if err != nil {
        // Fallback: query by display name via a more general listing
        // surface if ListAnonymousAccounts doesn't exist. Story
        // implementation picks the right query.
        t.Fatalf("list anonymous accounts: %v", err)
    }
    for _, a := range accounts {
        if a.DisplayName == "fern-moth" {
            t.Errorf("rollback failed: anon account %s survived a failed bearer-insert", a.ID)
        }
    }
}
```

**Implementation notes**:
- The struct-embedding pattern (`type X struct { store.Store }`) auto-
  satisfies the interface; only the overridden method is rewritten. If
  the `Store` interface grows a method in the future, the override
  inherits it for free.
- `realStore.ListAnonymousAccounts` may not exist as named — the story
  implementation should pick the right read surface. If no domain
  query targets anon accounts directly, fall back to `GetAccountByID`
  with a deterministic id, or to a raw `SELECT COUNT(*) FROM accounts
  WHERE display_name='fern-moth'` via a test-local DB handle.
  **Acceptance criterion is unchanged**: a failed bearer insert must
  leave zero account rows for that display_name.
- `errors.Is` propagation depends on the service wrapping the inner
  error with `%w`. Inspecting `service_impl.go:240` confirms it does
  (`fmt.Errorf("create anon bearer: %w", err)`).
- The two override types are kept inside `anon_bearer_test.go` (not
  promoted to a shared helper). They're cheap to copy if a second test
  needs the pattern later.

**Acceptance Criteria**:
- [ ] `TestIssueAnonymousSessionBearer_TransactionalRollback` exercises
      the bearer-insert-error rollback path via the embedded-store
      pattern.
- [ ] Injected `bearerErr` propagates back to the caller via `errors.Is`.
- [ ] After the failed call, zero account rows exist with
      `display_name='fern-moth'`.
- [ ] Renamed `TestIssueAnonymousSessionBearer_EmptySessionID_NoDBCalls`
      preserves the original no-DB-calls assertion.
- [ ] Existing `TestIssueAnonymousSessionBearer_EmptySessionID_Rejected`
      is untouched and still passes (distinct invariant: the error
      surface).
- [ ] Full token-package test suite green:
      `go test ./internal/portal/tokens/...`.

---

## Implementation Order

The two stories touch independent files (`internal/db/migrate_test.go`
vs `internal/portal/tokens/anon_bearer_test.go`) and assert
independent invariants. They can land in parallel.

1. **In parallel**:
   - `story-anon-bearer-test-integrity-migration-updownup` (Units 1 + 2)
   - `story-anon-bearer-test-integrity-transactional-rollback` (Unit 3)
2. Verify after both land: `go test ./internal/db/... ./internal/portal/tokens/...`
3. Postgres pass (if `JAMSESH_TEST_PG_DSN` available): same command
   with the env var set, to confirm the migration test on
   the SQLite path doesn't accidentally regress Postgres callers.

## Testing

**The work IS the tests.** Verification:

### Migration story (`internal/db/migrate_test.go`)
- `go test -run TestMigrate00016_AnonymousBearers_UpDownUp ./internal/db/...`
  must pass.
- Manual smoke: temporarily edit a 00016 Down SQL statement to a known-bad
  syntax; the test must fail. (Don't commit the edit.) Confirms the test
  actually exercises Down.
- The new `migrateDown` helper is only exercised through this test; its
  acceptance is the test passing.

### Rollback story (`internal/portal/tokens/anon_bearer_test.go`)
- `go test -run TestIssueAnonymousSessionBearer ./internal/portal/tokens/...`
  must pass — all 9 tests in the suite (the rename + add + 7 originals).
- Manual smoke: temporarily change the bearer-insert error wrap in
  `service_impl.go:240` from `%w` to `%v` (drops the error wrap); the
  test's `errors.Is` assertion must fail. (Don't commit.) Confirms the
  test actually exercises rollback.

### Cross-cutting
- After both stories: `go test ./...` from repo root, ensuring no
  unrelated package broke.
- Visual: `grep -rn "_TransactionalRollback\|_UpDownUp" internal/` —
  the only matches should be the renamed/extended tests, and the body
  of each must visibly do what the name says.

## Risks

- **SQLite Down migration on partial data**. The 00016 Down statement
  deletes `anonymous_session_bearer` rows before rebuilding
  `oauth_tokens`. The test seeds both pre- and post-migration rows; if
  the Down SQL deletes more than intended (e.g., a future Down rewrite
  bug), the test catches it via the "tok-001 survives" assertion.
- **Embedded-store pattern may shadow an unexpected `Store` method**.
  Go embedding is conservative — if `Store` grows a method that
  conflicts with a future `*storeOverride` field/method, the compiler
  flags it. Low risk; mitigated by keeping the override minimal.
- **`ListAnonymousAccounts` may not exist**. The unit 3 acceptance
  criterion is "zero accounts with display_name='fern-moth' after
  rollback"; the exact query is the story implementer's choice. If no
  domain-level listing exists, the story can use a raw SQL probe via
  the underlying `*sql.DB` (test-local, fine) or add a one-line
  `GetAccountByEmail`-style read.
- **Postgres parity gap**. The migration test runs SQLite-only;
  Postgres Down is presumed correct but not exercised. If a future
  Down bug is dialect-specific, this test won't catch it. Out of
  scope for this feature — file a follow-up if/when it matters.
- **Race with cleanup**. `openStore` cleans up via `t.Cleanup` (closing
  the in-memory DB). Both stories use this; no concurrent goroutines
  are introduced, so no race window.

## Mockups

No UI surface — pure test-suite work.

## Acceptance (rollup)

- Both children at stage:done with verdicts >= approve
- No test in the package has a name that lies about what it asserts
- `TestIssueAnonymousSessionBearer_TransactionalRollback` actually
  exercises a bearer-insert-error rollback via the embedded-store pattern
- `TestMigrate00016_AnonymousBearers_UpDownUp` actually runs Up -> Down -> Up
- `migrateDown` test helper available in `internal/db/migrate_test.go`
  for future migration tests

## Implementation summary (2026-05-23)

Both child stories landed in parallel (different packages, different files,
different invariants — no inter-dependency) and are at stage:review.

1. `story-anon-bearer-test-integrity-migration-updownup` (Units 1+2) —
   added unexported `migrateDown(testing.TB, ctx, *sql.DB, dialect, version)`
   helper to `internal/db/migrate_test.go`; extended
   `TestMigrate00016_AnonymousBearers_UpDownUp` body with the real Down +
   re-Up cycle. **Goose semantics correction:** design said `DownTo(16)`
   would leave the DB at version 15, but goose v3.27.1's `DownTo(N)`
   rolls back versions strictly > N, leaving the DB at N. Resolved by
   calling `migrateDown(t, ctx, db, "sqlite", 15)` and updating the
   helper's doc comment so future callers don't trip on the same
   misconception. This also cleanly rolls back 17 and 18 (org_protected,
   playground_sessions) along with 16 — intentional because 00018's
   oauth_tokens.session_id FK depends on the column 00016 introduces.

2. `story-anon-bearer-test-integrity-transactional-rollback` (Unit 3) —
   renamed `TestIssueAnonymousSessionBearer_TransactionalRollback` (body
   intact) to `_EmptySessionID_NoDBCalls` so the test name matches what
   it actually asserts; added the embedded-store override types
   (`txStoreOverride` + `storeOverride`); added the real
   `TestIssueAnonymousSessionBearer_TransactionalRollback` test that
   injects a bearer-insert error via the override and asserts both
   `errors.Is` propagation AND zero account rows with the test's
   display_name. The post-rollback assertion uses a raw `SELECT
   COUNT(*)` via the underlying `*sql.DB` (which `db.Open` already
   returns alongside the store) — no need to add a domain-level query
   to the production interface.

### Verification

- `go test ./internal/db/... ./internal/portal/tokens/...` → all green
- `go test ./...` → all green across the whole repo
- `migrateDown` is test-only and unavailable to production:
  `grep -n migrateDown internal/db/*.go` only returns matches in
  `migrate_test.go`.
- No mis-named tests remain in either package: `TestIssueAnonymousSessionBearer_TransactionalRollback`
  exercises real rollback, `_EmptySessionID_NoDBCalls` exercises the
  no-DB-calls invariant, `_EmptySessionID_Rejected` exercises the error
  surface (three distinct invariants, three distinct tests). The
  migration test's body matches its name (Up → Down → Up).
