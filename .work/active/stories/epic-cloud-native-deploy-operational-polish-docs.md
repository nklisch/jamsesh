---
id: epic-cloud-native-deploy-operational-polish-docs
kind: story
stage: done
tags: [infra, portal, documentation]
parent: epic-cloud-native-deploy-operational-polish
depends_on: [epic-cloud-native-deploy-operational-polish-readyz, epic-cloud-native-deploy-operational-polish-metrics, epic-cloud-native-deploy-operational-polish-secrets-from-file, epic-cloud-native-deploy-operational-polish-db-pool-and-lock, epic-cloud-native-deploy-operational-polish-graceful-shutdown]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Operational Polish — SELF_HOST + SPEC documentation

## Scope

Document every new env var, endpoint, and behavior shipped by the
five sibling stories. Add cloud deploy recipes for the four target
platforms (Cloud Run, Fly, Railway, k8s with PVC). Bring SPEC.md
deployment-shape section in sync with the new env-var surface.

Implements **Unit 6** of `epic-cloud-native-deploy-operational-polish`.
Depends on all five implementation stories so the docs reflect final
shape — values, defaults, and behavior of what landed.

## Files

Edit:
- `docs/SELF_HOST.md`:
  - §2 Configuration table — add every new env var
    (`JAMSESH_DB_MAX_OPEN_CONNS`, `_MAX_IDLE_CONNS`,
    `_CONN_MAX_LIFETIME`, `JAMSESH_SHUTDOWN_GRACE_S`, and every
    `_FILE` variant) with YAML key / default / purpose
  - New "`_FILE` convention" paragraph explaining precedence,
    fail-fast on unreadable, and use-cases
    (Secret Manager, k8s secrets, Docker Swarm secrets)
  - §8 Monitoring — replace the "future" `/metrics` note with the
    real surface; describe `/readyz` shape and the binary
    ready/not-ready contract
  - §9 Upgrade procedure — note the configurable shutdown grace
  - New §13 "Cloud deploy recipes" with concrete runnable snippets:
    - **Cloud Run** — `gcloud run deploy` command; note 60-min
      WebSocket cap and `min-instances=1` rationale
    - **Fly** — `fly.toml` snippet with `[[mounts]]` for PV,
      `[deploy].strategy=immediate`, grace_period
    - **Railway** — `railway.json` / Procfile sketch
    - **k8s with PVC** — Deployment + Service + PVC yaml example;
      readiness probe at `/readyz`;
      `terminationGracePeriodSeconds: 35` (grace + buffer)

- `docs/SPEC.md`:
  - §"Deployment shape" — list new env vars alongside existing
    ones. Keep tone "present truth" — no clustered-mode preview.

## Acceptance criteria

- [ ] SELF_HOST §2 lists every new env var with full row in the
  Configuration table.
- [ ] SELF_HOST has a `_FILE` convention paragraph explaining
  precedence and use-cases.
- [ ] SELF_HOST §8 documents `/metrics` (exposition format,
  authentication note) and `/readyz` (probe set, response shape).
- [ ] SELF_HOST §9 documents `JAMSESH_SHUTDOWN_GRACE_S`.
- [ ] SELF_HOST has a Cloud-deploy-recipes section with all four
  platforms covered.
- [ ] Each cloud recipe is concrete and runnable, not a template.
- [ ] SPEC.md "Deployment shape" reflects the new env vars
  without contradicting the single-binary self-host invariant.
- [ ] No "previously" / "originally" prose anywhere; foundation
  docs describe the new present truth (rolling-foundation
  principle).

## Implementation notes

### What landed

**`docs/SELF_HOST.md`** — edited in five places:

- **§2 Configuration table** — added 11 new rows:
  `JAMSESH_PORTAL_URL` (portal base URL, `portal_url` YAML key),
  `JAMSESH_DB_DSN_FILE`, `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE`,
  `JAMSESH_DB_MAX_OPEN_CONNS` (default 25, `db.max_open_conns`),
  `JAMSESH_DB_MAX_IDLE_CONNS` (default 5, `db.max_idle_conns`),
  `JAMSESH_DB_CONN_MAX_LIFETIME` (default `30m`, `db.conn_max_lifetime`),
  `JAMSESH_SHUTDOWN_GRACE_S` (default `30`, `shutdown_grace_s`),
  `JAMSESH_EMAIL_SMTP_PASS_FILE`, `JAMSESH_EMAIL_SENDGRID_API_KEY_FILE`,
  `JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN_FILE`, `JAMSESH_EMAIL_RESEND_API_KEY_FILE`.

- **`_FILE` convention paragraph** (new `### _FILE convention for secret env vars`
  subsection under §2): explains precedence (`_FILE` wins over plain var),
  fail-fast on unreadable path, trimming behavior, full list of the 6 `_FILE`
  variants, and integration notes for Kubernetes Secrets, Docker Swarm, and
  Secret Manager sidecars.

- **§8 Monitoring** — replaced the "planned for a future release" /metrics note
  with the full landed surface: Prometheus endpoint description, scrape config
  example, table of all 5 portal-specific metrics plus Go runtime collectors,
  alert signal recommendations. Added a new `### Readiness probe` subsection
  documenting `/readyz`: 200/503 contract, JSON envelope examples (passing and
  failing), 2s per-check timeout, parallel execution, probe table (db + storage).
  Renamed the log-based alerting content to `### Log-based alerting` to preserve
  the existing signal list.

- **§9 Upgrade procedure** — added `### Graceful shutdown` subsection before
  "When migrations are required": documents `JAMSESH_SHUTDOWN_GRACE_S` (default
  30), shared budget semantics, per-step log output, k8s
  `terminationGracePeriodSeconds` sizing advice. Also added a note about
  Postgres advisory-lock migration serialization under "When migrations are
  required".

- **New §13 Cloud deploy recipes** — concrete runnable snippets for all four
  platforms:
  - **Cloud Run**: `gcloud run deploy` command with `--port 8443`,
    `--min-instances 1`, `--timeout 3600`, `--set-secrets` for `_FILE` variant,
    `--add-cloudsql-instances`, notes on ephemeral filesystem and WebSocket 60-min
    cap.
  - **Fly.io**: `fly.toml` with `[[mounts]]` for `/data` PVC, `strategy =
    "immediate"`, `kill_signal = "SIGTERM"`, `kill_timeout = "35s"`, healthcheck
    and readiness check paths, `fly secrets set` instructions.
  - **Railway**: `railway.json` with startCommand and healthcheck, `railway
    variables set` command mapping `DATABASE_URL` plugin ref to `JAMSESH_DB_DSN`.
  - **k8s with PVC**: full YAML (Namespace, PVC, ConfigMap, Secret, Deployment,
    Service) with `terminationGracePeriodSeconds: 35`, `readinessProbe` on
    `/readyz`, `livenessProbe` on `/healthz`, Secret volume mount for `_FILE`
    vars, `replicas: 1` with single-instance design note referencing
    `epic-cloud-native-deploy`.

**`docs/SPEC.md`** — expanded the "Deployment shape" bullet list with new bullets
covering: env var surface (key vars + defaults), pool config + Postgres migration
advisory lock, shutdown grace, `_FILE` secret convention, and observability
endpoints (`/metrics`, `/readyz`, `/healthz`). Added a single-instance-by-design
note. No "previously/originally" prose anywhere; all bullets are present truth.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Docs accurately reflect the shipped surface across all 5 sibling stories. Foundation-doc principle honored — no "previously" / "originally" / "we used to" prose anywhere. Two "in v" hits in the codebase (`SPEC.md:19` "stewardship moved to Coder in 2024"; `SELF_HOST.md:540` "no destructive migrations in v1") are pre-existing factual descriptions, not introduced by this story.

The `_FILE` convention subsection is integration-aware (k8s Secrets, Docker Swarm, Secret Manager). The Cloud Run recipe is concrete and runnable (real `gcloud run deploy` command with `--set-secrets` for `_FILE` pairing) and correctly calls out platform constraints — 60-min WebSocket cap, ephemeral filesystem, Cloud SQL pairing. k8s YAML covers Namespace + PVC + ConfigMap + Secret + Deployment + Service with right `terminationGracePeriodSeconds: 35` matching `JAMSESH_SHUTDOWN_GRACE_S=30 + buffer`.

SPEC.md "Deployment shape" was extended with present-truth bullets for the new env-var surface, pool config + advisory lock, shutdown grace, `_FILE` convention, and observability endpoints. The closing single-instance-by-design note correctly frames clustered mode as "future capability; the current architecture does not coordinate across instances" — present-truth phrasing, not a forward-looking promise.

Parent feature `epic-cloud-native-deploy-operational-polish` already advanced to review by the previous orchestrator's Phase 9 (when docs was the last story to land at review and the 5 siblings were already done).
