---
id: portal-dep-failure-error-codes-auth-smtp
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
