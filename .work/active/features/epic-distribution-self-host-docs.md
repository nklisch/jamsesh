---
id: epic-distribution-self-host-docs
kind: feature
stage: done
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

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **README scope**: deliberately small. Identity (one paragraph),
  License (Apache 2.0), Quickstart (5 minutes from zero to portal
  serving), and a prominent link to `docs/SELF_HOST.md` for the full
  operator guide. Project-status badges, contribution guidelines,
  and feature-tour content are out of scope for v1 — `README.md` is
  for landing-page legibility, not marketing.
- **Quickstart pathway**: Docker is the primary path
  (`docker run ghcr.io/<owner>/jamsesh:latest`); binary download
  + signature verify is documented but secondary. Reason: Docker is
  one command; the binary path requires multiple steps (download,
  verify cosign sig, install systemd, configure TLS).
- **Tested-quickstart CI**: a separate workflow file
  `.github/workflows/quickstart.yml` triggered on PRs to `main`.
  Builds the portal locally (`go build ./cmd/portal`), runs it with
  default config in behind-proxy mode on `127.0.0.1:18443`,
  `curl /healthz`, asserts `200`. Deeper smoke tests (auth flow,
  push) come as feature implementations land — for now the
  healthcheck is the entire scope.
- **SELF_HOST.md as the canonical config reference**: every config
  flag introduced in `internal/portal/config/config.go` MUST appear
  in SELF_HOST.md's Configuration table. The gate-docs skill flags
  drift at release time. The Configuration section is the
  living-spec for the env-var + YAML surface.
- **OAuth callback URL stanza**: cross-references the GitHub-OAuth
  callback shape that `epic-portal-foundation-auth-flows` locks. The
  doc author works from that feature's design body, even though the
  implementation hasn't landed.
- **Two-story decomposition**: split docs (story 1, no binary
  dependency) from CI test (story 2, depends on the portal binary
  being buildable). Lets the docs land early while the CI test
  waits for `epic-portal-foundation-http-skeleton-config-tls-and-entry`.

## Architectural choice

Documentation feature; "architecture" is the section topology of
SELF_HOST.md and the placement of the CI smoke test.

**Section ordering rule**: Quickstart first (one-page success path),
then Configuration (the longest, table-driven), then per-concern
deep-dives in the order the operator hits them during install →
operation → upgrade → incident response. Avoid alphabetical or
strictly-grouped section ordering; operators read top-to-bottom on
first install.

## Implementation Units

### Unit 1: README.md

**File**: `README.md` (repo root)
**Story**: `epic-distribution-self-host-docs-readme-and-self-host`

Sections (skeleton):

```markdown
# jamsesh

> Multi-agent jamming for codebases — coordinated Claude Code sessions
> producing PR-shaped branches without merge headaches.

License: Apache 2.0 · See [docs/SELF_HOST.md](docs/SELF_HOST.md) for the
full operator guide.

## What it is

(2-paragraph description aligned with VISION.md)

## Quickstart (Docker)

\`\`\`bash
docker run --rm -p 8443:8443 \\
  -e JAMSESH_TLS_MODE=behind_proxy \\
  -e JAMSESH_BIND=:8443 \\
  -v $(pwd)/data:/data \\
  ghcr.io/<owner>/jamsesh:latest
curl http://localhost:8443/healthz
# → {"status":"ok"}
\`\`\`

For TLS, OAuth, database options, and production deployment, see
[docs/SELF_HOST.md](docs/SELF_HOST.md).

## License

Apache 2.0 — see [LICENSE](LICENSE).
```

### Unit 2: docs/SELF_HOST.md

**File**: `docs/SELF_HOST.md`
**Story**: `epic-distribution-self-host-docs-readme-and-self-host`

Section outline (each section is a substantive 1-5 paragraphs +
optional table / shell block):

```markdown
# Self-hosting jamsesh

## 1. Install

### Docker (recommended)

(image pull, image-tag conventions, GHCR location, image signature
verification with `cosign verify`)

### Binary

(download from GitHub releases, sha256 + cosign signature verify
recipe, place in /usr/local/bin, systemd unit example)

## 2. Configuration

A complete reference for the env-var + YAML surface. Single
authoritative table:

| Env var | YAML key | Default | Purpose |
|---|---|---|---|
| `JAMSESH_BIND` | `bind` | `:8443` | listen address |
| `JAMSESH_DB_DRIVER` | `db_driver` | `sqlite` | `sqlite` or `postgres` |
| `JAMSESH_DB_DSN` | `db_dsn` | `./jamsesh.db` | DSN for the driver |
| `JAMSESH_TLS_MODE` | `tls.mode` | `behind_proxy` | `native` or `behind_proxy` |
| `JAMSESH_TLS_CERT` | `tls.cert_path` |  | required when `tls.mode=native` |
| `JAMSESH_TLS_KEY` | `tls.key_path` |  | required when `tls.mode=native` |
| `JAMSESH_LOG_FORMAT` | `log.format` | `json` | `json` or `text` |
| `JAMSESH_LOG_LEVEL` | `log.level` | `0` (Info) | slog level (-4=Debug, 0=Info, 4=Warn, 8=Error) |
| `JAMSESH_STORAGE` | `storage` | `./storage` | path for per-session bare repos |
| (OAuth + email entries land as auth-flows ships) |

Plus a complete example YAML config file.

## 3. TLS

(when to pick `native` vs `behind_proxy`; example Caddyfile +
nginx.conf for the proxied case; Let's Encrypt notes for native)

## 4. OAuth callback URLs

(register the portal with GitHub OAuth at `https://<host>/auth/github/callback`;
Google/OIDC mention with "added in a future release"; how the portal
discovers its callback URL — config-driven)

## 5. Database

(SQLite vs Postgres trade-offs: single-process default vs
horizontal-scaling. Backup procedure for each. Migration discipline:
sqlc-generated, runs on portal startup, idempotent.)

## 6. Email (magic-link delivery)

(pluggable provider abstraction. Configure SMTP for self-host
default; SendGrid / Postmark / Resend as alternatives. Per-provider
env var matrix.)

## 7. Bare-repo storage

(disk usage estimate: ~20-50 MB per active session in typical doc
jams. Back up the entire `storage/` directory. Retention: 90 days
after session end (per SPEC.md), then archived.)

## 8. Monitoring

(JSON log format reference, useful slog attribute names. Future
section for Prometheus metrics endpoint once added.)

## 9. Upgrade procedure

(`docker pull` + `docker restart` or stop service + replace binary
+ restart. Database migrations run on startup; downgrade requires
restoring a pre-upgrade backup of the database.)

## 10. Security posture

(short cross-reference to docs/SECURITY.md; the operator-facing
summary: TLS termination is the operator's job in `behind_proxy`
mode, OAuth tokens stored hashed at rest, refresh-token revocation
propagates within 1 minute, full breach impact in SECURITY.md.)

## 11. Troubleshooting

(table of common JSON error codes from docs/PROTOCOL.md > HTTP
error contract, with operational interpretation:)

| Error code | Operator action |
|---|---|
| `auth.invalid_token` | client clock skew / OAuth misconfiguration |
| `auth.expired_token` | refresh issuance failing — check OAuth config |
| `push.scope_violation` | session writable_scope is too narrow for the user's commit |
| `push.ref_namespace_violation` | client misconfigured to push outside its namespace |
| `push.missing_trailer` | client plugin version mismatch — upgrade `jamsesh` plugin |
```

**Implementation Notes**:
- Pull the config-default values directly from
  `internal/portal/config/config.go`'s `defaults()` function;
  copy-paste discipline keeps the table honest until gate-docs
  catches drift.
- Where a section depends on a feature that hasn't yet implemented
  (email, OAuth callback shape), mark with `> NOTE: provider
  configuration lands with epic-portal-foundation-auth-flows in
  v0.x.y; this section will be filled out then.`

### Unit 3: Tested-quickstart CI

**File**: `.github/workflows/quickstart.yml`
**Story**: `epic-distribution-self-host-docs-quickstart-ci`

```yaml
name: quickstart
on:
  pull_request:
    branches: [main]
  push:
    branches: [main]
jobs:
  quickstart:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: 1.22.x }
      - name: build portal
        run: go build -o portal ./cmd/portal
      - name: run portal in behind-proxy mode
        env:
          JAMSESH_BIND: 127.0.0.1:18443
          JAMSESH_TLS_MODE: behind_proxy
          JAMSESH_DB_DRIVER: sqlite
          JAMSESH_DB_DSN: ./test.db
        run: |
          ./portal &
          pid=$!
          # Wait for /healthz to come up (max 10s)
          for i in $(seq 1 20); do
            sleep 0.5
            if curl -fsS http://127.0.0.1:18443/healthz | grep -q '"status":"ok"'; then
              echo "healthcheck passed"
              kill "$pid"
              exit 0
            fi
          done
          echo "portal failed to come up" >&2
          kill "$pid" || true
          exit 1
```

**Implementation Notes**:
- This workflow runs on every PR to `main` — the docs' quickstart
  steps stay honest because CI runs them.
- The matrix grows over time: once `docker-image` lands, add a
  parallel job that pulls the image and runs the same healthcheck.
- Once auth flows land, add smoke tests for token issuance and a
  push round-trip.

**Acceptance Criteria**:
- [ ] Workflow lints clean with `actionlint`
- [ ] After `epic-portal-foundation-http-skeleton-config-tls-and-entry`
      lands, the workflow runs green on PRs
- [ ] Failing healthcheck causes the workflow to fail with a
      readable error
- [ ] `./portal` exits cleanly when the workflow sends SIGTERM (via
      `kill $pid` — the server lifecycle's graceful-shutdown path is
      exercised here)

## Implementation Order

1. **readme-and-self-host** story — `README.md` and
   `docs/SELF_HOST.md` land first; no binary dependency
2. **quickstart-ci** story — `.github/workflows/quickstart.yml`
   lands once `http-skeleton-config-tls-and-entry` has shipped a
   working `cmd/portal`

## Testing

- **Linting**: `markdownlint` on `README.md` and `docs/SELF_HOST.md`
  (not enforced in CI — reviewer call). `actionlint` on the
  workflow.
- **Executable validation**: the quickstart-ci story IS the test
  for the README's quickstart steps.

## Risks

- **Config drift**: SELF_HOST.md is keyed off
  `internal/portal/config/config.go`. If env-var names change, the
  doc rots. Mitigation: gate-docs scans for divergence at release
  time and emits items to update the doc.
- **Sequencing**: the docs reference features (OAuth, email,
  Postgres) that haven't shipped. Mitigation: explicit "lands in a
  future release" markers in each section that's currently
  speculative. The gate-docs skill removes the markers when the
  features ship.

## Implementation summary

Both child stories advanced to `stage: review`:

| Story | Status | Notes |
|---|---|---|
| `self-host-docs-readme-and-self-host` | review | README.md + docs/SELF_HOST.md (all 11 sections) + LICENSE (Apache 2.0). Configuration table values match `internal/portal/config/config.go` defaults exactly |
| `self-host-docs-quickstart-ci` | review | `.github/workflows/quickstart.yml` PR-triggered, builds portal locally, hits /healthz, exercises graceful shutdown |

### Cross-cutting deviations
- README quickstart references `ghcr.io/<owner>/jamsesh:latest` — `<owner>` left as literal placeholder pending final repo URL (triage item)
- Quickstart workflow uses `go-version-file: go.mod` rather than a hardcoded version pin (mirrors release.yml approach)
- "Common setup issues" subsection added to Troubleshooting beyond the original outline (additive improvement; clearly within doc scope)

### Verification
- README.md / SELF_HOST.md / LICENSE present and well-formed
- `actionlint .github/workflows/quickstart.yml` clean
- Local simulation of quickstart-ci: portal starts, /healthz returns `{"status":"ok"}`, SIGTERM exits cleanly
- Configuration table cross-checked against `internal/portal/config/config.go:defaults()` — all 9 entries match

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Capability complete. README + SELF_HOST.md cover the operator install/configure/operate path; the executable-spec quickstart-ci workflow keeps the docs honest. Configuration table is in lockstep with config.go defaults — gate-docs picks up drift at release time. No cross-cutting drift in foundation docs.
