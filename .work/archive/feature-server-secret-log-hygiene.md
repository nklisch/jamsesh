---
id: feature-server-secret-log-hygiene
kind: feature
stage: done
tags: [security, portal, plugin, logging]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# Server-side secret & log hygiene

## Brief

Bounded server-side hardenings that share a shape — bound or scrub
sensitive material at trust boundaries (OAuth callback URLs, upstream
refresh responses, cached binaries, on-disk data dirs). All four are low
severity defense-in-depth changes, all are localized to a single
file/function, and none shift architecture or contracts.

Two surfaces span portal (`internal/portal/auth/oauth.go`,
`cmd/jamsesh/portalclient/refresh.go`) and the plugin wrapper
(`plugins/jamsesh/bin/jamsesh`, `cmd/jamsesh/state/state.go`) — grouped
together because the discipline is the same and the work is small enough
that a single feature pass keeps it coherent.

## Member stories

- `gate-security-oauth-callback-log-scrubbing` —
  redact `code`/`state` query params from OAuth callback access logs
- `gate-security-refresh-error-body-leak` —
  bound upstream response body in refresh-path error wrap (~512B cap,
  strip Authorization-like fields)
- `gate-security-wrapper-cache-hit-no-resig-verify` —
  re-verify cached jamsesh binary's sha256 (and/or cosign) on every
  cache-hit before exec
- `gate-security-datadir-permissions-not-validated` —
  `os.Stat` DataDir on resolve; refuse or chmod when group/world rwx

## Approach (high level)

All four are independent — no internal sequencing. Tests should assert
the scrubbed/bound output and the failure paths (chmod refusal,
sha256 mismatch).

## Design decisions

- **oauth-callback story already implemented?**: Yes — the `OauthCallback`
  endpoint is `POST /api/auth/oauth/callback` with a JSON body; `code`/`state`
  never appear in the URL query string. `RedactQueryTokens` already covers
  both params in `sensitiveParams` (redact.go). The story reduces to adding
  a targeted test that pins this invariant: a request to the callback path
  with `code`/`state` in the query (simulating a hypothetical mis-route or a
  future refactor to GET) must produce redacted access log output. No
  net-new production code needed — purely a test addition.

- **body-leak cap value**: 512 bytes is the documented bound. Rationale:
  enough to capture the portal's typed error envelope (`{"error":"...","message":"..."}`)
  which tops out around 200 bytes in practice, while preventing megabyte-scale
  provider blobs from ever appearing in local stderr/logs. Applied uniformly
  across `refresh.go`, `GetJSON`, `GetJSONWithBearer`, `PostJSON`, `PostJSONAnon`.

- **body-leak helper location**: Package-private `func truncatedBody(r io.Reader) string`
  in `client.go` — shared by all five call sites in the same package. No
  new file needed.

- **wrapper sha256 re-verify strategy**: Store the sha256 of the verified
  binary in a sidecar file `<cached>.sha256` at install time, then re-check
  on every cache hit. This avoids re-downloading `checksums.txt` on each
  invocation (which would add ~200ms network latency to every jamsesh call)
  while still catching tampered binaries. The sidecar is a plain hex string.
  If the sidecar is absent or mismatched, fall through to full re-download
  and re-verify.

- **datadir permission policy**: Refuse (return error) rather than silently
  chmod. Rationale: a pre-existing loose-permission directory may be
  deliberately managed by the operator (shared-runner scenario). Silently
  remediating it could break the operator's setup. The error message should
  name the offending path and the actual mode so the user can fix it with
  one `chmod 700` invocation. The `os.MkdirAll`-created dir is always 0700
  (mode is set before first use), so this check fires only for pre-existing
  dirs.

## Architectural choice

**Minimal in-place hardening** — each change is a localized patch to the
existing code path, adding no new abstractions, no new packages, and no
new dependencies. This is the right choice for four independent defense-
in-depth items: they share no runtime state and the lowest cognitive-load
fix is also the most obviously correct one.

The alternative (a centralized "secrets hygiene" middleware or helper
package) is overhead for four small patches that each live in different
subsystems (bash, Go portal, Go plugin client).

## Implementation Units

### Unit 1: OAuth callback log-scrubbing test

**File**: `internal/portal/logging/logging_test.go`
**Story**: `gate-security-oauth-callback-log-scrubbing`

No production code change — the invariant is already enforced. Add one
test that confirms `code` and `state` are in `sensitiveParams` and that
the access-log middleware redacts them.

```go
// TestAccessMiddlewareRedactsOAuthQueryParams confirms that code and state
// params — which appear in GET-style OAuth callback URLs and hypothetical
// mis-routes — are redacted from the access log query field.
func TestAccessMiddlewareRedactsOAuthQueryParams(t *testing.T) {
    const secretCode = "github_authorization_code_xyz"
    const secretState = "csrf_nonce_abc"
    // ... httptest setup, GET with code=+state= in query,
    // assert neither value appears in log output
}
```

**Implementation Notes**:
- Test lives in `logging_test` package (external) alongside existing tests
- Use the same `slog.SetDefault` + `bytes.Buffer` pattern as existing tests
- Cover: `code=<secret>` absent from log, `state=<secret>` absent from log,
  `code=` and `state=` key names are present (value is `<redacted>`)

**Acceptance Criteria**:
- [ ] `TestAccessMiddlewareRedactsOAuthQueryParams` passes
- [ ] Test explicitly asserts neither raw `code` nor `state` value appears in the logged `query` field
- [ ] Test confirms `<redacted>` sentinel is present for each sensitive param

---

### Unit 2: Bounded error body in portalclient

**File**: `cmd/jamsesh/portalclient/client.go`
**File**: `cmd/jamsesh/portalclient/refresh.go`
**Story**: `gate-security-refresh-error-body-leak`

Add a package-private helper `truncatedBody` and apply it at all five
error-path `io.ReadAll` call sites.

```go
// truncatedBody reads at most maxErrBodyBytes from r and returns the content
// as a string. It always closes the reader. Content exceeding the limit is
// silently truncated; the caller is responsible for including the truncation
// signal in the error message if desired.
const maxErrBodyBytes = 512

func truncatedBody(r io.Reader) string {
    b, _ := io.ReadAll(io.LimitReader(r, maxErrBodyBytes))
    return string(b)
}
```

Apply at:
1. `refresh.go:86` — `body, _ := io.ReadAll(resp.Body)` → `body := truncatedBody(resp.Body)`
2. `client.go:139` — `GetJSON` non-2xx path
3. `client.go:176` — `GetJSONWithBearer` non-2xx path
4. `client.go:218` — `PostJSONAnon` non-2xx path
5. `client.go:253` — `PostJSON` non-2xx path

Error message format stays the same (`"... returned %d: %s"`); the only
change is that the body argument is now bounded.

**Implementation Notes**:
- `io.LimitReader` is already in stdlib — no new import
- The `defer resp.Body.Close()` already exists at each call site; do not
  double-close; `truncatedBody` should NOT close the reader — the deferred
  close handles it
- For `refresh.go`, the body is read before any deferred close — the defer
  fires after return, so reading is safe

**Acceptance Criteria**:
- [ ] All five non-2xx body reads go through `truncatedBody`
- [ ] `TestRefresher_Refresh_ServerError` still passes (the error is still returned)
- [ ] New test: server returns 401 with a 1KB body → error message body portion is ≤ 512 bytes
- [ ] New test: server returns 400 from `GetJSON` with oversized body → body portion ≤ 512 bytes

---

### Unit 3: Wrapper sha256 re-verify on cache hit

**File**: `plugins/jamsesh/bin/jamsesh`
**Story**: `gate-security-wrapper-cache-hit-no-resig-verify`

At install time, write the sha256 of the verified binary to `<cached>.sha256`.
On cache hit, read the sidecar and re-verify before exec; fall through to
full re-download if the sidecar is absent or mismatched.

```bash
# After the existing sha256 verify block (line ~104), just before the
# "Atomic install" block, write the sidecar:
echo "${actual}" > "${tmpdir}/${binary_name}.sha256"
# ... then in the atomic install:
mv "${tmpdir}/${binary_name}" "${cached}"
mv "${tmpdir}/${binary_name}.sha256" "${cached}.sha256"

# Cache hit block replacement:
if [[ -x "${cached}" ]]; then
  if [[ -f "${cached}.sha256" ]]; then
    cached_expected=$(cat "${cached}.sha256")
    cached_actual=$(sha256_of "${cached}")
    if [[ "${cached_expected}" == "${cached_actual}" ]]; then
      log "cache hit verified: ${cached}"
      exec "${cached}" "$@"
    fi
    log "cache hit sha256 mismatch — re-downloading"
  else
    log "cache hit missing sidecar — re-downloading"
  fi
fi
```

**Implementation Notes**:
- The `sha256_of` function is already defined below the cache-hit block
  (line ~87 in the current file). The cache-hit block must be restructured
  to call `sha256_of` which means the function definition must move above
  the cache-hit check, or the block must be rearranged. Simplest: move
  the `sha256_of` function definition to before the cache-hit section (step 4).
- On sidecar absent or mismatch: do NOT exec. Fall through to the normal
  download+verify+install flow. The `if [[ -x "${cached}" ]]` guard is
  removed (so the download always proceeds when we reach this point);
  the install will overwrite the cached binary and write a fresh sidecar.
- No `die` on sidecar mismatch — it is not an unrecoverable error; it
  means the cached binary may be tampered, so we re-verify from source.
  But we should `log` a warning at stderr unconditionally (not just when
  `JAMSESH_PLUGIN_VERBOSE` is set) since this is a security signal.
  Use `printf 'jamsesh-wrapper: WARNING: %s\n' "..." >&2` directly.
- The `JAMSESH_BIN_OVERRIDE` early-return path is unchanged.

**Acceptance Criteria**:
- [ ] Cache hit with valid sidecar → exec immediately (no download)
- [ ] Cache hit with absent sidecar → re-download and re-verify
- [ ] Cache hit with tampered binary (sidecar mismatch) → warning to stderr, re-download
- [ ] Fresh install writes `<cached>.sha256` alongside the binary
- [ ] If `sha256_of` moves, it remains correct and available in all paths

---

### Unit 4: DataDir permission validation

**File**: `cmd/jamsesh/state/state.go`
**Story**: `gate-security-datadir-permissions-not-validated`

After every `os.MkdirAll(dir, 0o700)` call in `DataDir()`, stat the
resulting directory and return an error if any group/world bits are set.

```go
// checkDirPerms stats dir and returns an error if group or world
// read/write/execute bits are set (i.e. if mode & 0o077 != 0).
// It does not attempt to chmod — the operator is responsible for
// correcting the permissions.
func checkDirPerms(dir string) error {
    info, err := os.Stat(dir)
    if err != nil {
        return fmt.Errorf("stat data dir %q: %w", err)
    }
    if mode := info.Mode().Perm(); mode&0o077 != 0 {
        return fmt.Errorf(
            "data dir %q has unsafe permissions %04o (must be 0700 or tighter); run: chmod 700 %q",
            dir, mode, dir,
        )
    }
    return nil
}
```

Apply in `DataDir()` at both resolution paths:

```go
// JAMSESH_DATA_DIR path:
if err := os.MkdirAll(d, 0o700); err != nil { ... }
if err := checkDirPerms(d); err != nil {
    return "", err
}
return d, nil

// XDG default path:
if err := os.MkdirAll(dir, 0o700); err != nil { ... }
if err := checkDirPerms(dir); err != nil {
    return "", err
}
return dir, nil
```

**Implementation Notes**:
- `os.MkdirAll` only sets mode on directories it creates. Pre-existing dirs
  keep their mode. A dir created by `os.MkdirAll(d, 0o700)` in this
  session will always pass the check — `0o700 & 0o077 == 0`. The check
  catches only pre-existing loose-permission dirs.
- The `checkDirPerms` function is package-private (`state` package,
  internal file). It is exported-name-friendly if ever needed in tests.
- All callers of `DataDir()` throughout the package already propagate
  errors; no caller swallows the return error. The new error will surface
  properly everywhere.
- The `WriteSessionToken` helper calls `os.MkdirAll(sessDir, 0o700)` for
  the session subdirectory. Add `checkDirPerms(sessDir)` there too for
  consistency.

**Acceptance Criteria**:
- [ ] `DataDir()` returns an error when the existing dir has mode 0o755
- [ ] `DataDir()` returns an error when the existing dir has mode 0o750
- [ ] `DataDir()` succeeds when the dir has mode 0o700
- [ ] Error message includes the dir path, the actual mode, and the suggested `chmod 700` command
- [ ] `WriteSessionToken` also checks session subdir permissions

---

## Implementation Order

All four stories are parallel — no dependency between them.

1. `gate-security-oauth-callback-log-scrubbing` — test-only, 15 min
2. `gate-security-refresh-error-body-leak` — 5-line helper + 5 call sites, 30 min
3. `gate-security-datadir-permissions-not-validated` — one helper + 2 call sites, 30 min
4. `gate-security-wrapper-cache-hit-no-resig-verify` — bash refactor, 30 min

Recommended wave order for an orchestrator:
- Wave 1 (parallel): stories 1, 2, 3
- Wave 2 (sequential): story 4 (bash; different subsystem, no Go conflicts)

## Testing

### Unit 1 (oauth log scrubbing)
`internal/portal/logging/logging_test.go` — new `TestAccessMiddlewareRedactsOAuthQueryParams`

### Unit 2 (body bound)
`cmd/jamsesh/portalclient/refresh_test.go` — extend `TestRefresher_Refresh_ServerError`
with an oversized body; add a new `TestRefresher_Refresh_LargeErrorBodyTruncated`.
`cmd/jamsesh/portalclient/client_test.go` — add `TestGetJSON_OversizedErrorBodyTruncated`.

### Unit 3 (wrapper sha256)
`plugins/jamsesh/bin/jamsesh` is a bash script. Testing approach: manual
verification using a small BATS or shell test if the project has one;
otherwise document the manual test steps in the story. The logic is simple
enough that code-review of the bash diff is the primary quality gate.

### Unit 4 (datadir perms)
`cmd/jamsesh/state/state_test.go` — add `TestDataDir_LoosePermissionsRefused`
and `TestDataDir_StrictPermissionsAccepted`. Use `t.TempDir()` + `os.Chmod`.

## Risks

- **wrapper sha256 re-verify adds `sha256_of` before cache-hit check**: The
  function currently lives after the cache-hit block. Moving it up is a
  two-line cut-and-paste with no semantic change — low risk.
- **datadir permission check breaks existing installs with loose perms**: This
  is intentional — it's the remediation. First invocation on a loose-permission
  dir will fail with a clear error. This is the desired behavior. The user sees
  a clear fix instruction. Low operational risk since the dir is typically
  created by jamsesh itself at mode 0700.
- **body truncation hides useful diagnostic info from refresh errors**: The
  portal's error envelopes are compact (`{"error":"...","message":"..."}`) and
  always fit within 512 bytes. Only a misbehaving or non-portal server returns
  a larger body, and in that case we don't need the full body for diagnosis.
  Accepted tradeoff.

## Implementation summary (autopilot run)

All four child stories landed at stage:review:
- `gate-security-oauth-callback-log-scrubbing` — test pin (no production change)
- `gate-security-refresh-error-body-leak` — `truncatedBody` helper + 5 call sites
- `gate-security-datadir-permissions-not-validated` — `checkDirPerms` + 19 test files updated
- `gate-security-wrapper-cache-hit-no-resig-verify` — sha256 sidecar + 3 BATS tests

Cross-cutting side effect: 19 test helper sites under `cmd/jamsesh/`
gained a `_ = os.Chmod(<dir>, 0o700)` immediately after each
`t.Setenv("JAMSESH_DATA_DIR", <dir>)` to satisfy the new perm check.

Verified: `go test ./cmd/jamsesh/... -count 1` passes; `bats tests/wrapper/*.bats` → 18 passed.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Feature delivered as briefed. All 4 child stories approved individually. Cross-cutting test-helper sweep is the unavoidable consequence of the new perm check (Linux `t.TempDir()` is 0o755), correctly applied. No foundation-doc drift; defense-in-depth posture is preserved.
