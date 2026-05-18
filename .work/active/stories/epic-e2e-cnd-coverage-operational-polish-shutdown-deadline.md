---
id: epic-e2e-cnd-coverage-operational-polish-shutdown-deadline
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-operational-polish
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Graceful-shutdown deadline coverage — failure

## Scope

One test file with two subtests covering the `JAMSESH_SHUTDOWN_GRACE_S`
env var (verified at `internal/portal/config/config.go:42,99,494`):

- `request_finishes_within_deadline` — request that completes inside
  the deadline window finishes successfully even after SIGTERM.
- `request_exceeds_deadline` — request that exceeds the deadline is
  terminated; portal exits at the deadline (not later).

Long-running request source: WireMock-stubbed OAuth callback with an
injected `fixedDelayMilliseconds` (existing pattern at
`tests/e2e/chaos/testdata/github_delay_30s.json`).

## Files

- `tests/e2e/failure/graceful_shutdown_deadline_test.go`
- `tests/e2e/failure/testdata/oauth_delay_2s.json` (WireMock mapping)
- `tests/e2e/failure/testdata/oauth_delay_10s.json` (WireMock mapping)
- `tests/e2e/fixtures/portal/portal.go` (extension — add
  `SendSignal(ctx, sig syscall.Signal) error` to `*Portal` if absent)

## Acceptance criteria

- [ ] `request_finishes_within_deadline`: deadline=10s, OAuth call=2s,
      SIGTERM mid-flight → request returns a response within ~3s
      (success path), no shutdown abort error
- [ ] `request_exceeds_deadline`: deadline=2s, OAuth call=10s, SIGTERM
      mid-flight → request is terminated near the 2s mark (HTTP error,
      connection close, or 503); total elapsed < 4s; portal container
      transitions to `exited` state shortly after
- [ ] `Portal.SendSignal` extension lands (if not already present);
      uses `container.Exec` or docker stop+signal under the hood
- [ ] WireMock mappings checked in under `tests/e2e/failure/testdata/`
- [ ] If the test surfaces the `graceful-shutdown-shutdownstart-race`
      backlog race, the test calls it out via comment + `t.Skip(...)`
      with the backlog id rather than fixing inline or papering over

## Test integrity (from parent epic)

- A test that "passes" because the portal kills the connection
  immediately on SIGTERM (no grace period) is wrong — it fails the
  `request_finishes_within_deadline` invariant. The assertion must
  verify the in-flight request actually completes.
- For `request_exceeds_deadline`, asserting only "request didn't
  complete" misses the deadline check — must also assert total elapsed
  is bounded by the deadline + margin. Otherwise the test passes even
  if the portal hangs past deadline.
- The open backlog story
  `.work/backlog/graceful-shutdown-shutdownstart-race.md` documents a
  race that may surface here. If it does, the right move is to park
  this story's blocker on top of that backlog item and `t.Skip` with
  reference. Do not paper over with retries.

## References

- Parent feature body, Unit 5 — full scaffold + WireMock approach
- `internal/portal/config/config.go:42,99,494` — `JAMSESH_SHUTDOWN_GRACE_S`
- `tests/e2e/chaos/testdata/github_delay_30s.json` — WireMock delay
  mapping pattern to mirror
- `tests/e2e/chaos/network_and_provider_test.go > testOAuthProviderTimeout`
  — existing pattern for OAuth-with-delay testing
- `tests/e2e/fixtures/portal/portal.go:97-103` — existing
  `ContainerName` helper as model for `SendSignal` extension
- `.work/backlog/graceful-shutdown-shutdownstart-race.md` — open race
  this test may surface
