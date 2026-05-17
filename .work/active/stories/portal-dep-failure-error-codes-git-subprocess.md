---
id: portal-dep-failure-error-codes-git-subprocess
kind: story
stage: implementing
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
