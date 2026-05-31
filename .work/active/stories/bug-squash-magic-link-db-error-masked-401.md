---
id: bug-squash-magic-link-db-error-masked-401
kind: story
stage: review
tags: [bug, portal, error-handling, high]
parent: epic-bug-squash-handler-error-classification
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: high
bug_domain: error-handling
bug_location: internal/portal/auth/magic_link.go:174
---

# Magic-link token-consume DB error is masked as a 401 "already used"

**Location**: `internal/portal/auth/magic_link.go:174` · **Severity**: high · **Pattern**: transient error treated as permanent / errors.Is wrong sentinel

`ConsumeMagicLinkToken` is an `UPDATE ... WHERE id = ? AND used_at IS NULL`; a concurrent race-loser updates 0 rows but returns **no error**, so the documented "won the race" case never reaches this branch. The only way `err != nil` here is a genuine transient DB failure (connection drop, deadlock, timeout) — which is then reported to the user as a permanent 401 "magic link already used", so a valid unused link becomes unusable with no retry. Every other handler in the file routes such errors through `deperr.WrapDBIfTransient`. Fix: distinguish "0 rows affected" (race → 401) from a real driver error (wrap as transient → 503), e.g. via `:execrows` or a re-read of `used_at`.

```go
if err := h.store.ConsumeMagicLinkToken(ctx, ...); err != nil {
    return magicLinkUnauthorized("auth.invalid_token", "magic link already used"), nil
}

## Implementation notes

Changed `ConsumeMagicLinkToken` from `:exec` to `:execrows` in both
`db/queries/sqlite/magic_link_tokens.sql` and
`db/queries/postgres/magic_link_tokens.sql`. Updated generated sqlc files
(`internal/db/sqlitestore/magic_link_tokens.sql.go`,
`internal/db/pgstore/magic_link_tokens.sql.go`, and both `querier.go` files)
to return `(int64, error)`. Updated `MagicLinkTokenStore` interface, both
dialect adapters (including `sqliteTxStore` / `postgresTxStore`), and
`ExchangeMagicLink` in `magic_link.go` to classify: `err != nil` → 5xx via
`deperr.WrapDBIfTransient`, `affected == 0` → 401, `affected == 1` → proceed.

Also fixed two pre-existing test call sites (`crud_test.go`, `store_test.go`)
that used `err = s.ConsumeMagicLinkToken(...)` (now returns two values).

Tests added in `magic_link_consume_test.go`:
- `TestExchangeMagicLink_ConsumeDriverError_Returns5xx` — driver error → 5xx
- `TestExchangeMagicLink_ConsumeZeroRows_Returns401` — 0-rows → 401
- `TestExchangeMagicLink_ConsumeOneRow_Succeeds` — 1-row → 200
- `TestExchangeMagicLink_ConcurrentExchangeSingleUse` — exactly 1 winner
- `TestExchangeMagicLink_ConcurrentExchangeNoDoubleProvision` — no double provision
- `TestConsumeMagicLinkToken_ExecrowsSemantics` — first consume=1, second=0
```
