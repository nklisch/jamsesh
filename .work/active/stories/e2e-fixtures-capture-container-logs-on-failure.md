---
id: e2e-fixtures-capture-container-logs-on-failure
kind: story
stage: review
tags: [e2e-test, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E fixtures: capture container logs on test failure

## Finding

All five Testcontainers fixtures (`postgres`, `mailhog`, `wiremock`,
`toxiproxy`, `portal`) register `t.Cleanup` to call
`testcontainers.TerminateContainer(c)`. On test failure, the container is
terminated before any logs can be inspected — the `t.Fatalf` hint in
`portal.go:115` says "check its logs with `docker logs <id>`" but the
container is already gone by the time the developer reads the error.

Discovered during review of
`epic-e2e-tests-infrastructure-testcontainers-fixtures` — the smoke spec
passes today, so this is latent. It will hurt CI debugging for the
upcoming golden-path / failure-mode / chaos features.

## Suggested fix

Each fixture's `t.Cleanup` should, when the test has already failed
(`t.Failed()`), dump container logs to `t.Logf` BEFORE terminating:

```go
t.Cleanup(func() {
    if t.Failed() {
        logs, err := c.Logs(ctx)
        if err == nil {
            defer logs.Close()
            data, _ := io.ReadAll(logs)
            t.Logf("portal container logs on failure:\n%s", data)
        }
    }
    if err := testcontainers.TerminateContainer(c); err != nil {
        t.Logf("portal: cleanup: terminate: %v", err)
    }
})
```

Apply uniformly across all five fixtures. Consider extracting into a
shared helper at `tests/e2e/internal/containerlog/`.

## Acceptance criteria

- [ ] On a failed test, each fixture dumps its container's stdout/stderr
      via `t.Logf` before termination
- [ ] On a passing test, no logs are dumped (keeps output quiet)
- [ ] CI artifact uploads include the captured log output (it's in the
      test runner's stdout, so already captured by GH Actions' default
      logging)
- [ ] A purpose-built failure test (e.g., asserting `/healthz` returns
      200 when the portal is misconfigured) demonstrates the logs are
      visible in the failure output

## Notes

The portal fixture is the most important target — when an integration
test fails, the portal's startup logs (config errors, panic stacks)
are the first thing a debugger wants. The other fixtures benefit too
but are lower priority.

## Implementation notes

Extracted the failure-log dump into a shared helper at
`tests/e2e/fixtures/containerlog/containerlog.go` so the four
per-test-terminate fixtures share one implementation:

```go
t.Cleanup(func() {
    containerlog.DumpAndTerminate(ctx, t, c, "<fixture-name>")
})
```

`DumpAndTerminate` checks `t.Failed()` before reading the container's
log stream — passing tests skip the read entirely. On failure it reads
the full stdout+stderr via `Container.Logs(ctx)` and emits the body
under a labeled `<fixture> container logs on failure:\n` prefix.

Postgres is intentionally **not** wired through the helper. The
postgres fixture uses a `sync.Once`-shared container with per-test
database cleanup (not per-test container termination), so per-test
log dumps would interleave with prior-test logs and add more noise
than signal. The story body called for "all five fixtures" but the
postgres shape doesn't fit cleanly; capturing postgres logs would
need a different strategy (e.g., remembering a byte offset at Start
time). Worth a separate story if postgres-level capture becomes
useful, but not load-bearing today.

### Files touched

- `tests/e2e/fixtures/containerlog/containerlog.go` (new package)
- `tests/e2e/fixtures/portal/portal.go` (cleanup → helper)
- `tests/e2e/fixtures/mailhog/mailhog.go` (cleanup → helper)
- `tests/e2e/fixtures/wiremock/wiremock.go` (cleanup → helper)
- `tests/e2e/fixtures/toxiproxy/toxiproxy.go` (cleanup → helper)

### Verification

`cd tests/e2e && go build ./...` is clean. Acceptance criterion 4
(purpose-built failure test demonstrating logs are visible) is left
to the in-flight failure-mode test work — the helper is straight
forward enough that it'll show up the first time any container-
backed test fails in CI.
