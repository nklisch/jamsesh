---
id: epic-cloud-native-deploy-operational-polish
kind: feature
stage: drafting
tags: [infra, portal]
parent: epic-cloud-native-deploy
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Cloud-Native Deploy — Operational Polish

## Brief

The cloud-operability primitives that make the existing single-instance
portal deploy cleanly on any modern cloud platform. Ships as phase 1 of
`epic-cloud-native-deploy` and stands on its own — none of the
clustered-mode features depend on this beyond what's listed here.

Everything in this feature is also valuable for the clustered-mode
features (phase 2 of the epic), but the converse is not true: an operator
can take just this feature and get a noticeably smoother experience
deploying jamsesh on Cloud Run (min=max=1), Fly, Railway, a single VM,
or k8s with a `PersistentVolumeClaim`, without committing to the
clustered architecture.

## Scope

In:
- `/readyz` endpoint, separate from `/healthz`, that probes DB
  connectivity and storage-root accessibility. Used by k8s readiness
  probes, Cloud Run startup probes, and the future routing layer.
- `/metrics` Prometheus endpoint exposing standard process metrics plus
  portal-specific counters (HTTP request rates, push counts, auto-merger
  results, event-log throughput). Currently listed as "future" in
  `docs/SELF_HOST.md` §8.
- `JAMSESH_PUBLIC_URL` config — overrides bind-derived URL construction
  for OAuth callbacks and any other case where the portal needs its own
  externally-visible address. Required behind any LB / ingress / Cloud
  Run service.
- `JAMSESH_*_FILE` variants for every secret-bearing env var (DB DSN,
  OAuth client secret, future SMTP creds, etc.). Reads the file at the
  given path on startup; lets operators mount Secret Manager / k8s
  secrets / Docker Swarm secrets without env injection.
- Migration runner wrapped in a Postgres advisory lock so concurrent
  pod starts during a rolling deploy don't race. SQLite path unchanged
  (single-writer already serializes).
- Graceful shutdown: handle `SIGTERM`, stop accepting new connections,
  drain in-flight requests (especially long-running git pushes) within
  a configurable grace window (default 30s), then exit. Hooks into the
  existing `automerger.Worker.Stop` pattern.
- Postgres connection pool config knobs: `JAMSESH_DB_MAX_OPEN_CONNS`,
  `_MAX_IDLE_CONNS`, `_CONN_MAX_LIFETIME`. Cloud SQL / RDS small tiers
  cap total connections aggressively; the current single-DSN config
  has no way to tune.
- `docs/SELF_HOST.md` updates documenting all of the above (Cloud Run /
  Fly / Railway / k8s deploy recipes as appendices).

Out:
- Routing service, lease/fencing, object-storage sync, hydration. Those
  are the four other features in this epic.
- Tracing / OpenTelemetry support (worth doing later; not blocking).
- Log shipping integrations (operators wire their own; structured JSON
  logs already exist).

## Strategic decisions

Inherited from epic. No feature-local strategic ambiguities.

## Foundation-doc impact

- `docs/SPEC.md` — add the new env vars to the deployment-shape section
  when this feature lands at `stage: done`. Don't pre-document.
- `docs/SELF_HOST.md` — substantial updates documenting cloud deploy
  recipes (Cloud Run, Fly, Railway, k8s) and the new probes/metrics/
  secret-from-file conventions.

## Notes for design

The graceful-shutdown story interacts with the in-flight git push path
in `internal/portal/githttp/`. A push that's still streaming when
`SIGTERM` arrives needs a clean abort that doesn't half-write a packfile.
go-git's transaction semantics + the existing `post-receive` boundary
should give us a clean cut point — confirm during design.

The `/metrics` choice between Prometheus client lib vs. OpenMetrics
text-format hand-rolled is a small architectural call; expect the
Prometheus client lib (`github.com/prometheus/client_golang`) wins on
ergonomics.
