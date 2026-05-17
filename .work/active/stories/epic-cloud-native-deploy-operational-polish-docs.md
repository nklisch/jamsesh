---
id: epic-cloud-native-deploy-operational-polish-docs
kind: story
stage: implementing
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
