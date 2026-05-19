---
id: feature-docker-compose-self-host-template-template-files
kind: story
stage: done
tags: [infra]
parent: feature-docker-compose-self-host-template
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Compose template files

## Scope

Create the four-file self-host quickstart template under `deploy/compose/`:

- `deploy/compose/docker-compose.yml`
- `deploy/compose/.env.example`
- `deploy/compose/Caddyfile`
- `deploy/compose/README.md`

This is the foundational story — the docs and CI stories cross-reference
these files.

## Implementation

Follow the shapes specified in the parent feature's "Unit 1: Compose template
files" section. Key contract points:

- Portal service uses `ghcr.io/${JAMSESH_OWNER:-<owner>}/jamsesh:${JAMSESH_VERSION:-latest}`.
- Portal binds `:8443` internally only; Caddy fronts on `:80` / `:443`.
- SQLite default at `/data/jamsesh.db` inside the `jamsesh_data` named volume.
- Postgres profile (`profiles: [postgres]`) on the postgres service only —
  portal's DB driver/DSN flip via operator-edited `.env` values.
- Healthchecks on all three services. Caddy `depends_on` portal with
  `condition: service_healthy`.
- `.env.example` has exactly one uncommented var (`JAMSESH_DOMAIN`) and
  `JAMSESH_VERSION=v0.1.0`; all other vars are commented-out scaffolds with
  inline guidance.
- `Caddyfile` uses Caddy's `{$JAMSESH_DOMAIN}` env interpolation; relies on
  Caddy's automatic HTTPS for cert provisioning.
- `README.md` at `deploy/compose/README.md` covers prerequisites, 4-step
  quickstart, profile switching, version pinning, troubleshooting, and
  upgrading. Concise — aim for ~80 lines.

## Acceptance Criteria

- [ ] All four files exist at the specified paths.
- [ ] `docker compose -f deploy/compose/docker-compose.yml config` parses
      cleanly with `.env` copied from `.env.example` (default profile).
- [ ] `docker compose -f deploy/compose/docker-compose.yml --profile postgres config`
      parses cleanly and includes the `postgres` service in the merged
      output.
- [ ] `.env.example` has exactly one uncommented required-edit var
      (`JAMSESH_DOMAIN`) plus the version pin (`JAMSESH_VERSION`); every
      other var is commented out.
- [ ] Portal service does not publish ports to the host; only Caddy
      publishes `:80` and `:443`.
- [ ] Named volumes `jamsesh_data`, `caddy_data`, `caddy_config`,
      `postgres_data` are declared.
- [ ] Caddy `depends_on: portal { condition: service_healthy }`.
- [ ] `README.md` exists, references the parent docs (`docs/SELF_HOST.md`,
      root `README.md`) via relative paths.

## Notes

- The `<owner>` placeholder appears in the compose file's image string as
  `${JAMSESH_OWNER:-<owner>}` and in commented operator-facing examples
  in `.env.example` and `README.md`. This is intentional — operators
  must either override `JAMSESH_OWNER` in `.env` or fork the template
  and pin their own. Don't hard-code an org name in the published
  template.
- The compose file targets Compose v2 spec — no `version:` top-level key
  (deprecated in v2).

## Implementation Notes

### Files created

- `deploy/compose/docker-compose.yml`
- `deploy/compose/.env.example`
- `deploy/compose/Caddyfile`
- `deploy/compose/README.md`

### Deviation from spec: `POSTGRES_PASSWORD` default

The spec shows `POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}` (no default). In
practice, `docker compose config` emits a "variable is not set. Defaulting to
a blank string." warning for this var even when the postgres profile is
inactive, because the postgres service definition is still parsed. The CI
smoke check in Unit 3 greps for "variable is not set" and would fail on this.

Resolution: changed to `${POSTGRES_PASSWORD:-}` (explicit empty default) with
an inline comment: `# No default — operators must set POSTGRES_PASSWORD in
.env.` This suppresses the parse-time warning without providing a real default;
Postgres will still refuse to start cleanly with an empty password, which is
the correct operator-facing behaviour.

### Verification output

```
postgres service present in profile
postgres service correctly absent from default profile
Verification passed.
```

All acceptance criteria confirmed:
- Four files exist at `deploy/compose/`.
- `docker compose config` (default) parses cleanly with no unresolved vars.
- `docker compose --profile postgres config` parses cleanly; postgres service
  present in merged output.
- `.env.example` has exactly one uncommented required-edit var (`JAMSESH_DOMAIN`)
  plus the version pin (`JAMSESH_VERSION=v0.1.0`); every other var is
  commented.
- Portal service uses `expose: ["8443"]` only; Caddy publishes `:80`/`:443`.
- Named volumes: `jamsesh_data`, `caddy_data`, `caddy_config`, `postgres_data`.
- `caddy.depends_on.portal.condition: service_healthy` confirmed.
- `README.md` references `../../docs/SELF_HOST.md` and `../../README.md` via
  relative paths.
