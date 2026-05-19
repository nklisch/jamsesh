---
id: bug-portal-email-from-required-without-magic-link
kind: story
stage: done
tags: [bug, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Portal hard-fails at init when email.from is empty (even without magic-link)

## Brief

Portal hard-fails at init with `email sender init failed: senders: email.from
must not be empty` even when magic-link auth is not configured. This
contradicts the documented contract in `docs/SELF_HOST.md` §6, which states:
"Both [`JAMSESH_EMAIL_PROVIDER` and `JAMSESH_EMAIL_FROM`] are required when
magic-link auth is in use." An OAuth-only deployment, or a deployment that
hasn't yet wired auth at all, should not require an email envelope sender.

Symptom in `.github/workflows/quickstart.yml` CI: the portal binary exits at
startup before `/healthz` binds, every push since v0.1.0. Workaround applied
(commit `117f159`): the workflow now sets `JAMSESH_EMAIL_FROM: ci@localhost`.
The dev `compose.yaml` carries an equivalent workaround
(`JAMSESH_EMAIL_FROM: dev@localhost`). Once this story lands, both
workarounds can be removed.

## Fix approach

Two candidates, in order of preference:

1. **Conditional validation.** Make `email.from` validation conditional on
   `email.provider` being explicitly set (or on magic-link being enabled in
   config). If no provider is configured, skip sender construction entirely
   and route magic-link attempts to a "magic-link not enabled" error path
   at request time.

2. **Lazy sender init.** Defer Sender construction to first magic-link
   request. Risk: errors that should fail-fast at startup (bad SMTP
   credentials, malformed `from` address) now only surface when a user tries
   to log in.

Strong preference for (1) — fail-fast on misconfiguration when magic-link is
enabled, never validate what's not used. Matches the SPEC's `email.provider`
default of `smtp` only being meaningful when an auth flow that uses it is
present.

## Implementation pointers

- Email sender wiring is composed in `cmd/portal/main.go` (or wherever the
  portal init wires `senders` — the email-senders skill carries the Sender
  interface contract). Read the existing wiring path first; the validation
  that emits the `senders: email.from must not be empty` error is the call
  site to make conditional.
- Concrete config gating: an OAuth-only deployment has
  `JAMSESH_OAUTH_GITHUB_CLIENT_ID` set and no `JAMSESH_EMAIL_PROVIDER`. The
  intended user state should be "magic-link disabled" — skip sender init
  cleanly.

## Acceptance criteria

- [ ] Portal starts cleanly with `JAMSESH_OAUTH_GITHUB_CLIENT_ID` set and
      `JAMSESH_EMAIL_FROM`/`JAMSESH_EMAIL_PROVIDER` unset.
- [ ] Portal starts cleanly with neither OAuth nor email configured (auth is
      simply unavailable; portal serves `/healthz` and `/readyz`).
- [ ] Portal still fails fast at startup when `JAMSESH_EMAIL_PROVIDER` is
      set but `JAMSESH_EMAIL_FROM` is missing — i.e. fix doesn't loosen
      validation when email IS in use.
- [ ] A magic-link request against a portal without email configured returns
      a clear error (e.g. `auth.magic_link_not_enabled`) rather than a 5xx.
- [ ] `JAMSESH_EMAIL_FROM` workaround removed from
      `.github/workflows/quickstart.yml`.
- [ ] `JAMSESH_EMAIL_FROM` workaround removed from root `compose.yaml`.
- [ ] Regression test: portal start under each of the four configurations
      above (OAuth-only / no-auth / email-only / email-misconfigured) covered
      by a unit or integration test.

## Notes

- The fix path includes removing the CI workaround at commit `117f159` — the
  workaround is the canary; the test for this story's success is that CI
  goes green WITHOUT the workaround.
- Touches `internal/email/` or equivalent + `cmd/portal/main.go` init
  wiring. May also touch `internal/auth/` magic-link request handler to
  return the new "not enabled" error.

## Implementation Notes

### Files touched

- `internal/portal/senders/sender.go` — added `ErrMagicLinkNotEnabled` sentinel
- `internal/portal/senders/factory.go` — added `disabledSender` type; factory returns it when both `Provider` and `From` are empty; preserves fail-fast when `Provider` is set and `From` is empty
- `internal/portal/config/config.go` — removed `Provider: "smtp"` from defaults so a bare install has both fields empty and hits the disabled path
- `internal/portal/httperr/httperr.go` — added `ErrMagicLinkNotEnabled()` constructor (400, code `auth.magic_link_not_enabled`)
- `internal/portal/auth/magic_link.go` — added `errors.Is(err, senders.ErrMagicLinkNotEnabled)` guard before `deperr.WrapSMTP`; returns `httperr.ErrMagicLinkNotEnabled()` directly so the pipeline emits 400 instead of 503
- `internal/portal/sessions/invites.go` — skip email send (not hard-fail) when `ErrMagicLinkNotEnabled`; invite still created and returned so hosts can copy the link
- `internal/portal/accounts/orgs.go` — same skip pattern for org invites
- `internal/portal/senders/sender_test.go` — updated `TestNew_EmptyFrom_ReturnsError` comment; added `TestNew_NoEmailConfig_ReturnsDisabledSender` (matrix 1+2), `TestNew_ProviderSetFromEmpty_ReturnsError` (matrix 4)
- `internal/portal/auth/magic_link_test.go` — added `TestRequestMagicLink_DisabledSender_Returns400MagicLinkNotEnabled`
- `.github/workflows/quickstart.yml` — removed `JAMSESH_EMAIL_FROM: ci@localhost` workaround and explanatory comment
- `compose.yaml` — removed `JAMSESH_EMAIL_FROM: dev@localhost` workaround and explanatory comment

### Approach: conditional construction (Path 1)

`disabledSender` is a zero-value struct implementing `Sender`. `New` returns it with no error when both `cfg.Provider` and `cfg.From` are empty (entirely unconfigured). The disabled state is detected at send-time via `ErrMagicLinkNotEnabled` rather than at startup, which avoids the need to thread a boolean flag through all callers.

Deviation from recommended shape: `translate.go` was NOT updated to add a new `senders` import — instead, `magic_link.go` catches `ErrMagicLinkNotEnabled` before wrapping with `deperr.WrapSMTP` and returns an `httperr.Error` directly. This is cleaner (no cross-package import in the translate pipeline) and consistent with how auth middleware already returns `httperr.Error` values directly.

### Test coverage matrix

1. OAuth-only (provider/from unset): `TestNew_NoEmailConfig_ReturnsDisabledSender` confirms disabled sender; `TestRequestMagicLink_DisabledSender_Returns400MagicLinkNotEnabled` confirms 400 response.
2. No-auth (everything unset): same as matrix 1 — covered by above.
3. Email-only (provider+from set): existing `TestNew_SMTP_ValidConfig_ReturnsSender` (and friends) confirm working sender.
4. Email-misconfigured (provider set, from empty): `TestNew_EmptyFrom_ReturnsError` + `TestNew_ProviderSetFromEmpty_ReturnsError` confirm fail-fast preserved.

### Workaround removal confirmed

- `.github/workflows/quickstart.yml` — removed `JAMSESH_EMAIL_FROM: ci@localhost` and explanatory comment
- `compose.yaml` — removed `JAMSESH_EMAIL_FROM: dev@localhost` and explanatory comment

Full `go test ./...` passes with no failures. `go vet ./...` clean.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none

**Important** (inline-fixed):
- **Foundation-doc drift in `docs/SELF_HOST.md` §6**. The env-var reference table listed `JAMSESH_EMAIL_PROVIDER` default as `smtp`, but `internal/portal/config/config.go` removed that default as part of this fix. Per rolling-foundation, SELF_HOST must describe the system as it is NOW. **Resolution**: updated the table entry to default `_(none)_` with an explanatory note about OAuth-only deployments and the `400 auth.magic_link_not_enabled` response. Also tightened the `JAMSESH_EMAIL_FROM` row ("Required when `email.provider` is set") and removed the "(default)" subheading from the SMTP section since SMTP is no longer the default provider.

**Nits** (inline notes only):
- Invite email skip behavior (sessions/invites.go, accounts/orgs.go) is silent — when email is disabled, the invite is created and the link is returned in the API response, but the recipient gets no automatic email. Operators in OAuth-only mode need to copy invite links manually. Not documented in SELF_HOST yet — a future polish item, not blocking. Worth a one-line note in §6 or a dedicated invites-without-email subsection, but doesn't block this fix.

**Notes**:
- Test integrity: all four config matrices covered (factory + magic-link 4xx response). `go test ./internal/portal/...` clean.
- The deviation from the recommended fix shape (handling `ErrMagicLinkNotEnabled` in `magic_link.go` rather than threading a senders import through `translate.go`) is cleaner and justified — keeps the `httperr` translate pipeline import-clean.
- Workarounds removed from both `quickstart.yml` and `compose.yaml` per acceptance criteria. The canary works: portal starts cleanly with no email env vars.
- Security: no auth surface widened. Existing fail-fast for "provider set, from empty" is preserved.
