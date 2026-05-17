---
id: epic-distribution-self-host-docs
kind: feature
stage: drafting
tags: [infra]
parent: epic-distribution
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Distribution — Self-Host Docs

## Brief

The operator-facing documentation for self-hosting a jamsesh portal.
Two artifacts:

- **`README.md`** at repo root — the GitHub landing page. Quick-start
  (5-minute path from `git clone` or `docker pull` to a running
  portal serving on localhost), licensing (Apache 2.0), a one-line
  link to `docs/SELF_HOST.md` for the full operator guide.
- **`docs/SELF_HOST.md`** — the full operator reference. Sections:
  - **Install** — binary download + verify signature, Docker image
    pull + verify, systemd unit example
  - **Configuration** — full reference for env vars and YAML
    config file (bind address, TLS certs, DB driver + connection
    string, storage path for bare repos, OAuth provider configs,
    email provider configs, log level, retention windows)
  - **TLS** — native HTTPS with cert paths vs HTTP-behind-trusted-
    proxy mode; example reverse-proxy config (Caddy + nginx);
    Let's Encrypt setup notes
  - **OAuth callback URLs** — registering with GitHub (and later
    Google/OIDC providers); how to configure the portal's expected
    redirect URI
  - **Database** — SQLite vs Postgres trade-offs, backup/restore
    flows for each, migration discipline (sqlc-generated; releases
    include migration SQL)
  - **Email** — provider selection (SMTP / SendGrid / Postmark /
    Resend) and configuration for magic-link delivery
  - **Bare-repo storage** — disk usage estimates, backup strategy
    (just back up the storage directory), retention policy notes
  - **Monitoring** — log format, useful metrics (sessions active,
    events emitted per second, push success rate, auto-merger
    backlog)
  - **Upgrade procedure** — stop service, replace binary, restart;
    when migrations are required; rollback notes
  - **Security posture** — what a portal breach exposes (cross-ref
    SECURITY.md), token rotation, threat model summary
  - **Troubleshooting** — common error codes from the JSON error
    contract and what they mean operationally

**Tested quickstart**: a CI job (deferred to feature-design — could
land here or in build-pipeline) that spins up the portal in a
container, runs `curl /healthz`, posts a smoke-test request through
each major auth flow. The README's quickstart is what the CI test
runs — keeps the install steps honest.

**Maintenance discipline**: when the binary's config flags change,
the gate-docs skill at release-deploy time flags SELF_HOST.md drift.
Operators rely on this for production setups.

Does NOT cover developer-facing docs (those live in the foundation
docs in `docs/`). Does NOT cover marketplace-side documentation —
the marketplace repo has its own README authored by the `marketplace`
feature.

## Epic context

- Parent epic: `epic-distribution`
- Position in epic: independent; no dependencies on other features
  in this epic. Can land any time once the portal binary is
  buildable.

## Foundation references

- `docs/SPEC.md` — Deployment shape, Hard constraints (self-host-
  capable), What's explicitly deferred
- `docs/SECURITY.md` — Self-host security posture (the canonical
  list of operator responsibilities), Supply chain and integrity
- `docs/ARCHITECTURE.md` — Portal component overview, Data store

## Inherited epic design decisions

- **Docs location**: `README.md` + `docs/SELF_HOST.md`.
- **License**: Apache 2.0 — referenced in README.

## Decomposition risks

- **Self-host docs drift.** Operators rely on these in production;
  if config flags change without doc updates, real outages happen.
  Mitigation: tested-quickstart CI job keeps the install steps
  honest; gate-docs skill catches drift at release time.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
