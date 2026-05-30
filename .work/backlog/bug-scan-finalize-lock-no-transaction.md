---
id: bug-scan-finalize-lock-no-transaction
created: 2026-05-30
tags: [bug, error-handling]
bug_origin: scan
bug_severity: medium
bug_domain: error-handling
bug_location: internal/portal/finalize/lock_acquire.go:187
---

# Finalize-lock acquisition runs a 4-step mutation sequence with no transaction or compensation

**Location**: `internal/portal/finalize/lock_acquire.go:187` · **Severity**: medium · **Pattern**: multi-step operation, no transaction, no rollback

Four writes — `InsertFinalizeLock`, `SupersedeFinalizeLock`, `SetFinalizeLock` (sessions pointer), `UpdateSessionStatus` (active→finalizing) — are issued sequentially outside any `WithTx`. A transient failure at step 2/3/4 returns 503 but leaves partial state: the new lock row is inserted while the supersede marker, sessions pointer, and/or status are not updated. A retry can then hit the `override_race_lost` 409 branch against the caller's own prior attempt, or leave the session `active` while a lock row exists. Fix: wrap the insert→supersede→set-pointer→status sequence in `store.WithTx` so a mid-sequence failure rolls back, matching the tx-emit-then-fanout discipline used elsewhere.

```go
h.store.InsertFinalizeLock(...)     // step 1
h.store.SupersedeFinalizeLock(...)  // step 2
h.store.SetFinalizeLock(...)        // step 3
h.store.UpdateSessionStatus(...)    // step 4 — no enclosing tx
```
