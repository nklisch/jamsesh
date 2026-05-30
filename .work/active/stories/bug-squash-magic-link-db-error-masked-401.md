---
id: bug-squash-magic-link-db-error-masked-401
kind: story
stage: drafting
tags: [bug, portal, error-handling, high]
parent: epic-bug-squash
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
```
