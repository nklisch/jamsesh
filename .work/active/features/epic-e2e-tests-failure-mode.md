---
id: epic-e2e-tests-failure-mode
kind: feature
stage: implementing
tags: [e2e-test, testing]
parent: epic-e2e-tests
depends_on: [epic-e2e-tests-infrastructure]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Tests — Failure Mode

## Brief

Failure-mode coverage across the six standard failure categories from the
e2e-test-design taxonomy reference, scoped to jamsesh's actual failure
boundaries. Tests assert on the user-visible response — error code, error
body shape, log line, exit code — never on internal call traces.

The categories and the jamsesh-specific surface for each:

| Category | Jamsesh surface |
|---|---|
| Invalid input | OpenAPI schema violations on REST; malformed MCP tool args; malformed commit trailers; ref names outside the user's namespace |
| Missing config | Portal boots with no SMTP / no SendGrid key; portal boots with no DB DSN; `jamsesh` binary with no `CLAUDE_PLUGIN_DATA` |
| Unavailable dependency | DB down (Postgres container paused); SMTP provider returns 5xx; OAuth provider returns 5xx; git binary missing from PATH |
| Boundary values | Empty session goal; max-length comment body; max-line-range comment anchor; max-ref-name length; max commit message size accepted by pre-receive |
| Permission failures | Bearer token expired; bearer token for a different org; magic-link token reused; OAuth state reused; pushing to another user's namespace; reading another org's session |
| Interrupted operations | Push interrupted mid-pack; finalize lock acquired then process killed; magic-link request submitted but exchange skipped past TTL; WS connection drop mid-event |

## Scope

One Go spec file per category (6 specs) at `tests/e2e/failure/`. Each
file contains 5-10 subtests with `t.Run` covering the surface area
above for that category. Plus 1 Playwright spec covering visible error
states in the SPA (expired-token banner, malformed magic-link
redirect).

## Out of scope

- Chaos scenarios (network jitter, container kill mid-flight) — chaos
  feature
- Recovery semantics under fault injection — chaos feature
- Validator fuzzing — fuzzing feature (overlaps slightly with the
  invalid-input category here; this feature covers known-bad
  human-readable inputs; fuzzing covers generated/random inputs)

## Foundation references

- `docs/SECURITY.md` — permission boundaries
- `docs/PROTOCOL.md > HTTP error contract` — error response shapes
  every spec asserts against
- `docs/openapi.yaml` — schema constraints that drive the invalid-input
  spec
- `.work/active/epics/epic-e2e-tests.md` — parent mock policy

## Acceptance criteria

- [ ] 6 Go specs at `tests/e2e/failure/`, each green
- [ ] 1 Playwright spec at `tests/e2e/playwright/error-states.spec.ts`,
      green
- [ ] Every subtest asserts on a user-visible outcome (status code +
      error body shape, exit code, log line) — no `mock.WasCalledWith`
      patterns
- [ ] Every subtest's invariant is stated in plain English in a
      comment on the `t.Run` block
- [ ] Permission-failure subtests are explicitly cross-org / cross-user
      where applicable (matches the org_id invariant)

## Design decisions

Locked under autopilot (2026-05-17):

- **4 stories instead of 6**. The brief listed 6 categories with one
  Go spec per category. Consolidated into 3 Go specs +
  1 Playwright spec by grouping closely-related categories
  (invalid-input + boundary-values + permissions →
  `rest-validation`; missing-config + unavailable-dep →
  `config-and-deps`; interrupted-ops stays standalone). Subtest
  count per Go spec stays in the 8-15 range — coverage preserved,
  fewer files to maintain.

- **Auth-helpers extraction bundled into `rest-validation`** (the
  first failure-mode story). The helpers
  (`signInViaMagicLink`, `createOrg`, `inviteToOrg`, `acceptInvite`,
  `requireOrgMembership`, `postJSON*`) currently live in
  `tests/e2e/golden/onboarding_test.go > package golden_test` and
  can't be reused from `tests/e2e/failure/`. The refactor moves them
  to `tests/e2e/fixtures/authflow/` (exported package) and migrates
  golden's spec to import from there. Subsequent failure-mode
  stories use the extracted helpers without duplication.

- **Postgres everywhere** for failure-mode, matching golden-path's
  choice.

- **No new shared infrastructure beyond `authflow`**. Reuses the
  fixtures landed in infrastructure (postgres, mailhog, wiremock,
  toxiproxy, portal, binary). The `wsclient` fixture from
  `session-lifecycle` is needed by one subtest in `interrupted-ops` —
  if `session-lifecycle` hasn't landed when `interrupted-ops` is
  implemented, the WS subtest is deferred to a follow-on.

- **Granular error-code assertions**. Subtests assert on the `code`
  field of the error envelope (e.g., `auth.invalid_token`,
  `validation.required_field`) — NOT on the human-readable
  `message`. The code is a contract; the message is not. See
  `docs/PROTOCOL.md > Error response`.

- **Promote `e2e-portal-fixture-oauth-base-url-default` from backlog
  when implementing `config-and-deps`**. The "OAuth provider 5xx"
  subtest exercises a WireMock-served OAuth endpoint with injected
  5xx; that requires the portal fixture to default
  `JAMSESH_OAUTH_GITHUB_BASE_URL` to a safe sentinel value (or to
  WireMock). The backlog item should be promoted to active and
  satisfied before `config-and-deps` runs the OAuth subtest.

## Story decomposition

Four stories:

1. `epic-e2e-tests-failure-mode-rest-validation` — auth-helpers
   extraction + REST validation spec (invalid input + boundary
   values + permissions). No deps beyond infrastructure (already
   done).

2. `epic-e2e-tests-failure-mode-config-and-deps` — missing config +
   unavailable dependency spec. Depends on
   `rest-validation` (uses the extracted authflow fixture).

3. `epic-e2e-tests-failure-mode-interrupted-ops` — interrupted
   operations spec (pushes, locks, magic-link TTL, WS drop).
   Depends on `rest-validation`. May need a follow-on for the WS
   subtest if `session-lifecycle` hasn't landed.

4. `epic-e2e-tests-failure-mode-spa-error-states` — Playwright
   spec covering user-visible error states. Independent of the Go
   stories.

## Implementation Order

Wave 1 (parallel — no overlap):
- `rest-validation` (the refactor + 12-15 subtests)
- `spa-error-states` (Playwright; no Go-fixture dep)

Wave 2 (parallel — both depend on rest-validation):
- `config-and-deps`
- `interrupted-ops`

## Pre-mortem

- **Auth-helpers refactor risk**: moving helpers from
  `golden_test` to `authflow` package while keeping the existing
  golden spec green requires careful migration. Implementor should
  run `go test ./golden/ -v` after the migration before adding the
  new failure-mode subtests.
- **The "git missing" failure-mode subtest** is hard to test
  from outside the portal container — the binary's PATH is set at
  container build time. Skip or file a follow-on.
- **Clock-skew tests in `interrupted-ops`** require either
  libfaketime (heavy) or a test-only `/test/clock-advance` endpoint
  (portal change). The story body recommends skipping under
  `-short` for now and filing a follow-on for the clock endpoint.
- **Error-code assertion stability**: the assertions assume
  `docs/PROTOCOL.md > Error response` codes are stable. If the
  portal ever renames a code (e.g., `auth.invalid_token` →
  `auth.token_invalid`), all permission subtests fail at once.
  That's a feature — it's exactly the kind of breaking change
  e2e is meant to catch.
- **OAuth-default safety**: the OAuth subtest in `config-and-deps`
  depends on the portal fixture defaulting `OAuthBaseURL` to a
  safe value. If that doesn't land first, the test could
  inadvertently call real github.com. Mitigation noted in the
  story body.

Risks documented; no spike unit needed.
