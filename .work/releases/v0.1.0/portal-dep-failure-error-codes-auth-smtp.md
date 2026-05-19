---
id: portal-dep-failure-error-codes-auth-smtp
kind: story
stage: done
tags: [portal]
parent: portal-dep-failure-error-codes
depends_on: [portal-dep-failure-error-codes-envelope-helper]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Wire SMTP dep failures to `dep.smtp_unavailable`

Wraps every `senders.Sender.Send` error site so the response envelope
carries `error: "dep.smtp_unavailable"` at HTTP 503 with a
`Retry-After: 5` hint. Covers magic-link, org invites, and session
invites.

## Files

- **Edit** `internal/portal/auth/magic_link.go`:

  Current code:

  ```go
  if err := h.sender.Send(ctx, email, magicLinkSubject, body); err != nil {
      return nil, fmt.Errorf("magic-link: send email: %w", err)
  }
  ```

  Target:

  ```go
  if err := h.sender.Send(ctx, email, magicLinkSubject, body); err != nil {
      return nil, deperr.WrapSMTP(fmt.Errorf("magic-link: send email: %w", err))
  }
  ```

  The inner `fmt.Errorf` is preserved so the operator log still has
  the "magic-link: send email:" context.

- **Edit** `internal/portal/accounts/orgs.go` — `CreateOrgInvite`. Find
  the `h.sender.Send(...)` call (around line ~80-100; grep
  `"sender.Send"` in this file). Apply the same wrap pattern.

- **Edit** `internal/portal/sessions/invites.go` — `InviteToSession`.
  Same pattern.

- **Edit** `internal/portal/auth/magic_link_test.go` —
  `TestRequestMagicLink_SenderError_Returns500`. Update the assertion:
  - Expect HTTP **503** (not 500).
  - Expect `Content-Type: application/json; charset=utf-8`.
  - Decode body, expect `{error: "dep.smtp_unavailable"}`.
  - Expect `Retry-After: 5` header.

  Rename the test to
  `TestRequestMagicLink_SenderError_Returns503DepSMTPUnavailable`.

- **Edit** equivalent tests for org-invite and session-invite if they
  exist. Audit before editing:
  - `internal/portal/accounts/orgs_test.go` — search for any test
    asserting on sender-error behavior; if none exists, add one.
  - `internal/portal/sessions/invites_test.go` — same.

## Sentinel discipline note

Audit `senders/{smtp,sendgrid,postmark,resend}.go` to confirm each
provider impl wraps with `senders.ErrTransient` / `senders.ErrAuth` /
`senders.ErrPermanent` per the package contract. The wrap helper
treats *all* sender errors as dep failures — including `ErrPermanent`
(bad recipient) — because the magic-link handler has no other
recourse: an invalid-email-address kind of failure SHOULD have been
caught by request validation before reaching the sender. If audit
finds providers returning bare `error` without wrapping, file a
backlog item via `/agile-workflow:park` to fix the sender impl; this
story does not block on it.

## Acceptance criteria

- [ ] Magic-link sender failure returns 503 with
      `{error: "dep.smtp_unavailable"}` and `Retry-After: 5`
- [ ] Org-invite sender failure returns same shape
- [ ] Session-invite sender failure returns same shape
- [ ] Existing magic-link `_Returns500` test renamed and updated to
      assert on the typed envelope
- [ ] `go test ./internal/portal/auth/... ./internal/portal/accounts/... ./internal/portal/sessions/...` passes

## Test approach

Reuse the existing `magic_link_test.go` `failingSender` test double —
a `Sender` impl whose `Send` returns
`fmt.Errorf("%w: forced", senders.ErrTransient)`. Inject it into the
test env, POST `/api/auth/magic-link/request`, decode body, assert.

## Risk

LOW. Single-call-site wrap; the only behavior change visible to
callers is the status code (500 -> 503) and the body shape (text ->
JSON envelope). Both are improvements the PROTOCOL.md contract was
already promising.

## Rollback

`git revert`; restores the `_Returns500` plain-text assertion.

## Implementation notes

### Files touched

- `internal/portal/auth/magic_link.go` — added `deperr` import; the
  `RequestMagicLink` `sender.Send` failure path is now wrapped with
  `deperr.WrapSMTP(fmt.Errorf("magic-link: send email: %w", err))`.
  Operator log still carries the original context; the translator
  surfaces the typed envelope. The `Clock` interface threading from
  `portal-test-clock-advance-endpoint-clock-abstraction` was preserved
  untouched.
- `internal/portal/accounts/orgs.go` — same wrap on the
  `CreateOrgInvite` `h.sender.Send` call site.
- `internal/portal/sessions/invites.go` — same wrap on the
  `InviteToSession` `h.sender.Send` call site.

### Test updates

- `internal/portal/auth/magic_link_test.go`:
  - Magic-link strict handlers (both `newMagicLinkTestEnv` and the
    inline expired-token env) now use `NewStrictHandlerWithOptions`
    with `ResponseErrorHandlerFunc: httperr.WriteFromError` and
    `RequestErrorHandlerFunc: httperr.WriteBadRequest`, mirroring
    production wiring in `cmd/portal/main.go`. Without this the
    default oapi-codegen plain-text 500 path swallows the dep
    sentinel.
  - `TestRequestMagicLink_SenderError_Returns500` renamed to
    `TestRequestMagicLink_SenderError_Returns503DepSMTPUnavailable`
    and now asserts HTTP 503, `Content-Type: application/json;
    charset=utf-8`, `Retry-After: 5`, and `error =
    dep.smtp_unavailable` in the decoded body.

- `internal/portal/accounts/orgs_test.go`:
  - `captureSenderOrgs` gained an injectable `err` field.
  - `newOrgsMembersTestEnv` now wires `WriteFromError` /
    `WriteBadRequest` via `NewStrictHandlerWithOptions`. The
    `ServerInterfaceWrapper`'s path-param `ErrorHandlerFunc` now routes
    through `httperr.WriteBadRequest` too for consistency.
  - Added `TestCreateOrgInvite_SenderError_Returns503DepSMTPUnavailable`.

- `internal/portal/sessions/handler_test.go`:
  - `stubSender` gained an injectable `err` field; `testEnv` exposes
    the `sender` pointer.
  - `newTestEnv` uses `NewStrictHandlerWithOptions` with the same
    wiring.
- `internal/portal/sessions/invites_test.go`:
  - Added `TestInviteToSession_SenderError_Returns503DepSMTPUnavailable`.

### Provider sentinel audit

Verified every in-tree `senders` provider correctly wraps with
`senders.ErrTransient` / `senders.ErrAuth` / `senders.ErrPermanent`:

- `senders/smtp.go` — `classifySMTPError` maps 535 -> ErrAuth,
  4xx -> ErrTransient, 5xx -> ErrPermanent, network -> ErrTransient.
  Client-init failures map to ErrAuth, address-validation to ErrPermanent.
- `senders/resend.go` — `classifyResendError` falls back to ErrTransient
  for non-classifiable errors and maps the rate-limit sentinel.
- `senders/postmark.go` — `classifyPostmarkErrorCode` maps the canonical
  Postmark codes to the three sentinels, with sensible fallbacks for the
  generic 4xx/5xx ranges.
- `senders/sendgrid.go` — `classifySendGridStatus` maps 401/403 to ErrAuth,
  429/5xx to ErrTransient, other 4xx to ErrPermanent.

No sentinel gaps to park — every provider returns sentinel-wrapped errors
on every failure path.

### Test results

`go test ./internal/portal/auth/... ./internal/portal/accounts/...
./internal/portal/sessions/...` — all three packages PASS.
`go build ./...` and `go vet ./internal/portal/...` — clean.

The three new sender-failure tests assert end-to-end: stub Sender
returns `senders.ErrTransient`-wrapped error -> handler wraps with
`deperr.WrapSMTP` -> strict handler routes through
`httperr.WriteFromError` -> response is HTTP 503 with `Content-Type:
application/json; charset=utf-8`, `Retry-After: 5`, and body
`{"error":"dep.smtp_unavailable","message":"email delivery is
currently unavailable"}`.

## Review (2026-05-17)

**Verdict: Approve.**

Cross-checked production wraps in `magic_link.go:111-112`,
`accounts/orgs.go:94-95`, and `sessions/invites.go:122-123` — all three
`Sender.Send` failures wrap with `deperr.WrapSMTP(fmt.Errorf(...))`.
Operator-log context preserved via inner `fmt.Errorf`.

Grep audit confirms exactly 3 non-test, non-sender `.Send(` call sites
in `internal/portal/` — all covered. No other unwrapped Sender call
sites exist.

Provider sentinel audit verified directly: all four sender impls
(`smtp.go`, `resend.go`, `postmark.go`, `sendgrid.go`) wrap every
failure path with `ErrTransient`/`ErrAuth`/`ErrPermanent` via the
canonical `fmt.Errorf("%w: ...", senders.Err*, ...)` pattern. No
provider-gap backlog needed.

Three new `_SenderError_Returns503DepSMTPUnavailable` tests assert the
full envelope contract (status, Content-Type, Retry-After, error code).
Strict-handler test envs in all three packages correctly wire
`httperr.WriteFromError` via `NewStrictHandlerWithOptions`.

`go test ./internal/portal/{auth,accounts,sessions}/...` — green
(cached).

**Split-commit note (no penalty):** Production wraps landed in
`ce0de70` (swept into the sibling db story's commit during a wave-2b
parallel-agent race); tests + story body landed in recovery commit
`2bd489a`. Substrate state is correct — story's own acceptance criteria
are all met. The race itself is a workflow concern outside this story's
scope.

Findings: blockers 0, important 0, nits 0. No items parked.

Advancing review -> done.
