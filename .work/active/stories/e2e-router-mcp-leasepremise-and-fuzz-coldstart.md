---
id: e2e-router-mcp-leasepremise-and-fuzz-coldstart
kind: story
stage: review
parent: e2e-cloud-native-multipod-suite-red
tags: [portal, infra, testing, bug]
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-31
updated: 2026-05-31
---

# Fix last two in-scope e2e reds: router-MCP lease-premise + fuzz fencing cold-start

Closes the final two reds under epic `e2e-cloud-native-multipod-suite-red`.
Both were **test-reliability / wrong-precondition** problems, not product bugs.
No product code changed.

## RED 1 — golden/TestRouterMCPSessionHeader (lease premise)

### Root cause (verified)
Subtest `mcp_jam_session_id_pins_to_handshake_pod` failed with
`RequireLeaseHolder: no pod holds lease for "<sid>" after 5s`. The same class
of failure already skipped four sibling router tests (`router_consistent_hash`,
`router_hint_cache`, `router_lease_unavailable`, `router_backend_dead`) — see
`.work/backlog/idea-router-e2e-lease-premise.md`.

The per-session Postgres advisory lease
(`pg_try_advisory_lock(hashtext(sessionID))`, held by the portal's
`LifecycleManager`) is acquired **only on the git/object-storage path** — the
`LifecycleManager` is wired into the git smart-HTTP handler in
`cmd/portal/main.go`. REST and MCP requests never acquire it. The old test
created the session via REST and then waited for a lease holder that REST never
produces, so `RequireLeaseHolder` always timed out.

Crucially, the router does **not** consult the lease at all: it routes purely by
its consistent-hash ring (`internal/router/ring` + the soft-coordinator hint
cache in `internal/router/proxy`), keyed on the session id extracted from the
`Jam-Session-Id` header (`internal/router/extract/extract.go`). So MCP pinning
depends on the ring, not the lease.

### Chosen disposition — git-trigger (PREFERRED), genuinely green
Rather than skip-with-link (the sibling escape hatch), I took the preferred
faithful path: drive a **real git push through the router first**. The ring
routes that push to a pod, the pod's post-receive object-storage sync acquires
the advisory lease, and — because the ring is deterministic for a fixed pod
set — that lease holder is exactly the pod the ring also routes the session's
MCP tool calls to. The test then asserts every `Jam-Session-Id`-bearing MCP tool
call pins to the lease holder (observed independently via `pg_locks`).

This makes the test genuinely green **and** genuinely verifies router MCP
stickiness against an independent routing signal — not a tautology. The pinning
assertion (`require.Equal(firstHolder, holder)` across 5 calls) is unchanged;
only the precondition that establishes a real lease holder was fixed.

### Edits (`tests/e2e/golden/router_mcp_session_header_test.go`)
- Added the `gitclient` fixture import.
- `TestRouterMCPSessionHeader` now resolves `userID` (via the existing
  package-level `leaseFenceGetMe`) and passes it to the subtest.
- `testMCPJamSessionIDPinsToPod` now clones + commits + pushes via the router
  (`gitclient.Clone/Commit/Push`) to seed a real lease before the MCP handshake;
  `RequireLeaseHolder` timeout raised 5s → 15s to cover post-receive sync.
- File-level doc comment rewritten to explain the lease/ring relationship and
  why the test is sound.

## RED 2 — fuzz/TestFencingTokenFuzz (cold-start flake by design)

### Root cause
The test cold-started **two** portal containers (a bootstrap cluster + a fresh
"hot" cluster) **per seed** — ~27 seeds × 2 = ~54 cold starts. On the shared
Docker host, parallel cold-starts saturated the daemon and stalled boots past
the readiness deadline (`start container failed after 5 attempts: wait until
ready: context deadline exceeded` → `pod 0 is nil after startup`). The fencing
logic itself was correct.

### Fix — single shared cluster (PREFERRED: cluster reuse)
Verified the seeds need **no cluster isolation** — only a distinct session id.
The manifest store loads each session's manifest from MinIO **lazily on the
session's first git access** (hydration in `lifecycle.AcquireForRequest` →
`ManifestStore.Load`), keyed per session; there is no shared mutable in-process
state across sessions. REST session creation (`internal/portal/sessions/handler.go`)
is a pure Postgres write — it does not touch object storage, hydration, or the
lifecycle manager — so creating a session on the same shared portal that later
serves the push is safe and does not read/write the manifest before injection.

`TestFencingTokenFuzz` now starts **one** portal pod, signs in once, and builds
a shared `fencingFuzzEnv` (pod + token + userID + orgID). Each seed: creates a
fresh session on that pod, injects its manifest into MinIO under that session's
key, then clones + pushes (the first git access, which hydrates from the
injected manifest). One cold-start instead of ~54.

Because the portal log buffer is now shared across seeds, each seed captures a
per-seed log-length baseline (`fencingLogLen`) and the panic check inspects only
the new suffix (`fencingLogsSince`), so a panic is attributed to the correct
seed and a prior seed's logs cannot false-positive a later one. Seeds run
sequentially (no `t.Parallel()`), which the baseline scheme relies on.

### Edits (`tests/e2e/fuzz/fencing_token_test.go`)
- Added `fencingFuzzEnv` (shared pod/auth/org); added `portal` fixture import.
- `runFencingSeed` now takes `*fencingFuzzEnv` and runs against the shared pod
  (no per-seed `portalcluster.Start`); added `fencingLogLen` / `fencingLogsSince`
  for shared-log panic attribution.
- `TestFencingTokenFuzz` starts one cluster + one auth identity once and reuses
  it for every corpus + random seed.
- `limiter_test.go` left intact — `pack_manifest_test.go` still uses
  `acquireStartupSlot`; the fencing fuzz simply no longer needs it (one
  cold-start). No `acquireStartupSlot` call remains in the fencing path.

`TestFencingTokenRejectionIsExplicit` (out of scope, only 2 cold-starts total)
was left unchanged.

## Verification (Docker exclusive; GOTMPDIR/TMPDIR=$HOME/.cache/gotmp; -p 1)

Reused existing `jamsesh/portal:e2e` / `jamsesh/router:e2e` images (test-only
changes; no rebuild).

- `TestRouterMCPSessionHeader`: **PASS ×2** (12.4s, 7.8s). Lease holder = pod 1
  established by the push; all 5 MCP tool calls pinned to pod 1.
- `TestFencingTokenFuzz`: **PASS ×3** (7.2s, 12.4s, 12.6s). All corpus + random
  seeds green; no cold-start stalls, no `pod 0 is nil`, no panics.
- Regression: `TestPackManifestFuzz` (still uses the shared limiter) **PASS**
  (45s) — limiter untouched, no regression.

Both reds reliably GREEN. No product code changed.
