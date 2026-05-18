---
id: epic-e2e-cnd-coverage-hydration-handoff-chaos
kind: story
stage: review
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-hydration-handoff
depends_on: [epic-e2e-cnd-coverage-hydration-handoff-golden]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
implemented: 2026-05-17
---

# Hydration-Handoff Chaos — Pod Kill + Object-Storage Latency

## Scope

Two chaos scenarios:

1. **Pod kill during active session** (F13 handoff half) — SIGKILL the holding
   pod while an active session has ack'd commits. Assert zero data loss and
   that the client's subsequent requests succeed after pod B hydrates.
2. **Handoff under object-storage chaos** — Toxiproxy latency on the
   portal→MinIO path during hydration. Assert handoff eventually completes
   within an extended SLO and no commits are lost.

**Design boundary (F13 coordination):** `lease_holder_killed_test.go` (in
`epic-e2e-cnd-coverage-lease-fencing`) asserts the lease-ownership invariants
(auto-release, monotonic-token, system-recovery). This story's pod-kill test
asserts the **user-visible handoff outcome** — draft tip preserved, all ack'd
commits visible on pod B. Do not re-assert monotonic fencing tokens here; that
is lease-fencing territory. Each layer asserts its own invariant.

## Unit 1: `tests/e2e/chaos/handoff_under_pod_kill_test.go`

```
Package: chaos_test
Test: TestHandoffUnderPodKill
```

**Invariant:** "When the pod holding a session is SIGKILLed, all commits
acknowledged before the kill are present on the surviving pod after it
hydrates. No user-visible data loss across a hard pod crash."

**Stack:** `postgres.Start` + `minio.Start` + `mailhog.Start` +
`portalcluster.Start(Pods: 2, Router: true)` with short heartbeat:

```go
PortalExtraEnv: map[string]string{
    "JAMSESH_LEASE_HEARTBEAT_INTERVAL_S": "2",
    "JAMSESH_EMAIL_PROVIDER":            "smtp",
    // ... mailhog SMTP vars
},
```

**Setup:**
1. Alice signs in, creates org + session.
2. Push 5 commits via the router. Record each commit SHA in `ackedSHAs`.
   `gitclient.Push` returning successfully = push acknowledged (RPO=0 —
   all acked commits are in MinIO per the object-storage-sync coverage).
3. `holderPod := c.RequireLeaseHolder(ctx, t, sessionID, 10*time.Second)`.
4. Record `draftTipBefore` from `holderPod` pod directly.

**Chaos action:**
5. `c.Kill(ctx, t, holderPod)` — SIGKILL the holding pod.
6. `survivorIdx := (holderPod + 1) % len(c.Pods)`.

**Handoff assertions:**
7. `WaitForHydration(ctx, t, c.Pods[survivorIdx], orgID, sessionID, accessToken, 30*time.Second)`
   — wait for pod B's cache to be ready.
8. Push a 6th commit via `c.Pods[survivorIdx].URL` directly (not through the
   router — static-discoverer bug means the router may still route to the dead
   pod). Record `postKillSHA`.
9. `RequireLeaseHolder(ctx, t, sessionID, 15*time.Second)` — confirm the
   survivor holds the lease post-hydration.
10. **Draft tip assertion:** Query survivor's draft tip. It must equal or
    advance past `draftTipBefore`. All 5 acked SHAs must be reachable from
    the new draft tip: `gitclient.LsRemote` on `survivorIdx` pod, then
    `git merge-base --is-ancestor <ackedSHA> <draftTip>` for each.
11. **Bucket assertion:** `mn.ListObjects("sessions/"+sessionID+"/")` must
    be non-empty and include objects for the 6th commit.

**Router note:** After the kill, do NOT assert router-mediated requests succeed
without a retry window — `bug-router-static-discoverer-not-started` means the
dead pod stays in the ring. The test addresses the survivor directly.
Document this router limitation in the test with a `t.Logf`:

```go
t.Logf("NOTE: bug-router-static-discoverer-not-started — router may still " +
    "route to dead pod; asserting directly against survivor pod %d", survivorIdx)
```

**Coordination comment (must be in test source):**

```go
// Design boundary: lease-ownership invariants (auto-release, monotonic-token)
// are asserted in lease_holder_killed_test.go (epic-e2e-cnd-coverage-lease-fencing).
// This test owns the user-visible handoff outcome: all acked commits present,
// no data loss. Do not add fencing-token assertions here.
```

---

## Unit 2: `tests/e2e/chaos/handoff_under_object_storage_chaos_test.go`

```
Package: chaos_test
Test: TestHandoffUnderObjectStorageChaos
```

**Invariant:** "When the portal→MinIO path has a transient latency spike
during pod B's hydration, hydration eventually completes within an extended
SLO (45s). No commits acknowledged before the chaos window are lost."

**Stack:** `postgres.Start` + `minio.Start` + `mailhog.Start` +
`toxiproxy.Start` + `portalcluster.Start(Pods: 2, Router: false)` with
`PortalExtraEnv` wiring Toxiproxy as the MinIO proxy endpoint.

**Toxiproxy wiring (reuse pattern from `object_storage_partition_test.go`):**
- Create a Toxiproxy proxy `portal-minio` forwarding to `minio.ContainerEndpoint`.
- Override `JAMSESH_OBJECT_STORAGE_ENDPOINT_URL` in the cluster to point at
  the Toxiproxy listen address instead of the MinIO container directly.
- Both pods route through the proxy — this covers both the push sync path and
  the hydration fetch path.

**Setup:**
1. Alice signs in via pod 0, creates org + session.
2. Push 5 commits via pod 0. All 5 acked (RPO=0). Record `draftTipBefore`.
3. `c.RequireLeaseHolder` confirms pod 0.

**Chaos action:**
4. Inject Toxiproxy latency on `portal-minio`:
   `tp.AddLatency(ctx, t, "portal-minio", "hydration-chaos-latency", 4000)`
   — 4 000ms latency per packet on the upstream path.
5. `c.GracefulDrain(ctx, t, 0, 45*time.Second)` — drain pod 0. The drain
   itself may be slow because the shutdown sequence flushes remaining sync
   writes through the toxic.

**Assertions:**
6. `WaitForHydration(ctx, t, c.Pods[1], orgID, sessionID, accessToken, 45*time.Second)`
   — extended SLO; hydration is slow through the toxic.
7. Clear the toxic: `tp.RemoveToxic(ctx, t, "portal-minio", "hydration-chaos-latency")`.
8. Push a 6th commit via pod 1. Confirm success.
9. Draft tip on pod 1 must include all 5 pre-chaos commits plus the 6th.
   Use `RequireSessionStateMatch` pattern: query draft tip, assert it advances
   past `draftTipBefore`.
10. `mn.ListObjects` for the session must include objects for all 6 commits.

**Timing note:** 4 000ms latency × 8 workers (`JAMSESH_HYDRATION_WORKERS=8`)
means parallel downloads each take ~4s. For a session with O(10) pack objects
this is ~5-10s of hydration time. The 45s SLO is ~4-9× the expected hydration
time — conservative but not infinite.

## Acceptance criteria

- [ ] `TestHandoffUnderPodKill` green; all 5 acked SHAs reachable from
      survivor's draft tip after SIGKILL
- [ ] `TestHandoffUnderPodKill` test source includes design-boundary comment
      coordinating with lease-fencing F13
- [ ] `TestHandoffUnderObjectStorageChaos` green; hydration completes within
      45s SLO under 4s Toxiproxy latency; no acked commits lost
- [ ] Neither test adds fencing-token monotonicity assertions (lease-fencing
      territory)
- [ ] No in-process mocks

## Test integrity (from parent feature)

- If `TestHandoffUnderPodKill` fails because an acked SHA is not reachable
  from the survivor's draft tip after hydration — that is a **Critical**
  production bug (data loss). Park via `/agile-workflow:park`. Land the
  failing assertion with `t.Skip` + backlog id + comment naming the
  commit SHAs that are missing.

- Chaos tests are designed to assert on graceful degradation. A push that
  fails during the chaos window (object-storage latency test, step 5) is
  acceptable — the test does not assert on the drain-time push success,
  only on the post-chaos state.

- Never game: do not increase the SLO to infinity to make the test pass.
  If 45s is reliably insufficient for the hydration under 4s latency,
  investigate the hydration worker count or parallel download behavior —
  do not simply raise the timeout.

## Implementation Notes (2026-05-17)

Both test files implemented and verified clean with `go build ./chaos/...` and
`go vet ./chaos/...`.

### `tests/e2e/chaos/handoff_under_pod_kill_test.go`

- `TestHandoffUnderPodKill`: 2-pod cluster + router, short heartbeat (2s).
- Pushes 5 commits via router, captures all 5 SHAs from `gitclient.Commit`.
- `c.Kill(holderPod)` → `c.WaitForHydration(survivorIdx, 30s)` → push commit 6
  directly on the survivor (bypass router for bug-router-static-discoverer).
- State assertion: `podKillRequireAncestor` clones from the survivor, fetches,
  and runs `git merge-base --is-ancestor <ackedSHA> <currentTip>` for
  `draftTipBefore` and all 5 individually acked SHAs. Non-tautological: actual
  commit ancestry, not just HTTP status.
- Design-boundary comment in file header and in test body.
- No fencing-token assertions.

### `tests/e2e/chaos/handoff_under_object_storage_chaos_test.go`

- `TestHandoffUnderObjectStorageChaos`: Toxiproxy (port 9101) in front of
  MinIO; 2-pod cluster wired through proxy (Router: false). Startup order:
  MinIO → Toxiproxy → portal cluster.
- Pushes 5 commits via pod 0 (no toxic yet — baseline RPO=0 holds).
- Injects 4 000ms Toxiproxy latency on `portal-minio`; drains pod 0 (up to
  45s for slow flush); waits for pod 1 hydration with 45s SLO.
- Removes toxic; pushes commit 6 via pod 1; asserts all 5 pre-chaos SHAs
  are ancestors of pod 1's current tip via `git merge-base --is-ancestor`.
- Defensive `t.Cleanup` removes the toxic on any early exit.
- `stosStripScheme` strips `http://` from `mn.ContainerEndpoint` to give
  Toxiproxy its required bare `host:port` upstream address.

### Design decisions
- `gitMergeBaseIsAncestor` is defined in `handoff_under_pod_kill_test.go` and
  used by both tests (both in `package chaos_test` so same package scope).
- `stosStripScheme` mirrors `partitionStripScheme` in
  `object_storage_partition_test.go` — kept local rather than extracted to
  avoid cross-test coupling.
- Proxy port 9101 used (not 9001) to avoid collision with the existing
  `object_storage_partition_test.go` proxy on 9001.
