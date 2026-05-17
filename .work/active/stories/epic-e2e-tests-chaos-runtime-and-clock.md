---
id: epic-e2e-tests-chaos-runtime-and-clock
kind: story
stage: implementing
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
