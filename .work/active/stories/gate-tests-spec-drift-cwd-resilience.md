---
id: gate-tests-spec-drift-cwd-resilience
kind: story
stage: done
tags: [testing, portal, infra]
parent: feature-test-spec-drift-and-coverage
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-24
updated: 2026-05-25
---

# `TestEventTypeConstants_MatchOpenAPIYAML` path-resolution under non-default cwd not asserted

## Priority
Low

## Spec reference
Item: `story-spec-discipline-drift-ci-check`

Acceptance criterion: Story note: "Path lookup: use a `runtime.Caller`-based or relative-path discovery so the test still works when run from a different cwd."

## Gap type
adversarial-spec-silent (resilience)

## Suggested test
Add a sub-test that invokes the comparison from a temp cwd via
`t.Chdir(t.TempDir())` to confirm path discovery is robust.

## Test location (suggested)
`internal/portal/events/spec_drift_test.go`

## Implementation

Add `TestEventTypeConstants_MatchOpenAPIYAML_NonDefaultCwd` (or as a
`t.Run("from_temp_cwd", ...)` sub-test inside the existing test) to
`internal/portal/events/spec_drift_test.go`.

Steps:
1. Check project `go.mod` for minimum Go version. If >= 1.24, use
   `t.Chdir(t.TempDir())`. If < 1.24, use:
   ```go
   orig, _ := os.Getwd()
   _ = os.Chdir(t.TempDir())
   t.Cleanup(func() { _ = os.Chdir(orig) })
   ```
2. After changing to the temp dir, run the same enum↔AllTypes comparison
   that the parent test runs.
3. Consider extracting the shared comparison logic into a private helper
   `checkSpecDrift(t *testing.T)` to eliminate copy-paste between the two
   test functions.

The key invariant to verify: `runtime.Caller(0)` returns an absolute path
even when the process cwd changes, so the YAML file can be found.

## Implementation notes

- Extracted the YAML path resolution into a `openAPIYAMLPath(t)` helper that
  uses `runtime.Caller(0)` (unchanged from the original) — both the default-cwd
  test and the new non-default-cwd test share the resolver.
- Extracted the enum-comparison body into a `runEnumDrift(t, data)` helper —
  both tests share comparison logic, no copy-paste.
- New `TestEventTypeConstants_MatchOpenAPIYAML_NonDefaultCwd` calls
  `t.Chdir(t.TempDir())` (Go 1.24+; module is at 1.26) then re-runs the same
  comparison. If the path-resolver ever drifts to a relative path or
  `os.Getwd`, this test fails loudly.

Verified: `go test ./internal/portal/events/... -count 1` passes.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Path resolver extracted into `openAPIYAMLPath` helper, comparison body into `runEnumDrift` — eliminates copy-paste and centralizes the invariant. `t.Chdir(t.TempDir())` correctly uses the Go 1.24+ helper. Failure mode (path resolver drifting to `os.Getwd`) is the loud-fail target.
