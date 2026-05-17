---
id: epic-e2e-tests-golden-path-session-lifecycle
kind: story
stage: implementing
tags: [e2e-test, testing]
parent: epic-e2e-tests-golden-path
depends_on: [epic-e2e-tests-golden-path-ccdriver-env-fix, epic-e2e-tests-golden-path-onboarding-auth]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Golden — Session lifecycle (join, push, peer activity)

## Scope

Two specs that together prove an agent can join a session, push
commits to its ref, and see another agent's commits arrive via `git
fetch` + the WebSocket event stream.

- `tests/e2e/golden/session_join_and_push_test.go` (Go) — two
  ccdriver-driven "agents" pushing on independent refs, observing
  each other
- `tests/e2e/playwright/session_list.spec.ts` (Playwright) — the
  authenticated user opening the sessions list, clicking a session,
  and seeing WebSocket-delivered events render in the session view
  shell

## Go spec invariant

After Agent A and Agent B both join the same session and push commits
on independent refs, each agent's local working copy can `git fetch`
the peer's ref tip, AND the `commit.arrived` events for both pushes
land in the portal's WebSocket event stream within 2 seconds of the
push.

## Files to create / modify

- `tests/e2e/golden/session_join_and_push_test.go` — the Go spec
- `tests/e2e/playwright/session_list.spec.ts` — Playwright spec
- `tests/e2e/fixtures/wsclient/wsclient.go` (NEW) — small helper that
  connects to `/ws/sessions/{sessionID}` via the `coder/websocket`
  client (or directly via `gorilla/websocket`), reads events into a
  channel, and exposes `WaitFor(type, timeout)` for assertions
- `tests/e2e/fixtures/gitclient/gitclient.go` (NEW) — helper that
  exposes `Clone(url, tmpdir)`, `Commit(repo, file, content, msg)`,
  `Push(repo, ref)` — wrappers around `os/exec` for `git`. Uses
  appropriate trailers (`Jam-Session`, `Jam-Turn`, `Jam-Author`)
  derived from session/ref context.

## Acceptance criteria

- [ ] Go spec is green; runs in under 60s
- [ ] Both agents push commits on `jam/<session>/<user>/main` and
      receive `commit.arrived` events via the WebSocket subscription
- [ ] Each agent's `git fetch` after the peer's push surfaces the
      peer's commit (verified by `git log <peer-ref>` containing the
      peer's SHA)
- [ ] Playwright spec navigates to the sessions list, sees the
      created session, clicks into the session view, and verifies a
      WebSocket-delivered event renders within 5 seconds
- [ ] WebSocket event payloads match the OpenAPI envelope schema
      (`{seq, version, type, payload, timestamp, session_id}`)

## Notes for the implementer

- The git smart-HTTP endpoints are at `/{orgID}/{sessionID}.git/info/refs`
  + `git-upload-pack` + `git-receive-pack` per
  `internal/portal/githttp/handler.go`
- Authentication for git smart-HTTP uses HTTP Basic with the bearer
  token as password — see the auth middleware in
  `internal/portal/githttp/handler.go`
- Pre-receive validators require `Jam-Session`, `Jam-Turn`,
  `Jam-Author` trailers — the gitclient fixture must produce them
- WebSocket subprotocol auth: `Sec-WebSocket-Protocol:
  jamsesh.bearer.<token>` per the architecture doc
