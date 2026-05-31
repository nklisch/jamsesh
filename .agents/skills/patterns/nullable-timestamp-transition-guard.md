# Nullable-Timestamp Transition Guard

Single-use or terminal state transitions update a nullable timestamp only when
that timestamp is still `NULL`.

## Rationale

The nullable timestamp is both state and race guard.
`WHERE used_at/accepted_at/released_at/resolved_at IS NULL` makes the transition
single-winner or idempotent at the database boundary instead of relying only on
a prior read.

## Examples

### Resume token consume

**File**: `db/queries/sqlite/resume_tokens.sql:11`

```sql
UPDATE resume_tokens
SET used_at = ?
WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?
RETURNING *;
```

### Magic-link consume

**File**: `db/queries/sqlite/magic_link_tokens.sql:11`

```sql
UPDATE magic_link_tokens
SET used_at = ?
WHERE id = ? AND used_at IS NULL;
```

### Org invite accept

**File**: `db/queries/sqlite/org_invites.sql:12`

```sql
UPDATE org_invites
SET accepted_at = ?, accepted_by_account_id = ?
WHERE id = ? AND accepted_at IS NULL;
```

## When to Use

- One-time token consumption.
- Accept/release/resolve operations where a repeated transition must not rewrite the terminal timestamp.
- Racy workflows where multiple callers might attempt the same transition.

## When NOT to Use

- Non-terminal updates where repeated writes are expected.
- Counters or sequence allocation; use atomic increment/returning patterns instead.

## Common Violations

- Read-checking `used_at` and then doing an unconditional update.
- Forgetting to inspect affected rows or returned row when winner/loser distinction matters.
- Omitting expiry predicates for token consume operations.

