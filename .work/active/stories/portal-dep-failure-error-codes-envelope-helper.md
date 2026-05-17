---
id: portal-dep-failure-error-codes-envelope-helper
kind: story
stage: implementing
tags: [portal]
parent: portal-dep-failure-error-codes
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# `dep.*` sentinel package + strict-handler translator

Adds the foundational plumbing: a new `internal/portal/deperr/` package
that declares the four dep-class sentinel errors and shallow
wrap helpers, extends `internal/portal/httperr/` with four typed
constructors and a `WriteFromError` translator, and wires a custom
`ResponseErrorHandlerFunc` on the oapi-codegen strict handler in
`cmd/portal/main.go`.

No call sites are migrated by this story — the wrappers are introduced
as a no-op surface so subsequent stories (auth-smtp, db, oauth,
git-subprocess) can wrap their dep-touching sites with confidence the
translator picks them up.

## Files

- **New** `internal/portal/deperr/deperr.go`:

  ```go
  // Package deperr declares sentinel errors that mark a request failure
  // as caused by an external dependency (SMTP, DB, OAuth provider, git
  // subprocess) rather than a business-logic problem. Handlers wrap
  // dep-class failures with the helpers here; the strict-handler
  // translator in httperr classifies them into typed envelopes.
  package deperr

  import (
      "errors"
      "fmt"

      "jamsesh/internal/db/store"
  )

  var (
      ErrSMTP          = errors.New("dep: smtp unavailable")
      ErrDB            = errors.New("dep: database unavailable")
      ErrOAuthProvider = errors.New("dep: oauth provider unavailable")
      ErrGitSubprocess = errors.New("dep: git subprocess failed")
  )

  // WrapSMTP marks err as an SMTP-dep failure.
  func WrapSMTP(err error) error {
      if err == nil { return nil }
      return fmt.Errorf("%w: %v", ErrSMTP, err)
  }

  // WrapDB marks err as a DB-dep failure unconditionally. Prefer
  // WrapDBIfTransient at call sites where a known business sentinel
  // (ErrNotFound / ErrUniqueViolation) may also be the value.
  func WrapDB(err error) error {
      if err == nil { return nil }
      return fmt.Errorf("%w: %v", ErrDB, err)
  }

  // WrapDBIfTransient returns err unchanged when it is a recognized
  // business-class store sentinel; otherwise it wraps as ErrDB.
  func WrapDBIfTransient(err error) error {
      if err == nil { return nil }
      if errors.Is(err, store.ErrNotFound) ||
          errors.Is(err, store.ErrUniqueViolation) {
          return err
      }
      return WrapDB(err)
  }

  // WrapOAuthProvider marks err as an OAuth-provider HTTP failure.
  func WrapOAuthProvider(err error) error {
      if err == nil { return nil }
      return fmt.Errorf("%w: %v", ErrOAuthProvider, err)
  }

  // WrapGitSubprocess marks err as a git-subprocess failure.
  func WrapGitSubprocess(err error) error {
      if err == nil { return nil }
      return fmt.Errorf("%w: %v", ErrGitSubprocess, err)
  }
  ```

- **Edit** `internal/portal/httperr/httperr.go` — add four constructors
  and the translator. The pattern matches the existing
  `ErrInvalidToken()` etc.:

  ```go
  func ErrSMTPUnavailable(cause error) *Error {
      return &Error{
          Code:       "dep.smtp_unavailable",
          Message:    "email delivery is currently unavailable",
          HTTPStatus: http.StatusServiceUnavailable,
          Wrapped:    cause,
          Headers:    map[string]string{"Retry-After": "5"},
      }
  }

  func ErrDBUnavailable(cause error) *Error {
      return &Error{
          Code:       "dep.db_unavailable",
          Message:    "database is currently unavailable",
          HTTPStatus: http.StatusServiceUnavailable,
          Wrapped:    cause,
          Headers:    map[string]string{"Retry-After": "2"},
      }
  }

  func ErrOAuthProviderUnavailable(cause error) *Error {
      return &Error{
          Code:       "dep.oauth_provider_unavailable",
          Message:    "OAuth provider is currently unavailable",
          HTTPStatus: http.StatusServiceUnavailable,
          Wrapped:    cause,
          Headers:    map[string]string{"Retry-After": "10"},
      }
  }

  func ErrGitSubprocessFailed(cause error) *Error {
      return &Error{
          Code:       "dep.git_subprocess_failed",
          Message:    "git subprocess failed",
          HTTPStatus: http.StatusInternalServerError,
          Wrapped:    cause,
      }
  }
  ```

  The existing `Error` struct needs a `Headers map[string]string`
  field; `Write` writes any non-empty entries onto `w.Header()` before
  calling `WriteHeader`. (Audit `httperr.go` first — if `Headers` is
  already there, skip this part.)

- **New** `internal/portal/httperr/translate.go`:

  ```go
  package httperr

  import (
      "errors"
      "net/http"

      "jamsesh/internal/portal/deperr"
  )

  // WriteFromError translates any handler-returned error into the
  // canonical envelope:
  //
  //  1. *Error -> use it directly.
  //  2. deperr.Err* sentinel match -> typed dep envelope.
  //  3. Anything else -> ErrInternal (preserves today's "internal" 500).
  //
  // Wired as the strict-handler ResponseErrorHandlerFunc in
  // cmd/portal/main.go.
  func WriteFromError(w http.ResponseWriter, r *http.Request, err error) {
      var e *Error
      if errors.As(err, &e) {
          Write(w, r, e)
          return
      }
      switch {
      case errors.Is(err, deperr.ErrSMTP):
          Write(w, r, ErrSMTPUnavailable(err))
      case errors.Is(err, deperr.ErrDB):
          Write(w, r, ErrDBUnavailable(err))
      case errors.Is(err, deperr.ErrOAuthProvider):
          Write(w, r, ErrOAuthProviderUnavailable(err))
      case errors.Is(err, deperr.ErrGitSubprocess):
          Write(w, r, ErrGitSubprocessFailed(err))
      default:
          Write(w, r, ErrInternal(err))
      }
  }
  ```

- **Edit** `cmd/portal/main.go`:

  Replace the current strict-handler construction
  (`openapi.NewStrictHandler(combined, nil)`) with
  `openapi.NewStrictHandlerWithOptions(combined, nil,
  openapi.StrictHTTPServerOptions{ResponseErrorHandlerFunc:
  httperr.WriteFromError, RequestErrorHandlerFunc:
  httperr.WriteBadRequest})`.

  `RequestErrorHandlerFunc` today writes plain-text 400; replace it
  too with a small helper `httperr.WriteBadRequest(w, r, err)` that
  emits `{error: "request.malformed", message: err.Error()}` at 400 —
  this is a small consistency win that lives naturally with this
  change (still typed-envelope on every error response). Add the
  helper in `httperr.go` alongside the existing constructors.

- **New** `internal/portal/deperr/deperr_test.go`:
  - `WrapDBIfTransient(nil) == nil`
  - `WrapDBIfTransient(store.ErrNotFound)` returns unchanged (no wrap)
  - `WrapDBIfTransient(store.ErrUniqueViolation)` returns unchanged
  - `WrapDBIfTransient(errors.New("conn refused"))` wraps as ErrDB
  - `errors.Is(WrapSMTP(...), deperr.ErrSMTP) == true`
  - Likewise for each sentinel

- **New** `internal/portal/httperr/translate_test.go`:
  Table-driven test of `WriteFromError` against every sentinel and a
  default fallthrough:

  | input err                                          | want code                       | want status | want Retry-After |
  |----------------------------------------------------|---------------------------------|-------------|------------------|
  | `deperr.WrapSMTP(errors.New("x"))`                 | `dep.smtp_unavailable`          | 503         | `5`              |
  | `deperr.WrapDB(errors.New("x"))`                   | `dep.db_unavailable`            | 503         | `2`              |
  | `deperr.WrapOAuthProvider(errors.New("x"))`        | `dep.oauth_provider_unavailable`| 503         | `10`             |
  | `deperr.WrapGitSubprocess(errors.New("x"))`        | `dep.git_subprocess_failed`     | 500         | ""               |
  | `httperr.ErrInvalidToken()`                         | `auth.invalid_token`            | 401         | ""               |
  | `errors.New("anything else")`                       | `internal`                      | 500         | ""               |

  Each row decodes the JSON body and asserts on `error` field +
  `Content-Type: application/json; charset=utf-8`.

## Acceptance criteria

- [ ] `internal/portal/deperr/` package compiles with the four
      sentinels and four wrap helpers
- [ ] `internal/portal/httperr/` exposes
      `Err{SMTP,DB,OAuthProvider}Unavailable(cause)`,
      `ErrGitSubprocessFailed(cause)`, and `WriteFromError(w, r, err)`
- [ ] `*httperr.Error` carries optional `Headers` and `Write` writes
      them before status (specifically `Retry-After`)
- [ ] `cmd/portal/main.go` constructs the strict handler with
      `NewStrictHandlerWithOptions(... WriteFromError ...)`
- [ ] Unit tests for the translator cover every sentinel + fallthrough
- [ ] `go build ./...` clean
- [ ] `go test ./internal/portal/deperr/... ./internal/portal/httperr/...` passes
- [ ] No call site outside `httperr/` writes errors directly with
      `http.Error` for handler returns (this story doesn't migrate
      git-subprocess yet — that's story 5)

## Test approach

Pure unit. The translator is a small switch; tests run against an
`httptest.NewRecorder` and inspect status + headers + JSON body.

## Risk

LOW. Additive surface only. Existing handlers continue to return
plain `fmt.Errorf` wrapped errors that fall through to `ErrInternal`
(today's behavior). The contract widens only for handlers that adopt
the new wrappers in subsequent stories.

## Rollback

`git revert` the commit. No data migrations involved.
