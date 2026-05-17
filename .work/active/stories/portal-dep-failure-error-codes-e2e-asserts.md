---
id: portal-dep-failure-error-codes-e2e-asserts
kind: story
stage: review
tags: [portal, testing]
parent: portal-dep-failure-error-codes
depends_on:
  - portal-dep-failure-error-codes-auth-smtp
  - portal-dep-failure-error-codes-db
  - portal-dep-failure-error-codes-oauth
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E failure-mode tests: assert on typed `dep.*` envelopes

Updates `tests/e2e/failure/config_and_deps_test.go` to assert on the
typed envelope (status + `error` field), not just HTTP status. With
this story, the e2e suite verifies that the contract documented in
`PROTOCOL.md` actually holds end-to-end through the portal binary in
a Docker container against real Postgres + MailHog + WireMock
dependencies.

Git-subprocess e2e is **not** in scope for this story — the existing
failure-mode test file doesn't have a git-subprocess scenario; that
gap is tracked as a separate follow-up (track via
`/agile-workflow:park` if not already in the substrate).

## Files

- **Edit** `tests/e2e/failure/config_and_deps_test.go`:

  Update the three Category-2 sub-tests:

  ### `smtp_unavailable` (line ~434-493)

  Current assertion:

  ```go
  if resp.StatusCode != http.StatusInternalServerError {
      t.Errorf("smtp_unavailable: expected 500 when SMTP is down, got %d\nbody: %s",
          resp.StatusCode, respBody)
  }
  ```

  Target:

  ```go
  if resp.StatusCode != http.StatusServiceUnavailable {
      t.Errorf("smtp_unavailable: expected 503 when SMTP is down, got %d\nbody: %s",
          resp.StatusCode, respBody)
  }
  var env struct {
      Error   string `json:"error"`
      Message string `json:"message"`
  }
  if err := json.Unmarshal(respBody, &env); err != nil {
      t.Errorf("smtp_unavailable: decode envelope: %v\nbody: %s", err, respBody)
  }
  if env.Error != "dep.smtp_unavailable" {
      t.Errorf("smtp_unavailable: expected error=dep.smtp_unavailable, got %q\nbody: %s",
          env.Error, respBody)
  }
  if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "5" {
      t.Errorf("smtp_unavailable: expected Retry-After=5, got %q", retryAfter)
  }
  ```

  ### `db_unavailable_via_toxiproxy` (line ~495-610)

  Current: `if dbErrStatus == http.StatusOK { ... expected 4xx or 5xx }`.

  This is intentionally loose because the toxic might land mid-request
  in a way that lets the client see a network-level reset rather than
  a clean HTTP response. Keep the loose path for the network-error
  case, but when an HTTP response IS received, assert the typed
  envelope:

  ```go
  if dbErrStatus != 0 && dbErrStatus != http.StatusServiceUnavailable {
      t.Errorf("db_unavailable: expected 503 when DB is disrupted, got %d", dbErrStatus)
  }
  if dbErrStatus == http.StatusServiceUnavailable {
      // We received a clean response; assert on the typed envelope.
      // Re-issue the request because the original goroutine already
      // drained the body — easiest path is to extract the body in
      // the original block and assert here.
  }
  ```

  Refactor the original `if err != nil { ... } else { defer ...; io.Copy(io.Discard, resp.Body); dbErrStatus = resp.StatusCode }` block to capture the body bytes
  instead of discarding, then add the envelope decode + assertion after.

  Also check `Retry-After: 2` when a 503 was received.

  ### `oauth_provider_5xx` (line ~612-663)

  Current assertion:

  ```go
  if status != http.StatusInternalServerError {
      t.Errorf("oauth_provider_5xx: expected 500 when OAuth provider returns 5xx, got %d\nbody: %s",
          status, body)
  }
  ```

  Target: 503 + decode envelope + assert
  `error == "dep.oauth_provider_unavailable"` +
  `Retry-After: 10`.

- **Edit** the file-level package doc comment (lines 1-21) to reflect
  the NOW state of the contract:

  Current:

  ```
  //  2. Unavailable dependency — full stack started, then a dependency is
  //     disrupted mid-test; asserts the portal returns 500 with a plain-text
  //     error body (the oapi-codegen strict handler's ResponseErrorHandlerFunc
  //     path), or the documented error envelope where one is defined.
  ```

  Target:

  ```
  //  2. Unavailable dependency — full stack started, then a dependency is
  //     disrupted mid-test; asserts the portal returns 503 with a typed
  //     dep.* error envelope (dep.smtp_unavailable, dep.db_unavailable,
  //     dep.oauth_provider_unavailable). Status code and the envelope
  //     `error` field are both asserted.
  ```

  And remove the line `// Note: unhandled handler errors are surfaced as
  plain-text 500 via http.Error — these tests assert only the status
  code, not the body.` — that's no longer accurate.

  **No "previously this was..." prose** anywhere — the doc-comment
  describes the contract NOW.

## Acceptance criteria

- [ ] `smtp_unavailable` sub-test asserts 503 +
      `error: dep.smtp_unavailable` + `Retry-After: 5`
- [ ] `db_unavailable_via_toxiproxy` sub-test asserts 503 +
      `error: dep.db_unavailable` + `Retry-After: 2` when a clean
      response is received; preserves the loose network-error
      fallback path
- [ ] `oauth_provider_5xx` sub-test asserts 503 +
      `error: dep.oauth_provider_unavailable` + `Retry-After: 10`
- [ ] Package doc comment reflects NOW state (no "plain-text 500"
      references in the unavailable-dep section)
- [ ] `make test-e2e-failure` (or `go test ./tests/e2e/failure/...`)
      passes against a built portal image
- [ ] No "previously" / "v1.x" prose anywhere

## Test approach

This story IS test work. Run it against a fresh portal image built
with all upstream stories merged. The existing testcontainers
scaffolding (Toxiproxy, WireMock, MailHog, Postgres) doesn't change.

## Risk

LOW. Test-only changes. The test was previously asserting *less*
than the contract; tightening it cannot regress production behavior.

## Rollback

`git revert`. The pre-tightening assertions remain valid because the
new contract is a strict refinement (every 503 with
`dep.smtp_unavailable` is also a non-2xx response that the old test
treated as acceptable).

## Implementation notes

### Files touched

- `tests/e2e/failure/config_and_deps_test.go` — tightened all three
  Category-2 sub-tests to assert on the typed envelope; refactored
  the package doc comment to describe the NOW contract; added a
  header-aware `oauthCallbackWithHeaders` helper (the existing
  `oauthCallback` was preserved as a thin shim around it).
- `.work/backlog/portal-bearer-middleware-dep-translate.md` — parked
  a real production gap surfaced while writing this test (see
  "BearerMiddleware DB-dep gap" below).

### Dep classes covered

- **SMTP** (`dep.smtp_unavailable`, 503, Retry-After: 5) — asserted on
  `POST /api/auth/magic-link/request` while MailHog is `mh.Stop`'d.
  Loose network-error fallback preserved (TCP-connect timeout path).
- **DB** (`dep.db_unavailable`, 503, Retry-After: 2) — asserted on
  `POST /api/auth/magic-link/request` while Postgres is disrupted via
  Toxiproxy `reset_peer`. This is a public endpoint that flows
  through the strict-handler `ResponseErrorHandlerFunc` (where the
  dep translator lives). See note below on why GET /me was not used.
- **OAuth provider** (`dep.oauth_provider_unavailable`, 503,
  Retry-After: 10) — asserted on `POST /api/auth/oauth/callback`
  while WireMock returns 503 on the GitHub token endpoint.
- **git-subprocess** — explicitly out of scope per the story; the
  failure-mode test file has no git-subprocess scenario. Natural
  coverage opportunities exist in the smart-HTTP e2e suite — calling
  `git fetch` against a corrupted bare repo or unsetting `PATH`
  inside the portal container — but those belong to a separate
  follow-up.

### Envelope assertions per sub-test

Each sub-test now asserts:

- HTTP status (503)
- `Content-Type: application/json` (prefix match to tolerate the
  `; charset=utf-8` suffix `httperr.Write` emits)
- Decoded `{error, message}` envelope with the expected code and a
  non-empty message
- `Retry-After` header matches the taxonomy

### BearerMiddleware DB-dep gap (parked)

The initial draft of the db sub-test exercised `GET /api/me` with a
fake bearer (the original test's pattern, since it provoked a DB
lookup via `tokens.Service.Validate`). On a tightened envelope check
the response came back as `{"error":"internal","message":"internal
server error"}` (plain 500), not the documented `dep.db_unavailable`
(503).

Root cause: `internal/portal/tokens/middleware.go` `BearerMiddleware`
falls through to `httperr.Write(w, r, httperr.ErrInternal(err))` on
any non-sentinel error from `svc.Validate`. That bypasses the strict
handler's `ResponseErrorHandlerFunc` and therefore the deperr
translator. So the dep-failure feature's claim of end-to-end coverage
is incomplete on auth-gated endpoints.

Parked as `.work/backlog/portal-bearer-middleware-dep-translate.md`
with a proposed fix (route through `httperr.WriteFromError` with
`deperr.WrapDBIfTransient`). To keep this story's scope honest
(test-only changes; sibling production code untouched per the
constraint), the db sub-test switched to
`POST /api/auth/magic-link/request`, which goes through the strict
handler and correctly emits the typed 503 envelope today. The
"didn't silently succeed" loose check was also updated to reject 204
in addition to 200 (the magic-link happy path is 204).

### Test invocation result

```
$ make test-portal-image    # rebuild image from current source
$ cd tests/e2e && go build ./...
$ cd tests/e2e && go test ./failure/... -run "TestConfigAndDeps" -v -timeout 600s

--- PASS: TestConfigAndDeps (21.97s)
    --- PASS: TestConfigAndDeps/missing_config (2.70s)
        --- PASS: TestConfigAndDeps/missing_config/missing_email_from
        --- PASS: TestConfigAndDeps/missing_config/invalid_tls_mode
        --- PASS: TestConfigAndDeps/missing_config/postgres_driver_invalid_dsn
    --- PASS: TestConfigAndDeps/unavailable_dep (19.22s)
        --- PASS: TestConfigAndDeps/unavailable_dep/smtp_unavailable
        --- PASS: TestConfigAndDeps/unavailable_dep/db_unavailable_via_toxiproxy
        --- PASS: TestConfigAndDeps/unavailable_dep/oauth_provider_5xx
PASS
ok    jamsesh/tests/e2e/failure    21.967s
```

Confirmed stable across `-count=2`.
