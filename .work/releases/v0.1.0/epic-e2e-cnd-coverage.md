---
id: epic-e2e-cnd-coverage
kind: epic
stage: done
tags: [e2e-test, testing, infra, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Coverage — Cloud-Native Deploy

## Brief

The `epic-cloud-native-deploy` epic shipped 5 features (operational-polish,
routing-layer, lease-fencing, object-storage-sync, hydration-handoff)
spanning 24 stories — landing the clustered deploy shape with per-session
Postgres advisory-lock leases, monotonic fencing tokens, RPO=0 object-
storage sync, and clean cross-pod session handoff.

An `e2e-test-design --audit` pass (2026-05-17) of `tests/e2e/` against
those 5 surfaces produced an unambiguous verdict: **the entire clustered
shape of CND has zero coverage in any taxonomy layer**. 9 Critical+High
findings + 5 Medium + 1 Low. The single-instance polish (`/readyz`,
`/metrics`, `_FILE` secrets, migration advisory lock, graceful shutdown)
also has no failure-mode tests.

This epic backfills the e2e program against CND. Two structural truths
shape the decomposition:

- **The clustered fixture is the keystone.** `tests/e2e/fixtures/portal/`
  is single-instance only. Until a `portalcluster` fixture exists that
  brings up N portals against a shared Postgres + shared object-storage
  service mock, none of the clustered-mode tests can run end-to-end.
- **The five test programs are mostly parallel after the keystone.**
  Lease-fencing, object-storage-sync, and routing-layer each have their
  own externally-visible invariants and can be designed independently.
  Hydration-handoff is the capstone — it composes all three.

The operational-polish coverage is independent of all of the above and
can run in parallel from day one (single-instance portal fixture suffices).

## Audit context

Full audit report (counts and severity rubric below) was produced by an
opus sub-agent reading **test files only** under `tests/e2e/` against the
five CND feature areas. The auditor was explicitly forbidden from reading
implementation under `internal/` or `cmd/` — that's the same mistake the
audited tests likely made.

**Counts** — by severity:
- Critical: 5 (F1 lease-fencing journey gap, F2 object-storage journey gap,
  F3 hydration-handoff journey gap, F4 routing-layer journey gap, F15
  clustered-fixture architectural prerequisite)
- High: 5 (F5 `/readyz`, F6 `/metrics`, F7 `_FILE` secrets, F8 migration
  advisory lock, F13 pod-kill chaos)
- Medium: 5 (F9 graceful-shutdown deadline, F10 object-storage URL parser
  fuzz, F11 fencing-token fuzz, F12 pack-manifest fuzz, F14 cross-pod clock
  skew)
- Low: 1 (F16 mock-boundary discipline reaffirmation — no violations found)

**Mock-boundary lens** — the existing e2e program follows service-level
mocking discipline correctly (Testcontainers Postgres, real Toxiproxy,
real WireMock, MailHog). CND coverage must maintain the same discipline:
MinIO Testcontainer for object storage (not an in-process S3 mock library),
real Postgres for advisory locks, real `cmd/jamsesh-router/` binary.

## Mock policy

Inherits the parent test program's policy (`epic-e2e-tests` body):
service-level mocks only. CND-specific catalog additions:

- **Object storage** → MinIO (`minio/minio:RELEASE...`) for S3 + S3-
  compatible. GCS and Azure backends covered via the S3-compatible URL
  scheme path in CI (provider-specific SDKs covered at unit level — going
  full multi-provider in e2e doubles the matrix without unique invariants).
- **Multi-pod portal** → multiple Testcontainers spawned by the new
  `portalcluster` fixture sharing one Postgres + one MinIO bucket.
- **Router** → real `cmd/jamsesh-router/` binary built into a container
  image, fronting the multi-pod portal.
- **Pod kill** → reuse existing Pumba pattern from `tests/e2e/chaos/
  runtime_and_clock_test.go` (already in the suite).
- **Network partition** → reuse existing Toxiproxy pattern (already in
  the suite for portal↔DB and portal↔OAuth chaos).
- **Cross-pod clock skew** → libfaketime via `LD_PRELOAD` at the container
  level on one pod; the other pod's clock stays normal.

In-process mocks for any of the above are disallowed.

## Decomposition

Six child features. The dependency graph:

```
operational-polish ──── (independent; single-instance fixture suffices)

cluster-fixture ─┬──── lease-fencing ──────────┐
                 ├──── object-storage-sync ────┼──── hydration-handoff
                 └──── routing-layer ──────────┘
```

`cluster-fixture` is the keystone — all four clustered features depend on
it. The middle band parallelizes three ways. Hydration-handoff is the
capstone (depends on all three middle-band features because it composes
acquire-via-router → hydrate-from-object-storage → release-on-eviction).

### Child features

- `epic-e2e-cnd-coverage-cluster-fixture` — `portalcluster` fixture + MinIO
  fixture + router image build; smoke spec that proves N-pod stack boots
  and one session round-trips. depends on: `[]`
- `epic-e2e-cnd-coverage-operational-polish` — `/readyz` + `/metrics` +
  `_FILE` secrets + migration advisory lock + graceful-shutdown deadline,
  all against the existing single-instance fixture. depends on: `[]`
- `epic-e2e-cnd-coverage-lease-fencing` — golden + failure + chaos + fuzz
  for per-session advisory-lock leases and monotonic fencing tokens.
  depends on: `[epic-e2e-cnd-coverage-cluster-fixture]`
- `epic-e2e-cnd-coverage-object-storage-sync` — golden RPO=0 invariant,
  failure on unreachable backend, chaos on partition, fuzz on the 4-scheme
  URL parser and pack manifest format. depends on:
  `[epic-e2e-cnd-coverage-cluster-fixture]`
- `epic-e2e-cnd-coverage-routing-layer` — consistent-hash, MCP-header
  pinning, 503/Retry-After re-dispatch, backend disconnect chaos. depends
  on: `[epic-e2e-cnd-coverage-cluster-fixture]`
- `epic-e2e-cnd-coverage-hydration-handoff` — clean session migration
  golden, hydration with missing pack failure, handoff-under-chaos. depends
  on: `[epic-e2e-cnd-coverage-lease-fencing, epic-e2e-cnd-coverage-object-
  storage-sync, epic-e2e-cnd-coverage-routing-layer]`

### Decomposition risks

- **Cluster-fixture is the bottleneck.** Four of the five clustered
  features cannot start until it lands. Design pass on `cluster-fixture`
  should produce an explicit acceptance bar — a single trivial multi-pod
  golden test (e.g., "session created on pod A is visible on pod B after
  handoff") that proves the fixture end-to-end before any content feature
  begins.
- **Hydration-handoff's three-way dependency.** It composes three other
  features; the temptation is to ship parts of it alongside lease-fencing
  or object-storage-sync. Resist that — the eviction half is meaningless
  without hydration, and shipping them together keeps the lifecycle test
  surface coherent (same mistake the parent CND epic flagged for the
  production code).
- **MinIO is the only object-storage backend in the e2e matrix.** Real
  GCS / Azure SDK behavior (workload-identity refresh, generation-match
  semantics) does NOT get e2e coverage here. That's acceptable: unit tests
  cover provider-specific glue, and adding GCS/Azure to e2e doubles the
  matrix without adding new invariants the S3-compat path doesn't already
  exercise. Document this gap in `object-storage-sync` feature design so
  it's visible.
- **Cross-pod clock skew (F14) may surface a real bug.** The test design
  is: advance one pod's clock past lease TTL while the other pod's clock
  is normal; both pods should agree on lease ownership (Postgres `now()`
  is the anchor). If implementation uses local clocks anywhere, this test
  catches a split-brain bug. Treat any failure as a backlog item via
  `/agile-workflow:park`, not as a test bug.

## Test integrity (inherited from parent test program)

Per `epic-e2e-tests` body and `CLAUDE.md`:

- **Park production bugs, don't hide them.** If a test the design specs
  will fail because the product is genuinely broken (e.g., a missing
  fencing-token check; a hydration path that silently truncates on a
  missing pack object), park the bug via `/agile-workflow:park`, land the
  failing test with a `skip` / `xfail` linked to the backlog id and a
  one-line reason. The failing test is a feature, not a defect.
- **Fix bad tests in-session.** Drifted fixtures or assertions get
  repaired as part of the stride.
- **Never game an assertion.** No `expect(true).toBe(true)`, no asserting
  on whatever the code happens to return.

The CND surface is exactly the kind of code where lying tests do real
damage — a green-but-tautological test against a split-brain fencing
check would actively mislead operators about the durability contract.

## Foundation references

- `docs/ARCHITECTURE.md` — Horizontal Scaling subsection (clustered shape
  + dual-layer storage)
- `docs/SELF_HOST.md` §13 (cloud-deploy recipes) and §14 (clustered mode)
- `docs/SPEC.md` — Deployment shape (lists all new env vars)
- `docs/SECURITY.md` — object-storage IAM operator-responsibility row
- `.work/active/epics/epic-cloud-native-deploy.md` — the production epic
  this coverage targets (now done)
- `.work/active/epics/epic-e2e-tests.md` — the parent test program (done);
  this epic extends its surface

## Acceptance criteria for the epic

- [ ] `tests/e2e/fixtures/portalcluster/` exists and is exercised by at
      least one green spec proving N-pod boot + session round-trip
- [ ] Each of the 5 CND features has at least one golden test asserting
      a user-visible invariant (no in-process mocks)
- [ ] Lease-fencing, object-storage-sync, and hydration-handoff each have
      at least one chaos test (the safety/durability surfaces)
- [ ] Operational-polish has failure-mode coverage for `_FILE` missing
      target and `/readyz` reporting unhealthy under DB outage
- [ ] All 6 child features land at stage:review or beyond
- [ ] No in-process mocks introduced; service-level discipline maintained
- [ ] CI runs the new tests in `make test-e2e`
- [ ] Any production bugs surfaced by the new tests are parked or fixed
      per the test-integrity rules above (not silenced)

## Non-goals

- **Provider-specific e2e coverage for GCS and Azure.** S3-compat via
  MinIO covers the invariants; provider-SDK glue is unit-test territory.
- **Multi-region failover scenarios.** The parent CND epic explicitly
  defers multi-region; e2e mirrors that scope.
- **Long-running soak tests.** Chaos coverage is "does it recover within
  SLO", not "does it stay healthy for 24h". Soak is a separate concern.
- **Performance benchmarks.** Coverage is invariant-focused; perf
  characterization belongs in `/agile-workflow:perf-design`.

## Autopilot decision log

- 2026-05-17 — Advanced `drafting → implementing` directly without invoking
  `/agile-workflow:epic-design`. The decomposition was already produced by
  the upstream `e2e-test-design --audit` pass (6 child features with
  declared `depends_on` chains and acceptance criteria recorded in this
  body). Re-running `epic-design` would either no-op or risk duplicating
  children. The children are the work targets from here on. Mirrors the
  precedent set in `epic-e2e-tests` for the same situation.

## Next

Each child feature gets designed independently via
`/agile-workflow:e2e-test-design <feature-id>` once it's at stage:drafting.
Or run `/agile-workflow:autopilot epic-e2e-cnd-coverage` to drive the
program end-to-end (autopilot will respect the dependency graph: cluster-
fixture first, then the three middle-band features in parallel, then
hydration-handoff; operational-polish runs independently throughout).

## Children complete (2026-05-17)

All 6 child features advanced to `stage: done`:

| Feature | Stage |
|---|---|
| `epic-e2e-cnd-coverage-cluster-fixture` | done |
| `epic-e2e-cnd-coverage-operational-polish` | done |
| `epic-e2e-cnd-coverage-lease-fencing` | done |
| `epic-e2e-cnd-coverage-routing-layer` | done |
| `epic-e2e-cnd-coverage-object-storage-sync` | done |
| `epic-e2e-cnd-coverage-hydration-handoff` | done |

Advancing epic `implementing → review`.

## Review (2026-05-17)

**Verdict**: Approve — epic delivered as briefed.

**Blockers**: none
**Important**: none (3 production bugs surfaced and parked are upstream work, not review blockers)
**Nits**: none

### Aggregate findings (epic-level lenses)

- **Capability completeness**: ✓ All 6 child features delivered. ~27 stories implemented across cluster-fixture (4), operational-polish (5), lease-fencing (5), object-storage-sync (7), routing-layer (6), hydration-handoff (5). All 16 audit findings (5 Critical / 5 High / 5 Medium / 1 Low) addressed.

- **Decomposition match**: ✓ The DAG promised at scope time materialized exactly — cluster-fixture as keystone, operational-polish independent, lease-fencing/object-storage-sync/routing-layer as parallel middle band, hydration-handoff as capstone.

- **Foundation-doc alignment**: ✓ Rolling-foundation honored. One production code change (`slog.InfoContext("db migrations applied", ...)` in `internal/db/migrate.go`) is purely additive observability — no SPEC.md or ARCHITECTURE.md assertion invalidated. No doc updates required.

- **Breaking changes**: ✓ Six portal-fixture extensions (`ContainerFiles`, `Logs`, `SendSignal`, `ContainerIP`, `State`, `Exec`) all backward-compatible. No public-API shifts; no production behavior changes beyond the additive log line.

- **Mock-boundary discipline**: ✓ Maintained throughout. MinIO Testcontainer (not in-process S3 mock), real Postgres advisory locks, real `cmd/jamsesh-router` binary, Toxiproxy for network injection, docker kill SIGKILL (Pumba pattern explicitly rejected for simplicity). No in-process mocks introduced.

### Production bugs surfaced and parked (the e2e program doing its job)

The audit's premise was that the clustered shape had zero coverage. The first end-to-end exercise of the path surfaced three real production gaps — exactly the class of bugs e2e tests exist to catch:

1. **`bug-router-static-discoverer-not-started`** (Important) — `cmd/jamsesh-router/main.go` has placeholder `_ = publishWithMetrics` / `_ = probe` assignments where `discovery.Static(...).Run(ctx, ring.SetPods)` should be wired. Dead pods stay in the routing ring forever; clients get 502 instead of router-mediated re-sharding. One-goroutine fix. Tracked in `.work/backlog/`.

2. **`object-storage-fail-fast-clustered-startup`** (Important) — AWS SDK v2 S3 client is lazy at construction; portal boots cleanly in clustered mode even with unreachable object storage. Should add a startup `HeadBucket` probe before the HTTP listener starts.

3. **`object-storage-write-rejected-silent-acceptance`** (potentially Critical) — possible silent 2xx on missing-bucket push; needs verification under load.

Tests for all three failure modes are `t.Skip`'d with backlog-id references per the test-integrity rules. None silenced; all documented.

### Architectural findings (not bugs)

- **Lazy lease acquisition**: portal acquires the advisory lock in post-receive, after HTTP 200 is committed. The synchronous "lease held elsewhere" 503 only surfaces through the router's retry path; direct-pod requests don't see it. Tests assert via the LeaseHolder Postgres oracle, not by trusting HTTP response shapes — non-tautological.

### Statistics

- 6 features designed + implemented + reviewed
- ~27 stories landed
- ~50+ commits on the main branch
- 5 backlog items produced (3 bugs + 2 documentation/follow-up gaps)
- New e2e packages: `fixtures/minio`, `fixtures/router`, `fixtures/portalcluster`
- New portal extensions (all backward-compatible): `ContainerFiles`, `Logs`, `SendSignal`, `ContainerIP`, `State`, `Exec`
- New tests across `golden/`, `failure/`, `chaos/`, `fuzz/`, `scaffolding/` — every taxonomy layer has CND coverage

### What's now possible

The cloud-native-deploy epic shipped with zero e2e coverage of its clustered shape. With this epic done, every CND surface has a test that exercises it end-to-end against real service-level mocks (MinIO, real Postgres, real router binary, Toxiproxy). The keystone smoke spec proves a 3-pod cluster + handoff cycle works end-to-end in CI. Future changes to CND code will see test failures when they break documented invariants — that's the whole point.

**Epic delivered as briefed; advancing to done.**
