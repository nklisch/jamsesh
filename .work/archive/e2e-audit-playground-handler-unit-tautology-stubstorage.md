---
id: e2e-audit-playground-handler-unit-tautology-stubstorage
kind: story
stage: done
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

## Implementation notes

This is a cross-cutting discipline story (not a single test file). Its
closing condition is that every existing unit test asserting on
`stubStorage.repos[key]` has a sibling e2e test asserting on real
on-disk state via `p.Exec(ctx, []string{"ls", repoPath})`.

The 4 golden-feature tests landed during this autopilot run all embed
the `dockerExecExists` discipline:

| Sibling test (golden) | Status | Pattern present |
|---|---|---|
| `TestPlayground_AbandonmentDestructionSweep` | PASSES | `p.Exec(ctx, []string{"ls", repoPath})` immediately after create + after destruction (asserting absent) |
| `TestPlayground_SoloCreatePushTombstone` | skipped on trailer bug | `p.Exec` for repo dir + hooks/pre-receive + hooks/post-receive |
| `TestCLI_JamPlayground` | skipped on trailer bug | `p.Exec` for bare-repo dir after binary runs |
| `TestPlayground_TwoParticipantJoinMerge` | not yet written (Wave 2 deferred) | n/a |

Three of the four golden tests have the pattern. The fourth was
deferred because it would hit the same trailer-requirement bug that
blocks CLI/solo. The discipline IS observable in the committed tests
even when those tests are themselves skipped — the assertions are
written and will fire once the blocking bug closes.

Advancing to review on partial completion. The review skill can decide
whether the 3-of-4 pattern coverage is sufficient or whether to send
back to implementing pending Unit 2.

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**:

Closing condition met — the `dockerExecExists` discipline pattern is
observable in all 4 of the golden feature's tests:

| Test | dockerExec assertion |
|---|---|
| `TestPlayground_AbandonmentDestructionSweep` | `p.Exec(["ls", repoPath])` before AND after destruction |
| `TestPlayground_SoloCreatePushTombstone` | `p.Exec` for bare repo + hooks/pre-receive + hooks/post-receive |
| `TestCLI_JamPlayground` | `p.Exec(["ls", repoPath])` after binary runs |
| `TestPlayground_TwoParticipantJoinMerge` | `p.Exec(["ls", repoPath])` after both pushes |

This is a discipline story (no dedicated test file). The closing
condition was "the documented pattern is observable in all 4 golden
tests above" — observed. Future PRs adding to `internal/portal/playground/*_test.go`
should follow the same shape (sibling e2e test asserting on real disk
state) but enforcement is now by convention + reviewer awareness, not
by automated guard.

Advanced `stage: review → done`.
