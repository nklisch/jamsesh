---
id: e2e-cloud-native-multipod-suite-red
kind: epic
stage: backlog
tags: [portal, infra, testing, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-31
updated: 2026-05-30
---

# e2e suite broadly red: cloud-native multi-pod router/lease/object-storage tests

The `e2e` GitHub workflow (`.github/workflows/e2e.yml`, runs on push/PR to main)
has been **failing on `main` for a while** — red on `v0.4.1` (2026-05-25),
`4e7c6731`, and current `main`. Failures span 5 suites: `chaos`, `failure`,
`fuzz`, `golden`, `scaffolding`. They are NOT caused by the session-resume epic
(`epic-cli-browser-session-resume`) — that work is frontend auth + a new portal
contract and doesn't touch the router/lease/object-storage/git-serving paths
these tests exercise.

## Fixed already (commit ed32b562)

- **prometheus/common v0.66 unset-scheme panic.** `fetchRouterMetrics` in
  `tests/e2e/golden/router_hint_cache_test.go` used the zero-value
  `expfmt.TextParser`, whose validation scheme is `Unset` and panics in
  `model.ValidationScheme.IsValidMetricName` ("Invalid name validation scheme
  requested: unset"). The panic crashed the entire `golden` test binary.
  Switched to `expfmt.NewTextParser(model.LegacyValidation)` (mirroring the
  already-migrated `metrics_endpoint_test.go`). Golden no longer panics; it now
  runs and surfaces the *real* remaining failures below.

## Remaining root causes (need a dedicated multi-pod debugging pass)

1. **Cross-pod base-ref visibility** [chaos]:
   `handoff_under_object_storage_chaos_test.go:153` and
   `handoff_under_pod_kill_test.go:129` fail at the PREREQUISITE — "pod N
   returned empty SHA for ref jam/<sid>/<acct>/main before chaos/kill". The test
   pushes a base ref via one pod then reads its SHA from another pod and gets
   empty — i.e. the object-storage repo sync / cache hydration between pods isn't
   complete (or broken) before chaos is applied. Likely the object-storage sync
   provider (epic-cloud-native-deploy-object-storage-sync) or repo-cache
   hydration timing.

2. **Lease migration SLO** [failure, golden]:
   `lease_holder_killed_test.go:148` — "lease did not migrate from pod X within
   30s SLO after SIGKILL; check advisory-lock auto-release on Postgres connection
   drop and re-acquisition path in PostgresManager.Acquire".
   `lifecycle_evict_on_lease_release_test.go` related.

3. **Router redispatch / metrics** [failure, golden]:
   `router_lease_unavailable_test.go` (transparent_redispatch_on_503, bounded
   retry → 503), `router_consistent_hash_test.go` (metric counter "-1 not >= 0"),
   `router_hint_cache_test.go` decisions counter.

4. **git clone over router smart-HTTP** [scaffolding]:
   `cluster_smoke_test.go:143` TestClusteredSmoke — `git clone <router>/git/...`
   exits 128. Possibly shares a root cause with (1) (cross-pod git serving) or is
   a router-routing/auth issue. NOTE: githttp was heavily modified recently
   (bug-squash: receive-pack-truncated, git-auth-client-abort, looksLikeReportStatus)
   — check for a regression there.

5. **fuzz** suite also red — characterize after the above.

## How to work it

- Reproduce locally (Docker available): `make test-portal-image &&
  make test-router-image && (cd tests/e2e && go test ./scaffolding/... )` then
  widen to chaos/failure. The harness uses testcontainers (minio, toxiproxy,
  postgres, multiple portal pods, router, mailhog). NOTE: a full `/tmp` tmpfs
  breaks builds — set `GOTMPDIR`/`TMPDIR` off it.
- Bisect: did these suites EVER pass green on main? If the cloud-native-deploy
  epic landed them red, this is finishing that epic's stabilization, not a
  regression. If they regressed, `git bisect` the router/lease/object-storage/
  githttp commits.
- Likely splits into child features: object-storage cross-pod sync, lease
  migration on Postgres conn-drop, router redispatch/metrics, cluster git clone.

Promote via `/agile-workflow:scope` and decompose with `epic-design`.
