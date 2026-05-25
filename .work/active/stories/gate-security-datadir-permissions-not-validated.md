---
id: gate-security-datadir-permissions-not-validated
kind: story
stage: implementing
tags: [security, plugin, hardening]
parent: feature-server-secret-log-hygiene
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-25
updated: 2026-05-25
---

# DataDir() silently reuses existing dir without validating permissions

## Severity
Low

## Domain
Secrets & Configuration

## Location
`cmd/jamsesh/state/state.go:30-53`

## Evidence
```go
if d := os.Getenv("JAMSESH_DATA_DIR"); d != "" {
    if err := os.MkdirAll(d, 0o700); err != nil { ... }
    return d, nil
}
...
dir := filepath.Join(base, "jamsesh")
if err := os.MkdirAll(dir, 0o700); err != nil { ... }
return dir, nil
```

## Implementation

Add `checkDirPerms` to `state.go` and call it at both resolution paths in
`DataDir()`. Also add it to `WriteSessionToken` for the session subdirectory.

**`cmd/jamsesh/state/state.go`** — add below the existing `Write` function:

```go
// checkDirPerms stats dir and returns an informative error when group or world
// read/write/execute bits are set (mode & 0o077 != 0). It never attempts to
// chmod — the operator is responsible for correcting the permissions. Called
// by DataDir after every os.MkdirAll to catch pre-existing loose-permission
// directories.
func checkDirPerms(dir string) error {
    info, err := os.Stat(dir)
    if err != nil {
        return fmt.Errorf("stat data dir %q: %w", dir, err)
    }
    if mode := info.Mode().Perm(); mode&0o077 != 0 {
        return fmt.Errorf(
            "data dir %q has unsafe permissions %04o (must be 0700 or tighter); "+
                "run: chmod 700 %q",
            dir, mode, dir,
        )
    }
    return nil
}
```

**Update `DataDir()`**:

```go
func DataDir() (string, error) {
    if d := os.Getenv("JAMSESH_DATA_DIR"); d != "" {
        if err := os.MkdirAll(d, 0o700); err != nil {
            return "", fmt.Errorf("creating JAMSESH_DATA_DIR %q: %w", d, err)
        }
        if err := checkDirPerms(d); err != nil {
            return "", err
        }
        return d, nil
    }

    var base string
    if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
        base = xdg
    } else {
        home, err := os.UserHomeDir()
        if err != nil {
            return "", fmt.Errorf("resolving home directory for data dir: %w", err)
        }
        base = filepath.Join(home, ".local", "share")
    }
    dir := filepath.Join(base, "jamsesh")
    if err := os.MkdirAll(dir, 0o700); err != nil {
        return "", fmt.Errorf("creating data dir %q: %w", dir, err)
    }
    if err := checkDirPerms(dir); err != nil {
        return "", err
    }
    return dir, nil
}
```

**Update `WriteSessionToken`**:

```go
func WriteSessionToken(sessionID string, token []byte) error {
    dir, err := DataDir()
    if err != nil {
        return err
    }
    sessDir := filepath.Join(dir, "sessions", sessionID)
    if err := os.MkdirAll(sessDir, 0o700); err != nil {
        return fmt.Errorf("creating session dir %q: %w", sessDir, err)
    }
    if err := checkDirPerms(sessDir); err != nil {
        return fmt.Errorf("session dir permissions: %w", err)
    }
    return Write("sessions/"+sessionID+"/token", token, 0o600)
}
```

## Implementation Notes
- `os.MkdirAll(d, 0o700)` on a freshly-created directory always produces
  a dir with mode 0o700 — `checkDirPerms` on it returns nil immediately.
  The perm check only fires for pre-existing directories with loose mode.
- The function body is in the same file (`state.go`), package-private.
  Tests call it indirectly through `DataDir()`.
- `checkDirPerms` is safe to unit-test directly via the `state` package
  tests (internal package access).

## Acceptance Criteria
- [ ] `checkDirPerms` function added to `state.go`
- [ ] `DataDir()` calls `checkDirPerms` at both resolution branches (JAMSESH_DATA_DIR and XDG default)
- [ ] `WriteSessionToken` calls `checkDirPerms` on the session subdirectory
- [ ] `TestDataDir_LoosePermissionsRefused`: pre-existing dir at mode 0o755 → `DataDir()` returns non-nil error containing "unsafe permissions" and "chmod 700"
- [ ] `TestDataDir_TightPermissionsAccepted`: dir at mode 0o700 → `DataDir()` returns nil error
- [ ] `TestDataDir_GroupReadPermissionsRefused`: dir at mode 0o750 → error
- [ ] Existing `TestDataDir_envOverride` and `TestDataDir_xdgDefault` still pass
