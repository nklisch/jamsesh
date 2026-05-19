---
id: bug-portal-email-from-required-without-magic-link
kind: story
stage: implementing
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
