---
id: portal-dep-failure-error-codes-git-subprocess
kind: story
stage: review
tags: [portal]
parent: portal-dep-failure-error-codes
depends_on: [portal-dep-failure-error-codes-envelope-helper]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Wire git smart-HTTP subprocess failures to `dep.git_subprocess_failed`

Replaces the bare `http.Error(...)` calls in
`internal/portal/githttp/{info_refs,upload_pack,receive_pack}.go`
that handle subprocess spawn/wait failures with
`httperr.Write(w, r, httperr.ErrGitSubprocessFailed(err))`.

Git smart-HTTP does NOT flow through the oapi-codegen strict handler,
so it doesn't pick up the global translator from story 1
automatically. Each subprocess-failure site emits the envelope
directly via `httperr.Write`.

Pre-receive *rejection* paths (writeReportStatusRejection,
malformed-request 400, bad-content-type 400) are unchanged —
those follow the smart-HTTP report-status protocol, which is a
different contract a git client expects.

## Files

- **Edit** `internal/portal/githttp/info_refs.go`:

  Current (around line ~46-48):

  ```go
  if err := cmd.Run(); err != nil {
      slog.ErrorContext(r.Context(), "info/refs subprocess failed",
          "service", service, "err", err)
      http.Error(w, "git subprocess error", http.StatusInternalServerError)
      return
  }
  ```

  Target:

  ```go
  if err := cmd.Run(); err != nil {
      slog.ErrorContext(r.Context(), "info/refs subprocess failed",
          "service", service, "err", err)
      httperr.Write(w, r,
          httperr.ErrGitSubprocessFailed(deperr.WrapGitSubprocess(err)))
      return
  }
  ```

  (The deperr wrap is belt-and-suspenders here — `httperr.Write` is
  already the typed envelope; the wrap is for log-chain consistency
  with the rest of the codebase. Optional but recommended.)

- **Edit** `internal/portal/githttp/upload_pack.go`:

  Apply the same pattern at the `http.Error(w, "git subprocess
  error", http.StatusInternalServerError)` site (line ~39) and any
  other subprocess-spawn / pipe-setup failure sites.

- **Edit** `internal/portal/githttp/receive_pack.go`:

  Apply the pattern at every `http.Error(w, "internal server error",
  http.StatusInternalServerError)` site that follows a subprocess
  operation:
  - `slog.ErrorContext(... "build validation repo" ...)` -> use
    `httperr.Write(w, r, httperr.ErrInternal(err))` because this
    is not a subprocess error per se, it's an in-process pack-parse
    failure. Keep it as `internal` envelope, not `dep.git_subprocess_failed`.
  - `slog.ErrorContext(... "get session" ...)` -> wrap with
    `deperr.WrapDBIfTransient` and write via
    `httperr.WriteFromError`. **Wait — this is a DB failure, not a
    git subprocess failure.** Belongs in the DB story; track it
    there. Either move the wrap to story 3's scope or this story
    handles it explicitly. Recommended: handle here since the file is
    open, and add a note in story 3 referencing this delta.
  - `"validate"` site -> `httperr.ErrInternal` (validation is
    in-process; this isn't a dep failure).
  - `"stdin pipe"`, `"stdout pipe"`, `"start subprocess"` ->
    all wrap with `deperr.WrapGitSubprocess` and emit via
    `httperr.Write(w, r, httperr.ErrGitSubprocessFailed(err))`.

- **Edit** `internal/portal/githttp/handler_test.go` — add a test:
  `TestUploadPack_SubprocessFailure_Returns500DepGitSubprocessFailed`.
  Use a fixture where `git-upload-pack` is forced to fail (e.g., by
  pointing the handler at a non-existent repo path, or by injecting a
  hook that no-ops the subprocess). Assert on:
  - HTTP 500 (NOT 503 — git subprocess is not a transient-retry case)
  - `Content-Type: application/json; charset=utf-8`
  - Body decodes to `{error: "dep.git_subprocess_failed"}`
  - NO `Retry-After` header

## Note on git-client UX

Git clients (the CLI, libgit2, etc.) do not parse JSON error bodies.
They display whatever the server returns alongside the HTTP status.
A JSON body of
`{"error":"dep.git_subprocess_failed","message":"git subprocess failed"}`
displays approximately the same to a git CLI user as the previous
plain text `"git subprocess error"` — no UX regression for humans
running `git push`. The typed envelope is for programmatic callers
(the SPA's repo browser, future tooling) that DO parse it.

## Acceptance criteria

- [ ] `info_refs.go`, `upload_pack.go`, and `receive_pack.go` use
      `httperr.Write` instead of `http.Error` for all
      subprocess-failure sites
- [ ] Pre-receive *rejection* responses (`writeReportStatusRejection`)
      are unchanged
- [ ] 400-class responses (bad content-type, malformed push,
      request-too-large) still use `http.Error` or equivalent text
      since those are smart-HTTP protocol responses git clients
      consume directly — alternatively, keep them as plain text for
      now and track in a follow-up; do not regress git-client UX
- [ ] Unit test asserts the typed envelope for subprocess failure
- [ ] `go test ./internal/portal/githttp/...` passes

## Test approach

Audit `handler_test.go` for existing scaffolding that drives the
handler against a real bare repo. Add a test that points the handler
at a path with no `git` binary in PATH (override via `t.Setenv("PATH",
"")` scoped to the test); the subprocess Start fails. Assert envelope.

## Risk

LOW-MEDIUM. The smart-HTTP path is exercised by every plugin push and
fetch; a regression here breaks the plugin's `git push` flow. The
test scaffolding in `handler_test.go` is sufficient to catch
behavioral regressions for the happy path; the dep-failure path is
exercised by the new test.

## Rollback

`git revert`. The old `http.Error` plain-text path is restored.

## Implementation notes

Wired every subprocess-failure site in
`internal/portal/githttp/{info_refs,upload_pack,receive_pack}.go` to emit
the typed `dep.git_subprocess_failed` envelope.

### Subprocess-failure sites covered

- `info_refs.go`: `cmd.Output()` non-zero exit (the single subprocess
  call in this handler — covers both spawn failure and non-zero exit).
- `upload_pack.go`: `cmd.StdoutPipe()` failure and `cmd.Start()` failure.
  Pipe-copy and `cmd.Wait()` errors stay as log-only because response
  headers have already been written by then (the streaming-response
  caveat called out in the constraints).
- `receive_pack.go`: `cmd.StdinPipe()`, `cmd.StdoutPipe()`, and
  `cmd.Start()` failures — all wrapped with `deperr.WrapGitSubprocess`
  and emitted via `httperr.Write(w, r, httperr.ErrGitSubprocessFailed(err))`.
  Post-`WriteHeader` errors (subprocess exit non-zero, post-receive
  events) remain log-only.

### Non-subprocess error sites in receive_pack.go (per story scope)

- `buildValidationRepo` error -> `httperr.Write(w, r, httperr.ErrInternal(err))`
  (in-process pack-parse, not a subprocess; previously emitted
  `"repository unavailable"`).
- `Validator.Validate` error -> `httperr.Write(w, r, httperr.ErrInternal(err))`
  (in-process policy check).
- `Store.GetSession` error -> `httperr.WriteFromError(w, r, deperr.WrapDBIfTransient(err))`.
  This handles the DB-dep classification at the call site so the
  translator picks `dep.db_unavailable` for non-business errors. The
  parent feature's child-story 3 (`portal-dep-failure-error-codes-db`)
  will sweep the rest of the codebase; this one site was wrapped here
  because the file was open and the story body explicitly authorised it.

### Pre-receive rejection + 400-class sites untouched

The `writeReportStatusRejection` path, the "bad content type" 400, the
"malformed push request" 400, and the 413 "pack exceeds size limit"
path all remain as `http.Error`. They're either smart-HTTP protocol
responses git clients consume directly (report-status) or client-input
errors that aren't dep failures. The `unauthorized` 401 from
`AccountFromContext` mismatch also stays — that's an auth fallback, not
a dep failure.

### Test

Added `TestInfoRefs_SubprocessFailure_ReturnsDepGitSubprocessFailed` in
`upload_pack_test.go`. It exercises the full envelope contract:

- Forces a subprocess non-zero exit by skipping bare-repo creation so
  `git upload-pack ... <nonexistent-path>` exits 128.
- Asserts HTTP 500, `Content-Type: application/json; charset=utf-8`,
  no `Retry-After` header, and JSON body
  `{"error": "dep.git_subprocess_failed", "message": "..."}`.

`go test ./internal/portal/githttp/...` passes (19 tests, including the
new one). `go build ./...` passes cleanly.

### Files touched

- `internal/portal/githttp/info_refs.go`
- `internal/portal/githttp/upload_pack.go`
- `internal/portal/githttp/receive_pack.go`
- `internal/portal/githttp/upload_pack_test.go`
