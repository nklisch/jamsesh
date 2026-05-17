---
id: epic-e2e-tests-chaos-runtime-and-clock
kind: story
stage: done
tags: [e2e-test, testing]
parent: epic-e2e-tests-chaos
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Chaos — Runtime pause + clock skew

## Scope

One active chaos scenario + 1 documented-skip:

1. **`automerger_pause`** — Pumba pauses the portal container for
   5 seconds while a sync-ref push is mid-merge. Invariant: when the
   container resumes, the push completes and the auto-merger
   advances `draft`; no `conflict.detected` event spuriously fires.

2. **`clock_skew_token_expiry`** — DEFERRED. libfaketime shifts the
   portal's clock forward by 1 hour mid-session. Requires the
   `portal-test-clock-advance-endpoint` backlog item OR libfaketime
   shim at container level (Dockerfile.e2e change). Documented skip
   with explicit reference.

## Files to create / modify

- `tests/e2e/chaos/runtime_and_clock_test.go` (NEW) — main spec
- `tests/e2e/fixtures/pumba/pumba.go` (NEW) — helper that invokes
  `pumba pause --duration <d> <container-name>` via `os/exec`. The
  Pumba binary is available via the `gaiaadm/pumba` Docker image or
  as a Go binary. Simplest approach: use `docker pause <name>` /
  `docker unpause <name>` directly via `os/exec` (no Pumba dep
  needed for the simple pause case)

## Acceptance criteria

- [ ] `cd tests/e2e && go test ./chaos/ -v -run TestRuntimeAndClock -timeout 180s` runs green
- [ ] `automerger_pause` exercises a real `docker pause` mid-flight;
      asserts the merge completes after resume; asserts no spurious
      `conflict.detected` event
- [ ] `clock_skew_token_expiry` is skipped with `t.Skip` and a
      comment pointing at `portal-test-clock-advance-endpoint`
- [ ] Each active scenario has a paired "before chaos" assertion
- [ ] Each scenario's invariant is stated in plain English

## Notes for the implementer

- **Use `docker pause` directly, not Pumba**: Pumba is a useful
  abstraction but for "pause container by name" the underlying
  command is just `docker pause <name>` / `docker unpause <name>`.
  Skip the Pumba container entirely; just shell out via
  `exec.CommandContext`
- The container name needs to be known to the test. Testcontainers
  generates names; use `c.Name()` or similar to retrieve it
- For the test: push commit X on Alice's ref, immediately `docker
  pause <portal-name>` for 5s, then `docker unpause`. Wait for the
  `merge.succeeded` event on the WS stream
- The "no spurious conflict" assertion requires waiting a short
  window after the pause to confirm no `conflict.detected` event
  arrives. Use a buffered channel + select with a short timeout
- Per user directive: file production bugs to backlog. If
  `docker pause` mid-merge causes the merge to hang or fail
  permanently, that's a real bug to file
- Container logs on failure: capture before unpausing if the test
  is going to fail. The existing
  `e2e-fixtures-capture-container-logs-on-failure` backlog covers
  the fixture-side fix

## Risks

- **`docker pause` followed quickly by `docker unpause` may race**
  with Testcontainers' lifecycle management. The fixture's cleanup
  could try to remove the container while it's paused. Add a
  defensive `defer exec.Command("docker", "unpause", name).Run()`
  to ensure the container isn't left paused
- **The auto-merger uses go-git in-process** — if the portal
  process is paused mid-merge, the merge state is opaque. Resume
  semantics depend on the merge algorithm being idempotent.
  Document expected behavior; file as backlog if surprising

## Implementation notes

**Files changed:**
- `tests/e2e/chaos/runtime_and_clock_test.go` (NEW) — `TestRuntimeAndClock`
  with `automerger_pause` (active) and `clock_skew_token_expiry` (deferred skip)
- `tests/e2e/fixtures/portal/portal.go` — added `(*Portal).ContainerName(ctx)`
  helper that strips the leading `/` from `c.Name(ctx)`

**Design decisions:**
- `randEmail`, `requireDocker`, and `requirePortalImage` already exist in
  `network_and_provider_test.go` (same `chaos_test` package) — reused, not
  duplicated.
- All other scenario-local types and helpers prefixed with `chaos` to avoid
  future collisions when more chaos files are added.
- Defensive `t.Cleanup(docker unpause)` registered before the pause so the
  container is never left frozen if the test fails mid-scenario.
- `docker pause` skip: if `ContainerName` returns empty or `docker pause`
  itself errors (e.g. non-Linux CI without cgroups freeze support), the test
  skips gracefully rather than failing.

**Verification:** `go test ./chaos/ -run TestRuntimeAndClock -v -timeout 180s`
passes with `automerger_pause` PASS and `clock_skew_token_expiry` SKIP.
Resume semantics proved healthy on this machine — go-git resumes cleanly
after a 5-second freeze, no spurious `conflict.detected` event observed.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- Cleanly validated a real resilience claim — auto-merger's in-process go-git merge is idempotent across process pauses, no spurious conflict events. Defensive `defer docker unpause` registered before the pause prevents leaving the container frozen on test failure.

**Notes**: The `(*Portal).ContainerName(ctx)` helper added to the portal fixture is a clean small extension; future chaos tests targeting other containers (postgres, mailhog) can reuse the docker-pause pattern. clock_skew_token_expiry deferred-skip references the backlog item; appropriate handling.
