---
id: epic-e2e-cnd-coverage-lease-fencing-failure
kind: story
stage: done
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-lease-fencing
depends_on: [epic-e2e-cnd-coverage-lease-fencing-golden]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E CND Lease Fencing — Failure Mode

## Scope

Implement two failure-mode test files:
- `tests/e2e/failure/lease_already_held_test.go` (F1)
- `tests/e2e/failure/stale_fencing_token_rejected_test.go` (F1 + F11 partial)

These cover the two documented failure modes of the lease-fencing system:
a second pod receiving a request for a session held by the first, and the
object-storage sync layer rejecting a write with a stale fencing token.

## Implementation units

### Unit 1: `tests/e2e/failure/lease_already_held_test.go`

**Invariant:** when pod A holds session S's lease and pod B receives a
push for session S directly (bypassing the router), pod B must return 503
with a `Retry-After` header. The `error` JSON field must indicate lease
contention.

**Stack:** 2-pod cluster, `Router: false` (tests direct-pod behavior).

```go
package failure_test

func TestLeaseAlreadyHeld(t *testing.T) {
    ctx := context.Background()

    pg := postgres.Start(ctx, t, postgres.Options{})
    mn := minio.Start(ctx, t, minio.Options{})
    mh := mailhog.Start(ctx, t)
    c := portalcluster.Start(ctx, t, portalcluster.Options{
        Pods: 2, Postgres: pg, ObjectStore: mn, Router: false,
    })

    // Establish a session and push via pod 0 to claim the lease.
    alice := authflow.SignInViaMagicLink(ctx, t, c.Pods[0], mh,
        leaseFenceEmail(t, "alice-already-held"))
    orgID := authflow.CreateOrg(ctx, t, c.Pods[0], alice.AccessToken, "LeaseHeld Org")
    sessionID := createLeaseFenceSession(ctx, t, c.Pods[0], alice.AccessToken, orgID,
        "lease-already-held")

    pushDirectToPod(ctx, t, c.Pods[0], orgID, sessionID, alice)

    // Confirm pod 0 holds the lease.
    holder := c.RequireLeaseHolder(ctx, t, sessionID, 10*time.Second)
    if holder != 0 {
        t.Fatalf("expected pod 0 to hold lease, got pod %d", holder)
    }

    // Now push directly to pod 1 (same session). This MUST 503.
    resp := pushDirectToPodNoFail(ctx, t, c.Pods[1], orgID, sessionID, alice)
    defer resp.Body.Close()

    // SAFETY-CRITICAL ASSERTION: pod 1 must not serve a session it doesn't own.
    if resp.StatusCode != http.StatusServiceUnavailable {
        body, _ := io.ReadAll(resp.Body)
        t.Fatalf("lease_already_held: pod 1 returned %d (want 503) body=%s",
            resp.StatusCode, body)
    }

    // Assert Retry-After is present (any non-empty value).
    if resp.Header.Get("Retry-After") == "" {
        t.Errorf("lease_already_held: 503 response missing Retry-After header")
    }

    // Assert error envelope shape. The exact error code is informational until
    // PROTOCOL.md documents the lease-contention code.
    body, _ := io.ReadAll(resp.Body)
    var env errorEnvelope
    if err := json.Unmarshal(body, &env); err != nil {
        t.Errorf("lease_already_held: response body is not a valid error envelope: %v\nbody: %s",
            err, body)
    }
    // Expected: "lease.held_elsewhere" or "dep.lease_held" or similar.
    // Document whatever is returned; if missing, file a story to add the code to PROTOCOL.md.
    t.Logf("lease_already_held: 503 error code = %q (expected 'lease.held_elsewhere')", env.Error)
    if env.Error == "" {
        t.Errorf("lease_already_held: 503 response has empty error code (violates PROTOCOL.md envelope contract)")
    }
}
```

### Unit 2: `tests/e2e/failure/stale_fencing_token_rejected_test.go`

**Invariant:** a write to the object-storage sync path carrying a stale
fencing token (lower than the current stored token) is rejected. The
rejection surfaces as a documented error, not as a silent accept or a panic.

**Approach — direct Postgres manipulation (no new production endpoint):**
1. Push session S via pod 0 to establish lease with token T1. Record T1 via
   `c.FencingTokenForSession`.
2. Kill pod 0 (`c.Kill(0)`) — this releases the PG connection and the
   advisory lock.
3. Wait for pod 1 to acquire the lease for session S and record token T2
   (via `c.WaitForLeaseMigration` + `c.FencingTokenForSession`). Verify T2 > T1.
4. Now attempt to push to pod 0 again (the killed pod is gone; this won't
   work). Instead, the test manipulates the object-storage manifest directly
   via the MinIO fixture to write a fake manifest entry with token T1, then
   tries to push a new commit via pod 1. The manifest conditional-write check
   should compare pod 1's token (T2) against the stored T1 and accept (T2 > T1).
5. **The actual stale-token scenario:** roll back the Postgres sequence step
   by inserting a fake lease row with a high token T3 into the `leases`
   table (simulating a future pod that had a higher token), then force the
   manifest's stored token to T3 via MinIO, then push a commit from pod 1
   (token T2 < T3). The `objectstore.Syncer` should reject with `ErrFenced`
   and the push should fail.

**Note:** this is operationally complex. If the MinIO manifest format is
not public enough to manipulate from a test, file a follow-on story for a
`/test/inject-stale-manifest` endpoint (behind a build tag). Land the test
framework with a `t.Skip("stale-token injection requires manifest format
exposure — see <follow-on-story-id>")` until the mechanism is in place.

**The safety invariant is non-negotiable:** the test must verify that pod 1
does not silently write with a stale token. If the test mechanism cannot
be completed cleanly in this stride, skip with a documented reason. Do NOT
assert a different invariant to make the test green.

```go
func TestStaleFencingTokenRejected(t *testing.T) {
    // ... setup 2-pod cluster ...
    // ... establish T1 on pod 0 ...
    // ... kill pod 0, wait for pod 1 at T2 ...
    // ... inject T3 > T2 into manifest via MinIO ...
    // ... push from pod 1 (token T2 < T3) ...
    // ... assert push fails with documented error (not 200, not panic) ...
}
```

**Assertion targets:**
- Push from "stale pod" returns non-200 (documented error code, not panic)
- No object written to MinIO with the stale token (verify via `mn.ListObjects`)

## Acceptance criteria

- [ ] `TestLeaseAlreadyHeld` green: 503 + Retry-After + non-empty error code.
- [ ] `TestStaleFencingTokenRejected` either green or `t.Skip` with a
      documented reason pointing to a backlog item for the injection mechanism.
- [ ] No in-process mocks.
- [ ] No assertion gamification: the 503 assertion for `lease_already_held`
      must fail the test if the portal returns 200 or 5xx.

## Test integrity

**Park production bugs, don't hide them.** If `TestLeaseAlreadyHeld` finds
that pod B returns 200 (split-brain), park via `/agile-workflow:park` and
`t.Fatal`. If `TestStaleFencingTokenRejected` finds the write succeeds with
a stale token, that is a critical split-brain bug — park it immediately.

**If the manifest format cannot be manipulated from the test:** use
`t.Skip("<story-id>: stale-token injection needs manifest format exposure")`
rather than writing a fake version of the invariant. The skipped test is
more honest than a passing test that doesn't cover the invariant.

**Fix bad tests in-session.** If `pushDirectToPodNoFail` in the test helper
doesn't exist yet, write it (a variant of the push helper that returns the
`*http.Response` without calling `t.Fatal`). This is test-debt, not a
production bug.

**Never game an assertion.** Do not change the 503 assertion to `!= 200`
to accommodate a 500 or other non-intended status. 503 is the documented
behavior; 500 would indicate a production bug to park.

---

## Implementation notes (2026-05-17)

### Files landed
- `tests/e2e/failure/lease_already_held_test.go` — `TestLeaseAlreadyHeld`
- `tests/e2e/failure/stale_fencing_token_rejected_test.go` — `TestStaleFencingTokenRejected`

### Design decisions

**`TestLeaseAlreadyHeld`** — advisory-lock injection approach (same as
`router_lease_unavailable_test.go`). Rather than racing two real pods (timing
sensitive), the test process holds the Postgres advisory lock from a dedicated
DB connection (`pg_advisory_lock(hashtext(sessionID)::oid)`) so neither portal
pod can acquire the session lease. A git push directly to pod 0 triggers the
post-receive lease acquisition path, which sees the lock is held and returns
503. The test asserts 503 status, Retry-After header presence, and non-empty
error code. A secondary HTTP probe against the git info/refs endpoint asserts
the envelope shape independently of git's exit code parsing.

**`TestStaleFencingTokenRejected`** — manifest injection via `mn.GetObject` /
`mn.PutObject`. The manifest format is public JSON (`staleManifest` mirrors the
`Manifest` struct in `manifest.go`). The test reads the real on-disk manifest
after a successful push, replaces `FencingToken` with T3 = T2 + 1000, and
writes it back. A subsequent push from the surviving pod (with token T2 < T3)
must be rejected by `ManifestStore.Save`'s pre-flight check
(`onDisk.FencingToken > m.FencingToken → ErrFenced`). The test also verifies
the manifest is unchanged after the rejected write (T3 still on-disk, not T2).

**Skip paths (honest, not hiding bugs):**
- If `mn.GetObject` fails (manifest not yet written — lazy acquisition), the
  test skips with a documented reason and a follow-on story reference.
- If `mn.PutObject` fails (ETag conflict or permission), same skip treatment.
- Neither skip is triggered by the actual portal returning 200 on a stale push;
  that path is always a `t.Fatal`.

**`go build ./failure/...` + `go vet ./failure/...` both pass cleanly.**

### Deviations from story body

- The story sketched `pushDirectToPodNoFail` returning `*http.Response`. Implemented
  instead as two separate helpers: `leaseHeldAttemptPush` (returns int status from git
  exit-code parsing) and `leaseHeldProbeGitEndpoint` (returns `*http.Response` for
  header inspection). This is cleaner than combining git push + HTTP probe in one function.
- `TestLeaseAlreadyHeld` uses Router: false (direct-pod cluster) as the story specified;
  advisory lock injection is the mechanism rather than requiring a real push to one pod
  to happen first. This makes the test deterministic and non-racy.
- The stale-token test uses `Router: true` (needs surviving pod after Kill) rather than
  `Router: false` from the lease-already-held test — each test configures the cluster
  for its own scenario.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: Minor helper duplication (leaseHeldRandEmail / staleFencingRandEmail both do the same 4-byte hex pattern) — could be package-shared, not worth a follow-on.

**Notes**: Advisory-lock injection approach for `TestLeaseAlreadyHeld` is deterministic and correct — the test-process holds `pg_advisory_lock(hashtext(sessionID)::oid)` which matches the portal's acquire key exactly. The 503 assertion uses exact status-code match; 200 triggers a fatal (not a skip). The three `t.Skip` paths in `TestStaleFencingTokenRejected` all reference "stale-token-injection-needs-manifest-format-exposure" and are infrastructure-availability guards, not hidden bugs — each covers a case where the test cannot construct the stale-token scenario (manifest not yet on-disk, not parseable, or write rejected). The safety-critical path (portal returns 200 on stale push) is always a `t.Fatal`, never skipped. Manifest post-write integrity check correctly uses `t.Errorf` (not Fatalf) so both the token check and the rejection assertion contribute to the failure report. Deviations from the story design (two helpers vs. one combined helper) are acknowledged and justified.
