---
id: bug-portal-email-from-required-without-magic-link
created: 2026-05-18
tags: [bug, portal]
---

Portal hard-fails at init with `email sender init failed: senders: email.from
must not be empty` even when magic-link auth is not configured. This
contradicts the documented contract in `docs/SELF_HOST.md` §6, which states:
"Both [`JAMSESH_EMAIL_PROVIDER` and `JAMSESH_EMAIL_FROM`] are required when
magic-link auth is in use." An OAuth-only deployment, or a deployment that
hasn't yet wired auth at all, should not require an email envelope sender.

Symptom in `.github/workflows/quickstart.yml` CI: the portal binary exits at
startup before `/healthz` binds, every push since v0.1.0. Workaround applied:
the workflow now sets `JAMSESH_EMAIL_FROM: ci@localhost` (commit fixing
quickstart). The dev `compose.yaml` carries an equivalent dev-only workaround
(`JAMSESH_EMAIL_FROM: dev@localhost`).

Fix candidates: (1) make email sender init lazy — only construct the Sender
on first magic-link request; (2) make `email.from` validation conditional on
`email.provider` being non-empty AND magic-link being enabled by config. Likely
located near the portal init wiring under `internal/email/` or wherever
`senders` is composed in `cmd/portal/main.go`.

Also relevant: this is what's caused the `quickstart` GitHub workflow to fail
red on every push since v0.1.0 shipped. The CI workaround unblocks but masks
the underlying defect — fixing this lets operators run an OAuth-only setup
without setting a dummy email.
