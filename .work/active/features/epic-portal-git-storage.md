---
id: epic-portal-git-storage
kind: feature
stage: implementing
tags: [portal]
parent: epic-portal-git
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Git — Storage & Lifecycle

## Brief

The on-disk and DB-side layer for session bare repos: storage-path resolution,
bare-repo init/teardown helpers, and the archived-session semantics that
follow the 90-day retention window.

**On-disk layout** (locked at epic-design — no path abstraction layer):

```
<storage>/orgs/<org_id>/sessions/<session_id>.git
```

The storage root is configurable; org and session ids are uuid/ULID strings.
Bare repos use git's standard layout (`HEAD`, `objects/`, `refs/`, etc.).

**Lifecycle:**

- **Create**: called from `POST /api/sessions` (cross-epic call from
  `epic-portal-api`). Atomic with the `sessions` row insert: create the bare
  repo first (`git init --bare`), then commit the session row. On row-insert
  failure, `rm -rf` the half-created repo. Invariant after success: "session
  row exists ⟹ bare repo exists."
- **End** (finalize / abandon / timeout): no repo deletion at end. The repo
  becomes read-only via the pre-receive policy ("session.ended" rejection
  path), retained for the 90-day window so participants can fetch and
  finalize locally.
- **Archive** (90+ days post-end): hard-delete the bare repo directory.
  Insert a row into `archived_sessions` with: `session_id`, `name`, `org_id`,
  `member_account_ids` (string array or JSON), `goal_text`, `ended_at`,
  `end_reason` (`finalize | abandon | timeout`), `final_branch_name`
  (nullable). Delete the original `sessions` row. No restore path by design.
- **Archived stub response**: any HTTP/git request against an archived
  session id returns a 410 Gone with the JSON stub: "This session was
  archived on YYYY-MM-DD. Final branch: `<name>` (pushed to <repo>)." The
  smart-HTTP handler and the REST API both consume the same stub formatter
  from this feature.

**Schema additions** (extending `epic-portal-foundation-data-layer`):
the `archived_sessions` table is owned by this feature (added via a sqlc
migration that this feature ships).

Does NOT cover the smart-HTTP handlers (`smart-http` feature) or
pre/post-receive (their own features). Does NOT cover the retention sweep
trigger — that's a scheduled job that calls into this feature's archive
helper; the trigger itself can be a documented operator cron in v1 or a
deferred internal scheduler.

## Epic context

- Parent epic: `epic-portal-git`
- Position in epic: foundation feature — pre-receive, post-receive, and
  smart-http all consume storage helpers (path resolution, bare-repo
  opening, archived-session lookup).

## Foundation references

- `docs/SPEC.md` — Ref structure, Lifecycle (Creation, End, Retention),
  Deployment shape
- `docs/ARCHITECTURE.md` — Git smart-HTTP component (storage path)
- `docs/SECURITY.md` — Audit trail, What a portal breach exposes

## Inherited epic design decisions

- **Storage path schema**: v1 lock —
  `<storage>/orgs/<org_id>/sessions/<session_id>.git`. No abstraction layer.
- **Archived-session semantics**: hard-delete bare repo + DB rows; retain
  a tiny `archived_sessions` table row for the stub response. No restore.
- **Bare repo init timing**: eager, atomic with session row insert.

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Git binary for init**: subprocess `git init --bare` via
  `os/exec`. Rationale: depends only on the system `git` (already
  required for smart-HTTP), simpler than go-git for one-off ops,
  matches the smart-HTTP feature's existing pattern.
- **`archived_sessions` schema**: added by THIS feature as
  `00002_*.sql` migrations under `internal/db/migrations/{sqlite,postgres}/`,
  plus corresponding query files at `db/queries/{sqlite,postgres}/archived_sessions.sql`
  and a domain type + Store-interface extension in
  `internal/db/store/`. The data-layer feature is done, so this
  feature does NOT need to coordinate that schema work — it owns
  the schema-additive change cleanly.
- **Stub-response shape**: a single `ArchivedStub` formatter at
  `internal/portal/storage/stub.go` returning a struct that
  `httperr.Write`-compatible code can marshal. Used by both git
  smart-HTTP (410 Gone) and REST API (also 410 Gone), so the
  formatter lives in `storage/` (the owning feature).
- **Atomic create**: `CreateRepoAndSession` helper opens a Tx
  against the Store, inserts the session row inside the Tx, then
  creates the bare repo on disk, then commits the Tx. If repo
  creation fails, the Tx rolls back and there's no row. If the Tx
  commit fails (race / DB-side error), `os.RemoveAll` the partial
  repo. The invariant is "session row exists ⟹ bare repo exists";
  this ordering preserves it.

  Wait — actually the locked epic decision says "create the bare
  repo FIRST, then commit the session row. On row-insert failure,
  `rm -rf` the half-created repo." So the correct order is: repo
  init → row insert → on row-insert failure, rm-rf. Going with the
  locked decision: repo first.

- **Archive operation**: idempotent. Re-running archive on an
  already-archived session is a no-op (the bare repo is gone, the
  row is in `archived_sessions`, the `sessions` row is deleted).
- **No restore path**: explicit. Once archived, the only recovery
  is from operator's backup.

## Architectural choice

**A `storage` package at `internal/portal/storage/` exposing a
`Service` interface + concrete implementation. The Service has
helpers used by sibling features:**

```go
type Service interface {
    RepoPath(orgID, sessionID string) string
    CreateRepo(ctx context.Context, orgID, sessionID string) error
    RemoveRepo(ctx context.Context, orgID, sessionID string) error
    RepoExists(orgID, sessionID string) (bool, error)
    ArchiveSession(ctx context.Context, orgID, sessionID string, info ArchiveInfo) error
    LookupArchived(ctx context.Context, orgID, sessionID string) (*ArchivedRecord, error)
    StubResponse(rec *ArchivedRecord) ArchivedStub
}
```

Concrete implementation: `internal/portal/storage/service.go` taking
`(rootDir string, store store.Store)`.

## Implementation Units

### Unit 1: Path resolution + bare-repo helpers

**File**: `internal/portal/storage/paths.go`, `internal/portal/storage/repo.go`
**Story**: `epic-portal-git-storage-bare-repo-helpers`

```go
package storage

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
)

func (s *service) RepoPath(orgID, sessionID string) string {
    return filepath.Join(s.root, "orgs", orgID, "sessions", sessionID+".git")
}

func (s *service) RepoExists(orgID, sessionID string) (bool, error) {
    info, err := os.Stat(s.RepoPath(orgID, sessionID))
    if err != nil {
        if os.IsNotExist(err) { return false, nil }
        return false, err
    }
    return info.IsDir(), nil
}

func (s *service) CreateRepo(ctx context.Context, orgID, sessionID string) error {
    p := s.RepoPath(orgID, sessionID)
    if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
        return fmt.Errorf("storage: mkdir parent: %w", err)
    }
    if err := os.Mkdir(p, 0o750); err != nil {
        return fmt.Errorf("storage: mkdir repo: %w", err)
    }
    cmd := exec.CommandContext(ctx, "git", "init", "--bare", p)
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        os.RemoveAll(p) // best effort cleanup
        return fmt.Errorf("storage: git init --bare: %w", err)
    }
    return nil
}

func (s *service) RemoveRepo(ctx context.Context, orgID, sessionID string) error {
    return os.RemoveAll(s.RepoPath(orgID, sessionID))
}
```

### Unit 2: archived_sessions schema + queries

**Files**:
- `internal/db/migrations/sqlite/00002_archived_sessions.sql`
- `internal/db/migrations/postgres/00002_archived_sessions.sql`
- `db/queries/sqlite/archived_sessions.sql`
- `db/queries/postgres/archived_sessions.sql`
- `db/schema/sqlite.sql` (edit — add CREATE TABLE)
- `db/schema/postgres.sql` (edit — add CREATE TABLE)
- `sqlc generate` produces additions to sqlitestore + pgstore

**Story**: `epic-portal-git-storage-archive-and-stub`

Schema (sqlite):

```sql
CREATE TABLE archived_sessions (
    session_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal_text TEXT NOT NULL,
    member_account_ids TEXT NOT NULL,   -- JSON array
    ended_at DATETIME NOT NULL,
    archived_at DATETIME NOT NULL,
    end_reason TEXT NOT NULL CHECK (end_reason IN ('finalize','abandon','timeout')),
    final_branch_name TEXT
);
CREATE INDEX archived_sessions_org_idx ON archived_sessions(org_id);
```

(Postgres analogous with TIMESTAMPTZ.)

Queries: `InsertArchivedSession :exec`, `GetArchivedSession :one`
(WHERE org_id = ? AND session_id = ?).

### Unit 3: Archive operation

**File**: `internal/portal/storage/archive.go`
**Story**: `epic-portal-git-storage-archive-and-stub`

```go
type ArchiveInfo struct {
    Name             string
    GoalText         string
    MemberAccountIDs []string
    EndedAt          time.Time
    EndReason        string  // "finalize" | "abandon" | "timeout"
    FinalBranchName  *string
}

type ArchivedRecord struct {
    SessionID        string
    OrgID            string
    Name             string
    GoalText         string
    MemberAccountIDs []string
    EndedAt          time.Time
    ArchivedAt       time.Time
    EndReason        string
    FinalBranchName  *string
}

func (s *service) ArchiveSession(ctx context.Context, orgID, sessionID string, info ArchiveInfo) error {
    memberJSON, _ := json.Marshal(info.MemberAccountIDs)
    if err := s.store.InsertArchivedSession(ctx, store.InsertArchivedSessionParams{
        SessionID:        sessionID,
        OrgID:            orgID,
        Name:             info.Name,
        GoalText:         info.GoalText,
        MemberAccountIDs: string(memberJSON),
        EndedAt:          info.EndedAt,
        ArchivedAt:       time.Now().UTC(),
        EndReason:        info.EndReason,
        FinalBranchName:  info.FinalBranchName,
    }); err != nil {
        // If duplicate (already archived), treat as success.
        if errors.Is(err, store.ErrUniqueViolation) {
            return nil
        }
        return fmt.Errorf("storage: insert archived row: %w", err)
    }
    // Hard-delete the bare repo.
    if err := s.RemoveRepo(ctx, orgID, sessionID); err != nil {
        return fmt.Errorf("storage: remove repo: %w", err)
    }
    // Delete the live sessions row + cascaded session_members.
    if err := s.store.DeleteSession(ctx, orgID, sessionID); err != nil {
        return fmt.Errorf("storage: delete session row: %w", err)
    }
    return nil
}
```

(`DeleteSession` is added to the Store interface as part of this
feature — extension to data-layer's surface.)

### Unit 4: Stub formatter

**File**: `internal/portal/storage/stub.go`

```go
type ArchivedStub struct {
    Error      string `json:"error"`       // "session.archived"
    Message    string `json:"message"`     // user-readable explanation
    Details    struct {
        ArchivedAt       string  `json:"archived_at"`
        FinalBranchName  *string `json:"final_branch_name,omitempty"`
        EndReason        string  `json:"end_reason"`
    } `json:"details"`
    HTTPStatus int `json:"-"`              // 410
}

func (s *service) StubResponse(rec *ArchivedRecord) ArchivedStub {
    msg := "This session was archived on " + rec.ArchivedAt.Format("2006-01-02") + "."
    if rec.FinalBranchName != nil {
        msg += " Final branch: " + *rec.FinalBranchName + "."
    }
    return ArchivedStub{
        Error:      "session.archived",
        Message:    msg,
        Details: struct {
            ArchivedAt       string  `json:"archived_at"`
            FinalBranchName  *string `json:"final_branch_name,omitempty"`
            EndReason        string  `json:"end_reason"`
        }{
            ArchivedAt:      rec.ArchivedAt.Format(time.RFC3339),
            FinalBranchName: rec.FinalBranchName,
            EndReason:       rec.EndReason,
        },
        HTTPStatus: 410,
    }
}
```

## Story decomposition

Two stories chained:

1. **bare-repo-helpers** — Service interface, paths, CreateRepo /
   RemoveRepo / RepoExists, basic tests against `t.TempDir()`.
   depends_on: []
2. **archive-and-stub** — `archived_sessions` migration + queries +
   schema-additive, ArchiveSession helper, StubResponse formatter,
   DeleteSession extension on Store. depends_on:
   [epic-portal-git-storage-bare-repo-helpers]

## Implementation Order

1. bare-repo-helpers
2. archive-and-stub

## Testing

- `internal/portal/storage/repo_test.go` — path resolution,
  CreateRepo + RepoExists round-trip against `t.TempDir()`, error
  paths
- `internal/portal/storage/archive_test.go` — ArchiveSession
  end-to-end against in-memory SQLite Store + tempdir
- Stub formatter table tests (with and without `final_branch_name`)

## Risks

- **System git availability in CI**: the CreateRepo tests invoke
  `git init --bare` as subprocess. Ubuntu runners have git
  pre-installed; CI alpine-images may not. Mitigation: pin runner
  to `ubuntu-latest` in CI; if git is missing, skip the relevant
  tests with `t.Skip`.
- **JSON storage of member_account_ids**: SQLite has no native
  JSON type, so stored as TEXT. Reads return string; caller
  `json.Unmarshal`s. Documented in the ArchivedRecord struct.
- **Concurrent archive**: two archive calls racing for the same
  session would both attempt to delete. Mitigation: the unique
  constraint on `archived_sessions.session_id` ensures only one
  succeeds in the INSERT; the second is a no-op via the unique
  violation handling.
