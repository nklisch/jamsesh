---
id: gate-security-sqlite-default-dsn-file-mode
kind: story
stage: review
tags: [security, portal, documentation]
parent: null
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# SQLite default DSN path is relative (`./jamsesh.db`) — file mode defaults to 0644

## Severity
Low

## Domain
Secrets & Configuration

## Location
`internal/portal/config/config.go:350`

## Evidence
```go
DBDSN:     "./jamsesh.db",
```

The portal binary runs as `USER nobody` in the Dockerfile, but
`modernc.org/sqlite` creates the DB file with the default umask
(typically 0644) — readable by every container user. In the same-host
self-host model (e.g. running `portal` directly), `./jamsesh.db` may
land in the user's working directory readable by other local users. The
DB holds all oauth_tokens, magic_link_tokens, and account email PII.

## Remediation direction
Document that the operator must chmod the SQLite file to 0600 after
first start, or chmod the parent directory to 0700. Optionally wrap the
`db.Open` path with a post-open `os.Chmod(path, 0600)` for sqlite DSNs.

## Implementation notes

### Files changed

- **`internal/db/connect.go`** — primary change. Added `os.Chmod(fp, 0600)`
  in the `sqlite` branch of `Open()`, immediately after migrations complete.
  Also added a `sqliteFilePath()` helper that strips query params and the
  `file:` URI prefix to extract the on-disk path from a DSN.
  Added `log/slog` and `os` imports.

- **`internal/db/connect_test.go`** — two new tests:
  - `TestSQLiteFilePath` — table-driven unit test covering `:memory:`,
    `file::memory:`, paths with query params, `file:` URI prefix, etc.
  - `TestOpenSQLite_Chmod` — integration test asserting that `Open` with a
    temp file-backed DSN results in `0600` permissions on the created file,
    and that `:memory:` still opens without error.

- **`docs/SELF_HOST.md`** — added a "File permissions" note under §5 SQLite
  explaining that the portal chmods to `0600` on startup, and providing the
  one-time `chmod 0600` command for operators upgrading from older builds.

### SQLite vs Postgres branch detection

The `driver` argument to `Open()` is the authoritative discriminator
(always `"sqlite"` or `"postgres"` per config validation). The chmod lives
entirely inside `case "sqlite":` so Postgres is never touched.

### In-memory DSN handling

`sqliteFilePath()` returns `""` for `:memory:` and `file::memory:` (with or
without query params). The `if fp := sqliteFilePath(dsn); fp != ""` guard
skips the `os.Chmod` call entirely for those DSNs — no file to chmod.

### DSN with `?` query params

`sqliteFilePath()` uses `strings.Cut(dsn, "?")` to strip query params before
extracting the path, so DSNs like `./jamsesh.db?_pragma=foreign_keys(1)` are
handled correctly.

### Chmod failure handling

`os.Chmod` errors log via `slog.WarnContext` with `"path"` and `"err"` keys
but do not abort startup. This matches the project's pattern for best-effort
hardening (e.g. the migration advisory lock releases on disconnect without
failing the whole pod).

### Location correction

The story body referenced `internal/portal/config/config.go:350` as the
location. The actual DB-open site is `internal/db/connect.go:Open()`,
consumed by `cmd/portal/main.go:274`. The config file only holds the DSN
default string; the connection is established in `internal/db`.
