---
id: epic-e2e-tests-golden-path-session-lifecycle
kind: story
stage: done
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

## Implementation notes

### What was built

- `tests/e2e/fixtures/wsclient/wsclient.go` — WebSocket client fixture using
  `github.com/coder/websocket`. Authenticates via `Sec-WebSocket-Protocol:
  jamsesh.bearer.<token>`. Exposes `Connect`, `WaitFor`, `Events`, `Close`.
- `tests/e2e/fixtures/gitclient/gitclient.go` — git CLI wrapper that injects
  HTTP Basic credentials into the clone URL and appends `Jam-Session`,
  `Jam-Turn` (fresh UUID per commit), `Jam-Author` trailers to every commit
  message via a temp file + `git commit -F`.
- `tests/e2e/golden/session_join_and_push_test.go` — `TestSessionLifecycleJoinAndPush`:
  two agents sign in, Bob gets invited to org then session, both subscribe to WS,
  Alice pushes → both see `commit.arrived`, Bob pushes → both see it, Alice fetches
  Bob's ref and verifies the SHA. Runs in ~12 s.
- `tests/e2e/playwright/session_list.spec.ts` — three Playwright specs covering
  session list rendering, row click navigates to session view, and direct session
  view navigation. All stubs the API so they run without a live data set.

### Production bugs found and fixed

1. **Dockerfile used `distroless/static` which has no `git` binary**  
   `internal/portal/storage/repo.go` calls `exec.Command("git", "init", "--bare")`.
   Switched base image to `debian:bookworm-slim` + `apt-get install git`.
   Backlog item: `portal-docker-image-missing-git-binary.md`.

2. **`logging.Access` middleware broke WebSocket upgrade**  
   `statusRecorder` embedded `http.ResponseWriter` but didn't implement `Unwrap()`.
   `coder/websocket` uses `Unwrap()` to find `http.Hijacker` through middleware chains.
   Fixed by adding `func (s *statusRecorder) Unwrap() http.ResponseWriter` in
   `internal/portal/logging/logging.go`.

3. **Portal container lacked a writable storage directory**  
   Default storage is `./storage` — not writable for `nobody:nogroup`.
   Fixed in `tests/e2e/fixtures/portal/portal.go` by setting
   `JAMSESH_STORAGE=/tmp/jamsesh-repos` in the default container env.

### Dependency changes

- `tests/e2e/go.mod`: added `github.com/coder/websocket v1.8.14` and
  `github.com/google/uuid v1.6.0` as direct dependencies.

## Review (2026-05-17)

**Verdict**: Approve with comments

**Blockers**: none

**Important**:
- Production `Dockerfile` base image changed from `gcr.io/distroless/static:nonroot` to `debian:bookworm-slim` + apt-installed git. Real production bug fix (the portal's `git init --bare` subprocess would have failed in distroless), but a substantial operations change. Filed as `portal-prod-dockerfile-base-image-review` in `.work/backlog/` for ops review of image size, attack surface, user-name change (nonroot → nobody), and cosign signing.
- `Dockerfile` (production, debian+git) and `Dockerfile.e2e` (alpine+git) duplicate the git-install pattern across two base images. Filed as `portal-unify-dockerfile-strategy` for cleanup once the base-image-review decision lands.
- `logging.statusRecorder.Unwrap()` is the correct fix using Go's standard chained-writer pattern, but no unit test covers it directly. Coverage is indirect via the WS gateway tests. A focused unit test (`Unwrap() returns the embedded writer`) would document the contract.

**Nits**:
- `gitclient` uses `jamsesh` as the Basic-auth username — readable choice; the portal accepts any non-empty username.
- The migration `00012_sessions_finalizing_status` was authored by `interrupted-ops` but committed in this commit due to parallel-agent merge. Not actionable here.

**Notes**: The story's core deliverables — the wsclient + gitclient fixtures, the journey spec, the Playwright specs — are clean. Assertions pin to user-visible outcomes (WS event arrival, git fetch result, SHA equality). The QP-decode helper from the earlier session is correctly reused via authflow. Three production bugs caught and fixed — exactly the value the e2e program is supposed to deliver. The Dockerfile change is the most significant non-test side effect and deserves explicit ops review before release.
