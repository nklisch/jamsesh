---
id: epic-portal-git-storage-bare-repo-helpers
kind: story
stage: review
tags: [portal]
parent: epic-portal-git-storage
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Git Storage — Bare Repo Helpers

## Scope

Stand up the `storage` package: path resolution + bare-repo
init/teardown/existence helpers. After this story, callers can
materialize a bare repo on disk and check whether one exists.

## Units delivered

- `internal/portal/storage/service.go` — `Service` interface +
  `service` concrete struct constructor
- `internal/portal/storage/paths.go` — `RepoPath(orgID, sessionID)`
- `internal/portal/storage/repo.go` — `CreateRepo`, `RemoveRepo`,
  `RepoExists` (uses `os/exec` for `git init --bare`)
- `internal/portal/storage/repo_test.go` — covers path conventions,
  create/exists round-trip against `t.TempDir()`, error paths

## Acceptance Criteria

- [ ] `service.RepoPath("o", "s")` returns
      `<root>/orgs/o/sessions/s.git`
- [ ] `CreateRepo` against a fresh `t.TempDir()` produces a valid
      bare git repo (verified by `git rev-parse --is-bare-repository`
      against the path, or by checking for `HEAD`, `objects/`,
      `refs/`)
- [ ] `RepoExists` returns true after CreateRepo, false before
- [ ] `RemoveRepo` removes the directory tree and `RepoExists`
      returns false after
- [ ] CreateRepo against a path with a parent directory missing
      creates the parent directories (chain of mkdirs)
- [ ] CreateRepo against an already-existing repo returns an error
      (does NOT silently overwrite); the test verifies error path
- [ ] Tests skip cleanly if `git` is not on PATH (use
      `exec.LookPath("git")` to gate the suite)

## Notes

- The `service` constructor takes `(rootDir string, store store.Store)`
  even though `store` isn't used by this story's units. Including
  it now lets the archive-and-stub story add methods without
  changing the constructor signature.
- Directory permissions: 0o750 for created dirs (operator-readable,
  group-readable, others-no). Matches typical self-host posture.

## Implementation Notes

- **Full Service interface declared** (option a): all future methods
  (`ArchiveSession`, `LookupArchived`, `StubResponse`) are declared in
  `service.go` and return `fmt.Errorf("not implemented yet")` or a zero
  value. Each has a `// TODO: epic-portal-git-storage-archive-and-stub`
  comment. This avoids interface churn when the next story fills them in.
- **store.Store passed as nil in tests**: the constructor accepts a nil
  Store without panicking because no method in this story calls into it.
  The archive-and-stub tests will pass a real in-memory SQLite store.
- **CreateRepo error on existing dir**: uses `os.Mkdir` (not `MkdirAll`)
  for the repo dir itself, so an `os.IsExist` error surfaces clearly.
  A descriptive message is returned so callers can distinguish duplicate
  creation from permission errors.
- **git init --bare cleanup**: if `git init --bare` fails after `os.Mkdir`
  succeeds, `os.RemoveAll` is called best-effort to leave no stale dir.
- **Test coverage**: 10 tests across path resolution, create-verifies-bare-
  layout, parent-dir creation, duplicate-create error, round-trip, remove
  idempotency, and pre-create false existence.
- **Files delivered**:
  - `internal/portal/storage/service.go`
  - `internal/portal/storage/paths.go`
  - `internal/portal/storage/repo.go`
  - `internal/portal/storage/repo_test.go`
