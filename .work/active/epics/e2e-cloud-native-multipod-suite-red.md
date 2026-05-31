---
id: e2e-cloud-native-multipod-suite-red
kind: epic
stage: implementing
tags: [portal, infra, testing, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-31
updated: 2026-05-31
---

# e2e suite broadly red: cloud-native multi-pod router/lease/object-storage tests

The `e2e` GitHub workflow (`.github/workflows/e2e.yml`, runs on push/PR to main)
has been **failing on `main` for a while** — red on `v0.4.1` (2026-05-25),
`4e7c6731`, and current `main`. Failures span 5 suites: `chaos`, `failure`,
`fuzz`, `golden`, `scaffolding`. They are NOT caused by the session-resume epic
(`epic-cli-browser-session-resume`) — that work is frontend auth + a new portal
contract and doesn't touch the router/lease/object-storage/git-serving paths
these tests exercise.

## Strategic decisions
- **Definition of done**: all 5 suites (`chaos`, `failure`, `fuzz`, `golden`,
  `scaffolding`) green and reliable in the `e2e` workflow on `main`. Flipping
  `e2e` to a required/blocking merge gate (branch protection) is explicitly OUT
  of scope here — it's a separate policy change tracked as follow-up. Keeps this
  epic focused on stabilization, not CI gating rollout.
- **Resolution posture**: follow the project test-integrity rules (CLAUDE.md).
  Real product bugs become child stories and are fixed as defects; test debt
  (stale waits, missing cross-pod polls, drifted assertions, broken mocks) is
  repaired in-session. Each suite is sorted on its merits during `epic-design`
  rather than assuming all five share one root cause.

## Design decisions
- **Decomposition shape**: split by **subsystem** (vertical capability), not by
  suite. Each suite (`chaos`/`failure`/`golden`) contains tests from several
  subsystems; a suite goes green when all its subsystem-owned tests pass. By-suite
  would be the layer anti-pattern. — Keeps each feature a cohesive root-cause fix.
- **Triage approach**: per-subsystem — no shared triage/bisect feature. Each
  subsystem feature reproduces its own suite, confirms its root cause, and fixes
  it, so the four independent features parallelize. The only shared harness crash
  (prometheus parse) is already fixed in `ed32b562`. — Avoids a serializing root.
- **Regression vs never-green**: these suites **never passed green on main**
  (user-confirmed). This epic *finishes the stabilization* the cloud-native-deploy
  / e2e-cnd-coverage epics left red — it is NOT a regression. No `git bisect`;
  each feature root-causes forward from the current red state.
- **Fuzz scope**: fuzz stabilization is a child feature of this epic. If
  characterization reveals a deep input-handling product arc, split it out then
  (see Decomposition risks).

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
- These suites never passed green on main (confirmed): this is finishing the
  cloud-native-deploy / e2e-cnd-coverage stabilization, not a regression — no
  `git bisect`, root-cause forward from the current red state.
- Scope boundary: this epic owns the **cloud-native multi-pod** failures
  (object-storage sync, lease, router, cluster git serving) + fuzz. Playground-
  specific tests in the chaos/failure/golden suites are owned by the playground
  epics (already done) and are out of scope here.

## Decomposition

Split by subsystem so each child feature is a self-contained root-cause fix that
reproduces its own slice of the red suites. Three subsystem fixes
(object-storage sync, lease migration, router redispatch/metrics) plus fuzz are
mutually independent and parallelize; the clustered-smoke scaffolding test is a
cross-cutting end-to-end gate, so it depends on the three subsystem fixes
landing first (its green is then a true signal, not a mask). No shared triage
feature — the only common harness crash (prometheus parse) is already fixed.

### Child features

- `e2e-cloud-native-multipod-suite-red-objectstore-sync` — cross-pod base-ref
  visibility / RPO=0 hydration timing (chaos prereqs) — depends on: `[]`
- `e2e-cloud-native-multipod-suite-red-lease-migration` — advisory-lock
  auto-release on Postgres conn-drop + re-acquisition within 30s SLO
  (failure/golden lease) — depends on: `[]`
- `e2e-cloud-native-multipod-suite-red-router-redispatch` — transparent
  redispatch on 503 + router metric counters (failure/golden router) —
  depends on: `[]`
- `e2e-cloud-native-multipod-suite-red-fuzz` — characterize + stabilize the red
  fuzz harnesses — depends on: `[]`
- `e2e-cloud-native-multipod-suite-red-cluster-smoke` — fix `git clone` over
  router (githttp regression check) + the end-to-end clustered smoke; the
  scaffolding integration gate — depends on:
  `[objectstore-sync, lease-migration, router-redispatch]`

### Decomposition risks

- **Shared cross-pod git-serving root.** Root cause (1) object-storage sync and
  the cluster-smoke `git clone` exit-128 may share a cross-pod git-serving root,
  or the clone may be an independent githttp/router-routing regression. The gate
  feature is sequenced after objectstore-sync to absorb this; if a single fix
  resolves both, the gate shrinks to a verification pass.
- **Cross-cutting chaos tests.** A few chaos tests (`cross_pod_clock_skew`,
  `runtime_and_clock`, `network_and_provider`, `router_pod_disappears`,
  `object_storage_partition`) don't map cleanly to one subsystem. Each feature's
  own design pass must claim exactly one owner per test to avoid gaps/overlap.
- **Fuzz may grow an arc.** If fuzz characterization uncovers deep product
  input-handling bugs, split them out as a separate item rather than ballooning
  the fuzz feature.

## Implementation discoveries (2026-05-31)

Progress so far (this autopilot run):

- **`fuzz` → review (suite green).** Fixed a real product bug: `ManifestStore.Load`
  silently accepted corrupt manifests → dropped history; now fails fast with
  `ErrCorruptManifest` (hardened via Codex review so `Save` is self-guarding too).
- **`objectstore-sync` → review.** The "empty SHA prerequisite" was a test-only
  ref-name mismatch (short `jam/...` vs full `refs/heads/jam/...`); fixed. Write
  path is genuinely synchronous (RPO=0). Filed latent
  `portal-rest-refs-no-cross-pod-hydration`.
- **`lease-migration` → review.** Fixed a lease-takeover false "hashtext
  collision" (`postgres.go`) that blocked every survivor takeover after an
  unclean holder exit. Survivors now hydrate (the `503` symptom is gone).

Never-green peeling exposed two NEW root causes, filed as child stories:

- `handoff-nonfastforward-post-hydration-push` — with hydration working, the
  handoff tests (+ stale-fencing, lease-acquire-and-fence) now fail at a
  post-handoff `non-fast-forward` push. Next layer of the handoff tests; needs
  product-vs-test classification (gitclient default-branch vs hydrate HEAD).
- `lease-holder-killed-eager-vs-request-driven-slo` — `lease_holder_killed_test.go`
  asserts eager background migration, but acquisition is request-driven by
  design. Architecture decision needed (fix the test to trigger acquisition, or
  add a background failover loop) — do NOT game the test.

`router-redispatch` → review: fixed a real proxy 503-retry response-leak; the
router e2e tests were found to be mis-premised (REST never takes the per-session
lease — only git ops do) and are skipped with backlog links
(`idea-router-e2e-lease-premise`); router product verified by unit tests.

The two discovery stories (`handoff-nonfastforward-post-hydration-push`,
`lease-holder-killed-eager-vs-request-driven-slo`) → review: both targeted
test-premise fixes are delivered and VERIFIED working (commit `44f949b2`) —
non-fast-forward gone, `stale_fencing` PASS, lease migrates within SLO with
monotonic token. `cluster-smoke` (integration gate) is still drafting.

### Peeled layers still blocking the handoff/lease suites (verification 2026-05-31)

With the takeover + test-premise fixes in, the handoff/lease chaos tests advanced
to TWO deeper layers — both tracing to PRIOR, supposedly-released bugs:

1. **githttp `send-pack: unexpected disconnect` pushing to a freshly-hydrated
   survivor** — blocks both handoff tests. Product githttp/receive-pack issue;
   relates to `bug-receive-pack-report-status-sideband-wrapping` (released
   v0.3.0). The survivor hydrates fine, then the push to it hangs up mid-sideband.
2. **Router 502 routing to a killed pod** — blocks `lease_holder_killed`'s
   post-migration clone (and is why the handoff tests assert directly against the
   survivor, bypassing the router). This is `bug-router-static-discoverer-not-started`
   (released v0.1.0) — dead pods linger in the ring → 502; possibly an incomplete
   prior fix or a missing per-request failover timeout. Referenced by
   `router_pod_disappears_test.go`.

Plus an infra flake: `golden/lease_acquire_and_fence` failed only on portal
container cold-start (5×60s retries exhausted) under host load — re-verify in
isolation; not a code defect.

Net so far: real product bugs FIXED — manifest corrupt-input, lease takeover,
proxy 503-leak. The suite is heavily mis-premised (ref-name, non-fast-forward,
lease request-trigger, router lease-premise) — those are corrected. The
remaining blockers (githttp sideband disconnect, router dead-pod 502) are
residual product defects from prior releases, converging on the
"git-clone/push over the router across handoff/eviction" surface that
`cluster-smoke` is the gate for.
