---
id: bug-scan-cursor-pagination-drops-rows
created: 2026-05-30
tags: [bug, data-layer]
bug_origin: scan
bug_severity: medium
bug_domain: data-layer
bug_location: db/queries/sqlite/comments.sql:27
---

# Cursor pagination drops rows sharing the same created_at (LastID tiebreaker never applied)

**Location**: `db/queries/sqlite/comments.sql:27` (mirror `db/queries/postgres/comments.sql:27`, `sessions.sql:54`; cursor `internal/portal/pagination/cursor.go:33`) · **Severity**: medium · **Pattern**: missing tiebreaker breaking cursor stability / pagination drift

The cursor is documented to use `LastID` to disambiguate rows with equal `created_at`, but neither the WHERE clause nor ORDER BY uses `id`: the query is `... AND created_at < ? ORDER BY created_at DESC LIMIT ?`. `ORDER BY created_at DESC` has no stable tiebreaker, and the strict `created_at < lastCreatedAt` bound silently skips every row whose `created_at` exactly equals the last item of the previous page. ULID ids are minted with the same `now()` per request, so comments/sessions created in the same millisecond commonly share `created_at` and are lost across page boundaries. Fix: make it a true keyset on `(created_at, id)` — `ORDER BY created_at DESC, id DESC` with boundary `(created_at < $before) OR (created_at = $before AND id < $lastID)`, threading `cur.LastID` through both dialect query files and adapters.

```sql
WHERE session_id = ? ... AND created_at < ?   -- exclusive bound on created_at ONLY
ORDER BY created_at DESC                       -- no id tiebreaker; LastID never used
LIMIT ?;
```
