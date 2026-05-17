---
id: epic-e2e-tests-failure-mode-config-and-deps
kind: story
stage: done
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
  - [x] Portal with empty `JAMSESH_EMAIL_FROM` → exits non-zero at
        startup with `senders.New: email.from required`
  - [x] Portal with `JAMSESH_DB_DRIVER=postgres` but no valid `JAMSESH_DB_DSN`
        → exits non-zero at startup (invalid DSN triggers db.Open failure)
  - [x] Portal with invalid `JAMSESH_TLS_MODE=garbage` → exits non-zero
        at startup with the config validation error

- **Unavailable dependency**:
  - [x] Postgres disrupted mid-session via Toxiproxy reset_peer toxic →
        portal returns 4xx/5xx on subsequent REST calls; recovers after
        toxic removed
  - [x] MailHog stopped after portal start → magic-link request
        returns 500 (send error surfaced as internal server error)
  - [x] WireMock returns 503 on `/login/oauth/access_token` → OAuth
        callback flow returns 500 (Exchange error surfaced as internal
        server error)

## Implementation notes

### Subtests written

1. `missing_config/missing_email_from` — portal exits non-zero; logs mention email.from
2. `missing_config/invalid_tls_mode` — portal exits non-zero; logs mention config/tls
3. `missing_config/postgres_driver_invalid_dsn` — portal exits non-zero; logs mention database/DSN
4. `unavailable_dep/smtp_unavailable` — MailHog stopped mid-test; portal returns 500 on magic-link request
5. `unavailable_dep/db_unavailable_via_toxiproxy` — Toxiproxy reset_peer toxic disrupts DB; portal returns non-200; recovers after toxic removed
6. `unavailable_dep/oauth_provider_5xx` — WireMock returns 503 on token exchange; portal returns 500 on OAuth callback

### Deferred subtests

- "git binary missing from PATH" — skipped; too invasive to test from outside the container without modifying the image.

### Design discoveries

- The portal's error codes for SMTP/DB/OAuth failures are all plain-text 500 (from the oapi-codegen strict handler's `ResponseErrorHandlerFunc` path), not JSON error envelopes with machine-readable codes. The spec documents `dep.*` codes but these are not yet implemented in the production code. Tests assert on HTTP status code 500 only.
- The shared postgres fixture cannot be paused (would disrupt other tests). Toxiproxy's reset_peer toxic provides fast failure without pausing the container.
- `mh.Stop(ctx)` causes ECONNREFUSED on the SMTP port, making the portal's Send call fail quickly (not hang).

### Fixture changes

- `tests/e2e/fixtures/mailhog/mailhog.go` — added `Stop(ctx) error` method
- `tests/e2e/fixtures/toxiproxy/toxiproxy.go` — added `ContainerIP string` field populated at Start time

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- 685 lines in one file is approaching the upper readable limit. Could split into `missing_config_test.go` + `unavailable_deps_test.go` for the two distinct categories. Not blocking; the t.Run structure already groups them clearly.
- "git binary missing" subtest is skipped with documented reason — acceptable.

**Notes**: The production-side discovery (dep.* codes not implemented — failures surface as plain-text 500) was correctly filed as `portal-dep-failure-error-codes` backlog item rather than blocked-on. Tests pin to HTTP status 500 only, with comments documenting the contract gap. The Toxiproxy reset_peer pattern for DB disruption is a good choice — surfaces a clear "connection refused" failure mode without pausing the container (which would affect other tests sharing the same Postgres instance).
