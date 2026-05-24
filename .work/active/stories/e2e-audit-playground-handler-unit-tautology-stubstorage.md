---
id: e2e-audit-playground-handler-unit-tautology-stubstorage
kind: story
stage: implementing
tags: [testing, e2e-test, audit, playground]
parent: feature-e2e-playground-coverage-golden
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Playground handler unit tests assert against `stubStorage` map state — tautology risk; e2e counterpart needed for real filesystem assertions

## Severity
Medium

## Finding type
tautology

## Evidence

`internal/portal/playground/handler_test.go` defines a `stubStorage` at
lines 36-67 that implements the storage interface as an
`map[string]bool`. The test then asserts the handler set the map (line
504):

```
if !env.stor.repos[key] { ... }
```

This is a textbook tautology: the test verifies the handler called
`stubStorage.CreateRepo`, which the test itself defined to flip a map
bit. No assertion checks that a real bare repo exists on real disk, that
its config is correct, that pre-receive / post-receive hooks are
installed, or that the `playground` org-id is used in the on-disk path.

Identical patterns:
- `TestCreatePlaygroundSession_RepoCreated` — asserts on stub map.
- `TestDestruction_RepoRemovedFromStorage` (destruction_test.go) —
  asserts on stub map.
- Every handler test runs through `httptest.NewServer(r)` (line 309) +
  `fixedClock{2026-05-23 12:00 UTC}` (line 243) — no real Postgres, no
  real time.

The `TestJoinPlaygroundSession_Success` / `..._WithNickname_UsesIt`
parked bug
(`.work/backlog/bug-playground-join-with-nickname-returns-410-on-fresh-session.md`)
is precisely the kind of regression a fixed-clock unit suite missed for
an entire release cycle: the comment "Possible clock-injection mismatch
between handler and test" in the parked bug is the smoking gun. If the
handler accidentally calls `time.Now()` in one place instead of the
injected clock, the unit suite passes against the fixed clock but blows
up against real time — exactly the gap an e2e test would have caught.

## Why this matters

A test that asserts the handler called the test's own stub method is
indistinguishable from no test at all when the production storage
implementation breaks. The
`gate-cruft-playground-ratelimit-test-dead-time-second-line.md` finding
already flagged adjacent test debt in the same package. The e2e
counterparts proposed by other findings in this audit
(`e2e-audit-playground-solo-create-push-tombstone-journey`,
`e2e-audit-playground-abandonment-destruction-sweep-journey`) are the
specific remedies; this finding is the **rationale for prioritizing
them** and the explicit identification of the tautology pattern.

## Suggested remedy

Two coordinated changes:

1. **Don't remove the unit tests** — they're still useful for fast
   feedback on handler control flow. Instead, every unit test that
   asserts on `stubStorage.repos[key]` should have a sibling e2e test
   in `tests/e2e/golden/playground_*_test.go` that asserts on real
   on-disk state via `dockerExecExists` (pattern from
   `lifecycle_evict_on_lease_release_test.go > VerifyCacheEvicted`).
2. **Audit the handler for clock-injection completeness** as part of
   the parked-bug fix. If the unit tests are tautological because they
   inject a clock that the handler partially ignores, the fixed-clock
   pattern itself is suspect — every code path that compares against
   "now" must use the same injected clock or the divergence will recur.

## Test sketch

The e2e siblings are sketched in the other audit findings; this finding
contributes the **assertion shape** they all need:

```go
// In every e2e test that creates a playground session, after the
// `POST /api/playground/sessions` returns 201:
repoPath := portalRepoPath("playground", sess.ID)
require.True(t, dockerExecExists(t, p, repoPath),
    "create must produce a real bare repo on disk, not just a stub map flip")
require.True(t, dockerExecExists(t, p, filepath.Join(repoPath, "hooks", "pre-receive")),
    "pre-receive hook must be installed for content-cap enforcement")
require.True(t, dockerExecExists(t, p, filepath.Join(repoPath, "hooks", "post-receive")),
    "post-receive hook must be installed for event emission")
```
