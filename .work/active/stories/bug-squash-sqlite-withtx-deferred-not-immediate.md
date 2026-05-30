---
id: bug-squash-sqlite-withtx-deferred-not-immediate
kind: story
stage: review
tags: [bug, portal, data-layer]
parent: epic-bug-squash-data-tx-integrity
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: medium
bug_domain: data-layer
bug_location: internal/db/store/sqlite_adapter.go:1034
---

# SQLite WithTx claims BEGIN IMMEDIATE but opens a DEFERRED transaction

**Location**: `internal/db/store/sqlite_adapter.go:1034` (DSN at `internal/db/connect.go:152`) · **Severity**: medium · **Pattern**: wrong isolation / missing upfront write lock for read-modify-write

The comment says `BEGIN IMMEDIATE acquires a write-lock upfront ... to avoid SQLITE_BUSY`, but `BeginTx(ctx, &sql.TxOptions{})` is the zero value, which opens a DEFERRED transaction, and the DSN does not inject `_txlock=immediate` (only `foreign_keys` + `busy_timeout`). A DEFERRED tx takes a read lock first and must upgrade to a write lock on first write; with `MaxOpenConns` defaulting to 25, two read-then-write `WithTx` bodies on distinct connections can each hold a read lock and deadlock on upgrade — `busy_timeout` does not retry the lock-upgrade case, so multi-statement read-then-write transactions can fail spuriously under load. Fix: add `_txlock=immediate` to the SQLite DSN (or begin so the write lock is taken upfront), or cap SQLite `MaxOpenConns` to 1 to serialize writers.

```go
// BEGIN IMMEDIATE acquires a write-lock upfront ...  <- comment
tx, err := a.db.BeginTx(ctx, &sql.TxOptions{})  // zero value == DEFERRED, not IMMEDIATE
```

## Implementation notes

Added `_txlock=immediate` to the SQLite DSN in `connect.go:sqliteDSN` via a new `!strings.Contains(query, "_txlock")` guard (same pattern as foreign_keys and busy_timeout). The modernc.org/sqlite v1.50.1 driver honors this parameter and emits `BEGIN IMMEDIATE` for every `BeginTx` call. Updated the `WithTx` comment in `sqlite_adapter.go` to accurately describe the behavior (DSN controls it; no TxOptions needed). Test: `TestSQLiteWithTxImmediateNoDeadlock` in `internal/db/store/withtx_immediate_test.go` — 6 goroutines each do a read-then-write tx against a shared file-backed SQLite with MaxOpenConns=5; all must commit without SQLITE_BUSY.
