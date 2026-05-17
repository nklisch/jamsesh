---
id: epic-e2e-tests-failure-mode-config-and-deps
kind: story
stage: implementing
tags: [e2e-test, testing]
parent: epic-e2e-tests-failure-mode
depends_on: [epic-e2e-tests-failure-mode-rest-validation]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Failure — Missing config + unavailable dependency

## Scope

One Go spec `tests/e2e/failure/config_and_deps_test.go` covering two
failure-mode categories: startup failures from missing required
configuration, and runtime failures when external dependencies are
unavailable.

### Categories covered

- **Missing config** — portal boots with no SMTP provider configured
  (and a magic-link request is issued); portal boots with no DB DSN;
  portal boots without `JAMSESH_EMAIL_FROM`
- **Unavailable dependency** — DB down mid-session (Postgres container
  paused); SMTP returns 5xx (MailHog killed mid-test);
  OAuth provider returns 5xx (WireMock injects 503); git binary missing
  from PATH

## Files to create / modify

- `tests/e2e/failure/config_and_deps_test.go` (NEW) — main spec with
  ~8-10 subtests

## Acceptance criteria

- [ ] `cd tests/e2e && go test ./failure/ -run TestConfigAndDeps -v`
      runs green
- [ ] Subtests for missing-config exercise the portal's STARTUP path
      (asserts portal container exits non-zero with the expected error
      class in its logs, OR — if startup is too coupled to other env
      vars — runs the binary directly with `go run` and asserts stderr)
- [ ] Subtests for unavailable-dependency exercise the RUNTIME path
      (start the stack, kill a dep mid-test, assert the portal
      responds with the documented 503 / `dep.*` error codes)
- [ ] Each subtest's invariant is stated in plain English
- [ ] No assertion on internal call traces — only HTTP responses,
      exit codes, and log lines

## Notes for the implementer

- For "missing config" tests: the portal fixture in
  `tests/e2e/fixtures/portal/` always sets `JAMSESH_EMAIL_FROM` etc.
  Bypass the fixture for these tests — directly construct a
  `testcontainers.ContainerRequest` with the minimal env you want.
  Confirm the container exits non-zero and the log contains the
  expected error string.
- For "DB down" tests: use the existing Postgres fixture, then issue
  `docker pause <postgres-container-id>` mid-test. Use Toxiproxy
  instead if available — gives finer control (timeout vs. complete
  outage).
- For "SMTP unavailable": stop the MailHog container after the portal
  is connected, then issue a magic-link request and assert the portal
  surfaces a 503 with the documented error code.
- For "OAuth provider 5xx": use WireMock's stateful scenarios to
  return 503 on the OAuth endpoints, then exercise the OAuth start
  flow.
- For "git binary missing": this is hard to test from outside the
  portal container. Skip if too invasive — file a follow-on if
  coverage is required.

## Notes on the OAuth provider availability test

This subtest depends on the OAuth fixture defaulting to something safe
(see backlog item `e2e-portal-fixture-oauth-base-url-default`). If
that fix isn't landed when this story is implemented, the implementor
should either:

- Promote that backlog item to a sibling story and depend on it
- Or implement the safe-default behavior inline as part of this story
  (it's small — change the portal fixture's `OAuthBaseURL` default to
  a sentinel unroutable value)

## Subtest checklist

- **Missing config**:
  - [ ] Portal with empty `JAMSESH_EMAIL_FROM` → exits non-zero at
        startup with `senders.New: email.from required`
  - [ ] Portal with `JAMSESH_DB_DRIVER=postgres` but no `JAMSESH_DB_DSN`
        → exits non-zero at startup
  - [ ] Portal with invalid `JAMSESH_TLS_MODE=garbage` → exits non-zero
        at startup with the config validation error

- **Unavailable dependency**:
  - [ ] Postgres paused mid-session → portal returns 503 on subsequent
        REST calls; recovers after unpause
  - [ ] MailHog stopped after portal start → magic-link request
        returns 502 / 503 with the documented `dep.smtp_unavailable`
        (or similar) error code
  - [ ] WireMock returns 503 on `/login/oauth/access_token` → OAuth
        callback flow returns 502 / 503 with the documented error
        code
