---
id: feature-docker-compose-self-host-template
kind: feature
stage: implementing
tags: [infra]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Docker Compose self-host template

## Brief

Ship a turn-key `docker-compose.yml` (and matching `.env.example`) for
self-hosting jamsesh so the path from "I want to try this on my own box" to
"running portal" is as close to one command as possible — ideally `docker
compose up -d` after editing two or three variables in `.env`.

The template bundles the portal container, a TLS-terminating reverse proxy
sidecar (auto-LE), and a volume for bare-repo storage. Database backend is
chosen via compose profile (sqlite single-file by default, Postgres opt-in).
Reasonable defaults for OAuth callback URL, ports, and storage paths so the
operator edits a tiny `.env` rather than a full compose file.

`docs/SELF_HOST.md` already covers the full operator reference; this template
is the "happy-path quickstart" companion. Once shipped, `SELF_HOST.md` gains a
quickstart section pointing at the template and `README.md` gains a one-liner.

## Strategic decisions

Locked at scope time so feature-design inherits the framing.

- **Storage default: SQLite, with a Postgres profile** — matches the
  "happy-path quickstart" framing from the idea body. SQLite is the
  zero-friction default; Postgres opt-in via `--profile postgres` for
  operators who already run a database or want HA-ready storage.
- **Profile coverage: single-node primary, clustered deferred** — the
  template ships single-node only in v1. Clustered self-hosters get k8s
  manifests; compose is for the quickstart audience.
- **TLS sidecar: Caddy** — auto-LE with zero config beyond a domain name is
  the lowest-friction fit for the "edit two or three variables" target.

## Design decisions

Resolved at design time. None are large irreversible choices.

- **Location: `deploy/compose/`** — root `compose.yaml` is already the dev
  orchestration (file-watched with `air`). A separate directory disambiguates
  the production self-host template from dev and gives it room for siblings
  (`.env.example`, `Caddyfile`, `README.md`) without polluting the repo root.
- **Image-tag pinning: pin to a specific version in `.env.example`** — sets
  `JAMSESH_VERSION=v0.1.0` rather than `latest`. Reproducible deploys are
  the default; operators can opt into `latest` by editing. Sync is enforced
  by a checklist line in `docs/RELEASING.md` step "Cutting a release".
- **OAuth/email in `.env.example`: commented-out scaffolds** — operators
  see the variable names + format but the template doesn't ship with empty
  required vars that would prevent the portal from starting clean.
  Magic-link auth via email is documented as the simpler-to-bootstrap
  alternative to OAuth (no GitHub app registration required for first run).
- **Profile structure: single `docker-compose.yml` with `profiles:`** — one
  file is more discoverable than `docker-compose.postgres.yml`. The
  `postgres` service has `profiles: [postgres]`; the portal service uses
  env-file values that operators flip when activating the profile.
- **Caddy auto-LE target: real domain only** — the Caddyfile assumes a
  publicly-reachable `${JAMSESH_DOMAIN}` for cert provisioning. For
  purely-local trials, the README's `docker run` snippet (already there) is
  the right path. We don't ship a `tls internal` Caddyfile variant in v1.
- **Healthchecks: Docker-native, hitting `/healthz`** — portal healthcheck
  hits `http://localhost:8443/healthz`; Caddy healthcheck hits its own
  admin endpoint; Postgres uses `pg_isready`. Caddy `depends_on` portal
  with `condition: service_healthy`. When the postgres profile is active,
  portal `depends_on` postgres with `condition: service_healthy`.
- **CI smoke depth: parse-only in v1** — `docker compose config` against
  both the default and `--profile postgres` shapes runs in CI to catch
  typos and structural errors. End-to-end "compose up + curl healthz"
  smoke is deferred to a backlog item that lands once a published image
  tag is available (current commits don't have pull-able `ghcr.io`
  images during PR CI). The parse-only check is enough to keep the
  template from rotting structurally.

## Mockups

N/A — infrastructure/config only, no UI surface.

## Architectural choice

The template is a four-file bundle under `deploy/compose/`:

```
deploy/compose/
├── docker-compose.yml    # services, profiles, volumes, healthchecks
├── .env.example          # operator-facing config; copy to .env and edit
├── Caddyfile             # reverse proxy config; references ${JAMSESH_DOMAIN}
└── README.md             # 4-step quickstart, profile switching, troubleshooting
```

Operator flow:

```bash
git clone https://github.com/<owner>/jamsesh
cd jamsesh/deploy/compose
cp .env.example .env
$EDITOR .env          # set JAMSESH_DOMAIN + OAuth or email creds
docker compose up -d
```

The portal image is `ghcr.io/<owner>/jamsesh:${JAMSESH_VERSION}` with
`JAMSESH_VERSION` pinned in `.env.example`. The Caddy sidecar terminates TLS
on `:443` (and `:80` for ACME challenges), reverse-proxies to `portal:8443`
on the internal network, and stores certs in a named volume so renewals
survive container restarts. The portal binds `:8443` internally only — it
is **not** published to the host — so all external traffic goes through Caddy.

Profiles:

- **default** (no `--profile` flag): `portal` + `caddy` services, SQLite at
  `/data/jamsesh.db` inside the `jamsesh_data` named volume.
- **postgres** (`docker compose --profile postgres up -d`): adds `postgres`
  service. Portal's `JAMSESH_DB_DRIVER` and `JAMSESH_DB_DSN` env values are
  overridden via `.env` to point at the postgres service. Postgres data
  lands in a named `postgres_data` volume.

## Implementation Units

### Unit 1: Compose template files
**Story**: `feature-docker-compose-self-host-template-template-files`
**Files**:
- `deploy/compose/docker-compose.yml`
- `deploy/compose/.env.example`
- `deploy/compose/Caddyfile`
- `deploy/compose/README.md`

**`docker-compose.yml` shape:**

```yaml
# Self-host quickstart template — see ./README.md and ../../docs/SELF_HOST.md.
# Edit ./.env after copying from ./.env.example; do not edit this file unless
# you need to deviate from the recommended single-node shape.

services:
  portal:
    image: ghcr.io/${JAMSESH_OWNER:-<owner>}/jamsesh:${JAMSESH_VERSION:-latest}
    restart: unless-stopped
    env_file: .env
    environment:
      JAMSESH_BIND: ":8443"
      JAMSESH_TLS_MODE: behind_proxy
      JAMSESH_STORAGE: /data/storage
      # SQLite default; overridden via .env when the postgres profile is active.
      JAMSESH_DB_DRIVER: ${JAMSESH_DB_DRIVER:-sqlite}
      JAMSESH_DB_DSN: ${JAMSESH_DB_DSN:-/data/jamsesh.db}
      JAMSESH_PORTAL_URL: https://${JAMSESH_DOMAIN}
      JAMSESH_LOG_FORMAT: json
    volumes:
      - jamsesh_data:/data
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8443/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
    # Internal only — Caddy fronts.
    expose: ["8443"]

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    environment:
      JAMSESH_DOMAIN: ${JAMSESH_DOMAIN}
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      portal:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:2019/config/"]
      interval: 30s
      timeout: 5s
      retries: 3

  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    profiles: [postgres]
    environment:
      POSTGRES_USER: ${POSTGRES_USER:-jamsesh}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB:-jamsesh}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-jamsesh}"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s

volumes:
  jamsesh_data:
  caddy_data:
  caddy_config:
  postgres_data:
```

Note the portal service does NOT have `depends_on: postgres` because
postgres is profile-gated — compose ignores `depends_on` references to
inactive-profile services. Operators activating the postgres profile add
the dependency via override or accept that the portal will retry the DB
connection until postgres is healthy (Postgres comes up in ~5s typically;
the portal's startup migration loop tolerates this). This keeps the
default profile simple. Document the behavior in `README.md`.

**`.env.example` shape:**

```bash
# === REQUIRED — edit these before first run ===

# Your public-facing hostname. Caddy uses this for Let's Encrypt cert
# provisioning. Must resolve to this host on the public internet for ACME
# HTTP-01 challenges to succeed.
JAMSESH_DOMAIN=jamsesh.example.com

# === COMMONLY EDITED ===

# The portal image version. Pinning a specific tag is recommended over
# `latest` for reproducible deploys. Bump this when upgrading.
JAMSESH_VERSION=v0.1.0

# GitHub OAuth (preferred login method). Register an OAuth app at
# https://github.com/settings/applications/new with callback URL:
#   https://<JAMSESH_DOMAIN>/api/auth/oauth/callback
# Then uncomment and fill these:
# JAMSESH_OAUTH_GITHUB_CLIENT_ID=...
# JAMSESH_OAUTH_GITHUB_CLIENT_SECRET=...

# Magic-link email auth (simpler alternative to OAuth; requires an SMTP
# relay or transactional email provider). Required if OAuth is not
# configured. See docs/SELF_HOST.md §6 for provider-specific vars.
# JAMSESH_EMAIL_PROVIDER=smtp
# JAMSESH_EMAIL_FROM=jamsesh <noreply@example.com>
# JAMSESH_EMAIL_SMTP_HOST=smtp.example.com
# JAMSESH_EMAIL_SMTP_USER=...
# JAMSESH_EMAIL_SMTP_PASS=...

# === POSTGRES PROFILE (optional) ===
#
# To use Postgres instead of SQLite:
#   1. Uncomment the three POSTGRES_* lines below.
#   2. Uncomment JAMSESH_DB_DRIVER and JAMSESH_DB_DSN.
#   3. Bring the stack up with: docker compose --profile postgres up -d
#
# POSTGRES_USER=jamsesh
# POSTGRES_PASSWORD=changeme
# POSTGRES_DB=jamsesh
# JAMSESH_DB_DRIVER=postgres
# JAMSESH_DB_DSN=host=postgres user=jamsesh password=changeme dbname=jamsesh sslmode=disable

# === ADVANCED — change only if you know why ===

# Defaults to the upstream owner. Override to use a fork's image registry.
# JAMSESH_OWNER=<owner>
```

**`Caddyfile` shape:**

```
# Auto-LE for the configured domain. Reverse-proxies to portal:8443 inside
# the compose network. WebSocket upgrades pass through Caddy's
# reverse_proxy directive natively.
{$JAMSESH_DOMAIN} {
    reverse_proxy portal:8443 {
        header_up Host {host}
        header_up X-Forwarded-Proto {scheme}
    }

    log {
        output stdout
        format json
        level INFO
    }
}
```

**`README.md` in `deploy/compose/`** — concise (~80 lines):

- **What this is**: 1-paragraph framing; pointers to root README and SELF_HOST.md.
- **Prerequisites**: Docker 24+, Docker Compose v2, a domain pointing at this host (or skip to the bare `docker run` quickstart in the root README).
- **Quickstart (4 steps)**: clone → cd → cp .env.example .env → edit → up.
- **Choosing SQLite vs Postgres**: when to switch profiles.
- **Image version pinning**: how to bump `JAMSESH_VERSION`.
- **Troubleshooting**:
  - Cert provisioning failing → check DNS, check ports 80/443 reachable.
  - "no auth configured" on login → OAuth or email must be set up.
  - Volume permissions → fresh named volumes work out of the box; if
    you mounted a host path, ensure the portal user (`nobody`, UID 65534)
    can write to it.
  - Postgres profile not picking up → `docker compose --profile postgres up -d`,
    not plain `up -d`.
- **Upgrading**: bump `JAMSESH_VERSION`, `docker compose pull`,
  `docker compose up -d`. Migrations run automatically on portal start.

**Implementation Notes**:
- The portal image is built `USER nobody`, so the `/data` volume must be
  writable by UID 65534. Docker named volumes inherit ownership from
  first write — works out of the box.
- The Caddyfile uses Caddy's env-var interpolation syntax (`{$VAR}`) not
  compose's. Compose passes `JAMSESH_DOMAIN` into the caddy container's
  env; Caddy resolves it at startup.
- The `expose: ["8443"]` line on portal is documentation — compose
  publishes nothing by default; it's there to make the contract explicit
  for readers.
- Caddy's admin API at `localhost:2019` is used for the healthcheck (not
  exposed externally — Caddy binds admin to loopback only by default).

**Acceptance Criteria**:
- [ ] `docker compose -f deploy/compose/docker-compose.yml config` parses
      without errors.
- [ ] `docker compose -f deploy/compose/docker-compose.yml --profile postgres config`
      parses without errors and includes the `postgres` service.
- [ ] `.env.example` has at most 1 required uncommented var
      (`JAMSESH_DOMAIN`) and the rest are commented-out scaffolds.
- [ ] All four files are present at `deploy/compose/`.
- [ ] The portal service does not publish ports to the host (Caddy fronts).
- [ ] Named volumes for portal data, caddy data/config, postgres data are
      declared.
- [ ] Healthchecks declared on portal, caddy, postgres.
- [ ] Caddy `depends_on: portal { condition: service_healthy }` is set.
- [ ] No reference to `<owner>` placeholder remains where a real org name
      is expected at runtime — `<owner>` only appears in commented
      operator-facing examples in `.env.example` and README. The compose
      file uses `${JAMSESH_OWNER:-<owner>}` so operators can override
      cleanly.

---

### Unit 2: Documentation updates
**Story**: `feature-docker-compose-self-host-template-docs`
**Depends on**: `feature-docker-compose-self-host-template-template-files`
**Files**:
- `docs/SELF_HOST.md` (edit: new §1.0 "Quickstart with Docker Compose" at top of §1)
- `README.md` (edit: replace "Operator quickstart" body)
- `docs/RELEASING.md` (edit: add a step in "Cutting a release" for the version pin)

**`docs/SELF_HOST.md` delta:**

Add a new §1.0 subsection at the top of §1 ("Install"), before the existing
"Docker (recommended)" subsection:

```markdown
### 1.0 Quickstart with Docker Compose

For the fastest path from clone to running portal, use the bundled compose
template at `deploy/compose/`:

\`\`\`bash
git clone https://github.com/<owner>/jamsesh
cd jamsesh/deploy/compose
cp .env.example .env
$EDITOR .env       # set JAMSESH_DOMAIN + OAuth or email creds
docker compose up -d
\`\`\`

This brings up the portal behind a Caddy reverse proxy with automatic
HTTPS via Let's Encrypt. See [`deploy/compose/README.md`](../deploy/compose/README.md)
for the full template reference (profiles, volumes, upgrading).

The rest of this document is the deep operator reference — TLS modes,
OAuth callbacks, database options, monitoring, and backup. Read on when
the quickstart template doesn't fit your shape.
```

Existing §1 ("Docker (recommended)" / "Binary" / "systemd unit") stays
unchanged below this new §1.0.

**`README.md` delta** — replace the "Operator quickstart" section with:

```markdown
## Operator quickstart

The fastest way to run jamsesh on your own host:

\`\`\`bash
git clone https://github.com/<owner>/jamsesh
cd jamsesh/deploy/compose
cp .env.example .env
$EDITOR .env       # set JAMSESH_DOMAIN + OAuth or email creds
docker compose up -d
\`\`\`

This brings up the portal behind a Caddy reverse proxy with automatic
HTTPS. See [`deploy/compose/README.md`](deploy/compose/README.md) for the
template reference and [docs/SELF_HOST.md](docs/SELF_HOST.md) for TLS,
OAuth, database options, and production deployment details.

To kick the tires locally without a domain or TLS:

\`\`\`bash
docker run --rm -p 8443:8443 \
  -e JAMSESH_TLS_MODE=behind_proxy \
  -e JAMSESH_BIND=:8443 \
  -v $(pwd)/data:/data \
  ghcr.io/<owner>/jamsesh:latest

curl http://localhost:8443/healthz
# → {"status":"ok"}
\`\`\`
```

**`docs/RELEASING.md` delta** — add a numbered step to "Cutting a release",
positioned between current step 1 (drain the queue) and step 2 (confirm
CHANGELOG):

```markdown
2. **Bump the compose template's `JAMSESH_VERSION` pin.** The self-host
   quickstart template at `deploy/compose/.env.example` pins
   `JAMSESH_VERSION` to a specific tag for reproducible operator deploys.
   Bump it to the version you're about to release, then commit:

   \`\`\`bash
   sed -i 's/^JAMSESH_VERSION=.*/JAMSESH_VERSION=v0.X.0/' deploy/compose/.env.example
   git add deploy/compose/.env.example
   git commit -m "release-prep: bump compose template to v0.X.0"
   \`\`\`

   Skipping this step means freshly-cloned operator setups will deploy
   the previous release until they edit their `.env` manually.
```

Renumber subsequent steps (current 2 → 3, etc.).

**Acceptance Criteria**:
- [ ] `docs/SELF_HOST.md` has a §1.0 quickstart pointing at `deploy/compose/`.
- [ ] `README.md` "Operator quickstart" leads with the compose template path.
- [ ] `docs/RELEASING.md` has a "Bump the compose template" step in the
      cutting-a-release sequence with a one-line edit command.
- [ ] All cross-references resolve (no broken relative paths).

---

### Unit 3: CI parse-validation
**Story**: `feature-docker-compose-self-host-template-ci-smoke`
**Depends on**: `feature-docker-compose-self-host-template-template-files`
**Files**:
- `.github/workflows/quickstart.yml` (edit: add a new job)

Add a `compose-template` job to `quickstart.yml`:

```yaml
  compose-template:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: validate default compose shape
        working-directory: deploy/compose
        run: |
          cp .env.example .env
          # Caddyfile reads $JAMSESH_DOMAIN from compose env; .env.example's
          # placeholder value is enough to satisfy parsing.
          docker compose config > /dev/null

      - name: validate postgres profile shape
        working-directory: deploy/compose
        run: |
          docker compose --profile postgres config > /dev/null

      - name: assert no unresolved env vars in default shape
        working-directory: deploy/compose
        run: |
          # `docker compose config` emits warnings for unresolved vars to
          # stderr but exits 0. Capture stderr and fail if any
          # `WARN[…] The "..." variable is not set.` lines appear.
          stderr=$(docker compose config 2>&1 >/dev/null) || true
          if echo "$stderr" | grep -E 'variable is not set'; then
            echo "Unresolved env vars in default compose shape — .env.example must satisfy every interpolation."
            exit 1
          fi
```

**Implementation Notes**:
- `docker compose config` is the canonical compose-file linter. It parses,
  resolves env interpolation, and emits the merged YAML. Exit code is
  nonzero on parse errors. Unresolved env vars go to stderr but don't
  fail the command — we grep for them explicitly.
- The job is fast (no image pulls, no container start). Adds <10s to CI.
- End-to-end "up + curl healthz" smoke is deferred to a follow-up
  backlog item once a pull-able image tag exists for the commit under
  test. The parse-only check is enough to catch template rot from
  refactors elsewhere in the repo.

**Acceptance Criteria**:
- [ ] `quickstart.yml` has a `compose-template` job.
- [ ] Job runs `docker compose config` against the default shape.
- [ ] Job runs `docker compose --profile postgres config` against the
      postgres shape.
- [ ] Job fails CI if `.env.example` is missing a variable that the
      compose file interpolates.
- [ ] Job runs on every PR and on push to main, parallel to existing
      `quickstart` job.

---

## Implementation Order

1. **template-files** — the four files in `deploy/compose/`. Foundational;
   nothing else can land without these on disk.
2. **docs** — SELF_HOST.md + README.md + RELEASING.md updates. Depends on
   template-files (cross-references the new path).
3. **ci-smoke** — `quickstart.yml` parse-validation job. Depends on
   template-files (validates them).

Stories 2 and 3 are independent of each other once story 1 is done, so
`/agile-workflow:implement-orchestrator` can fan them out in parallel
after story 1 lands.

## Testing

### Manual smoke (operator-side, post-merge)
Once a release with this feature ships:
- Fresh clone, `cp .env.example .env`, set `JAMSESH_DOMAIN` to a test
  subdomain pointing at a test host.
- `docker compose up -d`.
- Visit `https://<test-domain>/healthz` → `{"status":"ok"}`.
- Configure OAuth or email in `.env`, restart, complete login flow.
- Verify cert renewal works after Caddy's first renewal cycle (manual
  spot check is fine — Caddy renews 30 days before expiry).

### CI-side
- `docker compose config` (default + postgres profile) covered by
  the `compose-template` job in `quickstart.yml`.
- No end-to-end "compose up + login" CI smoke in v1 — deferred.

## Risks

- **Image-tag drift between releases.** Mitigated by the RELEASING.md
  checklist step (story 2). Worst case: operators on a fresh clone get
  the previous release until they bump `.env`. Low-severity; documented.
- **Caddy auto-LE rate limits during operator misconfig.** Let's Encrypt
  has a 5-failure-per-host-per-hour rate limit. An operator with bad DNS
  can blow through it. Mitigation: `README.md` troubleshooting section
  links to Caddy's LE staging endpoint for testing. Out of scope for v1.
- **Operators without GitHub OAuth or SMTP.** The portal starts cleanly
  but no one can log in. `.env.example` and `README.md` both call this
  out; documented behavior, not a defect.
- **Postgres profile race on first start.** Portal's startup migration
  runs against postgres before postgres finishes initdb. The portal's
  retry loop tolerates this in practice (Postgres comes up in <5s).
  If this becomes a real problem, add `depends_on: postgres` inside an
  override file scoped to the postgres profile in a follow-up.
- **Caddy admin endpoint healthcheck.** Caddy binds `:2019` to loopback
  by default; `wget http://localhost:2019/config/` from within the
  container works. If a future Caddy version changes this default, the
  healthcheck breaks. Pinning `caddy:2-alpine` mitigates near-term.

<!-- Implementation Notes accumulate as each story lands. -->
