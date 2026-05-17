---
id: epic-e2e-tests-golden-path-onboarding-auth
kind: story
stage: done
tags: [e2e-test, testing]
parent: epic-e2e-tests-golden-path
depends_on: [epic-e2e-tests-golden-path-ccdriver-env-fix]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Golden — Onboarding + auth

## Scope

Two specs that together prove a new user can sign in via magic link,
land on the sessions list, accept an org invite, and be a member of an
org session.

- `tests/e2e/golden/onboarding_test.go` (Go) — full magic-link flow
  through the REST API + MailHog
- `tests/e2e/playwright/login.spec.ts` (Playwright) — UI rendering of
  the magic-link login screen and form submission

## Go spec invariant

After requesting a magic link via `POST /auth/magic-link/request`,
fetching the link from MailHog's HTTP API, exchanging it via
`POST /auth/magic-link/exchange`, accepting an org invite, and listing
org sessions — the user reaches an authenticated state with a session
they're a member of.

## Files to create / modify

- `tests/e2e/golden/onboarding_test.go` — the Go spec
- `tests/e2e/playwright/login.spec.ts` — extend the existing smoke spec
  OR create a new spec that fills in the email, submits, and verifies
  the "check your email" confirmation state renders
- `tests/e2e/fixtures/mailhog/messages.go` (NEW) — small helper
  exposing `(*MailHog).LatestMessageTo(email) Message` that polls
  `/api/v2/messages` and returns the most recent message addressed to
  the given recipient. Includes a `Body` field that returns the parsed
  text body — needed to extract the magic-link URL.
- `tests/e2e/fixtures/binary/jamsesh.go` (NEW; shared across golden
  stories) — `func Build(ctx, t) string` returning the absolute path to
  a freshly-built `jamsesh` binary. Uses `sync.Once` to build once per
  test binary invocation. Output dir under `t.TempDir()` at suite level
  (use `testing.Main` or a `TestMain` to manage lifetime).

## Acceptance criteria

- [ ] Go spec is green when run via `cd tests/e2e && go test ./golden/
      -run TestOnboardingMagicLink`
- [ ] Spec brings up the Postgres + MailHog + WireMock + portal stack
      via the existing fixtures
- [ ] Spec creates an org via `POST /orgs`, sends an invite, has a
      second user accept it via `POST /orgs/{id}/invites/{inviteID}/accept`
- [ ] Spec verifies the second user can list the org via `GET /me` or
      a similar endpoint
- [ ] Playwright spec verifies the login form accepts an email and
      transitions to a "check your email" confirmation state (or the
      sessions-list page if auth completed in one flow)
- [ ] No fixture talks to real github.com or real SMTP servers
- [ ] Test invariant is stated in plain English at the top of each
      spec file

## Notes for the implementer

- The magic-link flow is described in `internal/portal/auth/magic_link.go`.
  The HTTP shape is in `docs/openapi.yaml` (auth section).
- MailHog's HTTP API: `GET /api/v2/messages` returns recent messages.
  Filter by recipient via `to` field. Parse the text body to find the
  magic-link URL.
- The magic-link URL format is documented in
  `internal/portal/auth/magic_link.go`. Use that to derive the regex
  / parser.
- For the Playwright spec, point `PORTAL_URL` at the Go-spawned portal
  container's URL.

## Implementation hints

- Build the binary once via the new `binary.Build()` fixture — avoid
  rebuilding per test
- Each spec gets its own fresh portal container (no shared state)
- Use the existing `mailhog.Start(ctx, t)` and exercise the new
  `LatestMessageTo` helper inside the spec

## Implementation notes

### Files created

- `tests/e2e/fixtures/binary/jamsesh.go` — `Build(t) string` using `sync.Once`
  to compile `./cmd/jamsesh` once per test binary invocation; walks upward from
  cwd to find the repo root via `go.mod` module line.
- `tests/e2e/fixtures/mailhog/messages.go` — adds `(*MailHog).LatestMessageTo`
  and private `fetchLatestTo` to the existing `mailhog` package. Polls
  `GET /api/v2/messages`, filters by `Mailbox@Domain` case-insensitively, and
  returns the first (newest) match. Includes `Message` struct with `Body` field.
- `tests/e2e/golden/onboarding_test.go` — seven-step golden-path test:
  Alice magic-link signin → create org → invite Bob → capture Bob's invite
  token → Bob magic-link signin → Bob accept invite → verify membership via
  `GET /me`.
- `tests/e2e/playwright/login.spec.ts` — three Playwright tests: form
  transitions to "Check your inbox" after submission, confirmation state
  displays the submitted email, and "Try a different email" button returns to
  the form.

### Design discovery: invite token ordering

MailHog's `LatestMessageTo` returns the newest message addressed to a
recipient. In the golden test, both the org-invite email and Bob's magic-link
email land in Bob's inbox. To avoid the magic-link email shadowing the invite
token, the invite token is captured from MailHog immediately after Alice sends
the invite (before Bob requests his magic link). This ensures `LatestMessageTo`
reliably returns the invite email at that point.

### AcceptInviteBody token source

The `AcceptOrgInvite` endpoint requires a `token` body field (the raw invite
token from the accept URL). The invite email body format from `orgs.go` is:

```
{portalURL}/orgs/{orgID}/invites/{inviteID}/accept?token={raw}
```

The same `token=([A-Za-z0-9]+)` regex used for magic-link tokens extracts it.

### No new dependencies

`go.mod` and `go.sum` for both `jamsesh` root and `jamsesh/tests/e2e` are
unchanged. All helpers use stdlib only.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**:
- Hardcoded `alice@example.com` / `bob@example.com` could conflict if tests share a MailHog container across processes. Filed as `onboarding-test-randomize-emails` in `.work/backlog/`.

**Nits**:
- Token regex `[A-Za-z0-9]+` broader than actual `[0-9a-f]+` from `hex.EncodeToString`. Functionally correct but doesn't document the contract.
- `magicLinkTokenRE` and `inviteTokenRE` are identical — could be one constant.
- Playwright extension grew from 1 test to 3 (beyond story scope, but extra coverage is welcome).

**Notes**: The invite-token-vs-magic-link-token ordering (capture invite from MailHog before Bob's magic-link email arrives) is a subtle correctness point well-caught at implementation time and documented in the implementation notes. The 7-step flow reads top-to-bottom as documentation of the user journey. Helpers (`signInViaMagicLink`, `createOrg`, `inviteToOrg`, etc.) compose naturally for the next journey stories. `binary.Build` uses `sync.Once` correctly; `mailhog.LatestMessageTo` polls with a reasonable timeout.
