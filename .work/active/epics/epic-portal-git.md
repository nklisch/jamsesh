---
id: epic-portal-git
kind: epic
stage: implementing
tags: [portal]
parent: null
depends_on: [epic-portal-foundation]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Git Server

## Brief

Hosts one bare git repo per session on disk under
`<storage>/orgs/<org-id>/sessions/<session-id>.git`. Exposes them over HTTPS
via smart-HTTP: `git-receive-pack` for push, `git-upload-pack` for fetch,
`info/refs` for capability negotiation. Wraps git's canonical
`git-http-backend` CGI (or invokes the pack subprocesses directly) with
Go-implemented HTTP Basic authentication using the user OAuth token as
password.

The pre-receive hook is the policy-enforcement point: required commit
trailers (`Jam-Session`, `Jam-Turn`, `Jam-Author`), writable scope
enforcement (every changed path must match the session's declared globs),
ref namespace enforcement (the pushed ref must be in the authenticated
user's `jam/<session>/<user>/*` namespace), no force-pushes on shared refs
(`base`, `draft`). Rejection messages list offending commits/paths.

The post-receive hook is the integration point for the rest of the portal:
emits `commit.arrived` events into the portal event log for each accepted
commit, which the WebSocket gateway and auto-merger both subscribe to.

Also includes the session-creation base-push flow: when a creator pushes
their source-repo HEAD to `jam/<session>/base` during session creation, the
pre-receive hook permits this specific operation despite `base` normally
being read-only.

This epic does NOT cover the auto-merger (`epic-auto-merger` consumes
post-receive events); it does NOT cover any REST endpoints
(`epic-portal-api`).

## Foundation references

- `docs/SPEC.md` — Ref structure, Hard constraints
- `docs/ARCHITECTURE.md` — Git smart-HTTP component
- `docs/SECURITY.md` — Git push authorization
- `docs/PROTOCOL.md` — Git smart-HTTP routes, Commit trailer conventions

## Design decisions

Locked at epicize time:

- **Smart-HTTP serving mechanism**: invoke `git-receive-pack` and
  `git-upload-pack` as subprocesses directly (the Gitea/Forgejo pattern).
  Portal HTTP handlers parse the request, run pre-receive validation,
  spawn the subprocess with appropriate env (`GIT_DIR`, etc.), pipe
  request body to stdin and stream stdout back to the response. Most
  control; cleaner than CGI wrapping for our auth + policy injection
  needs. Process-per-request cost is acceptable for jamsesh's scale.
- **Archived-session semantics (after 90-day retention)**: hard delete
  the bare repo and DB rows. Retain a tiny `archived_sessions` table row
  per archived session with: name, member list (account ids only),
  goal/manifest text, end date, end reason (finalize/abandon/timeout),
  the finalized branch name if any (purely for the session URL's
  archived-stub response). The session URL responds with that stub:
  "This session was archived on YYYY-MM-DD. Final branch:
  `<name>` (pushed to <repo>)." Cleanest storage footprint; clearest
  data-retention story for GDPR/compliance asks. No restore path
  by design — restore is "you should have used the 90-day window."

Locked at epic-design time (this pass):

- **Bare repo init timing**: eager. The bare repo is created during
  `POST /api/sessions` handling (cross-epic call from portal-api into a
  storage helper here), atomic with the `sessions` row insert. Order:
  create repo, then insert row; on insert failure, `rm -rf` the repo.
  Invariant after success: "session row exists ⟹ bare repo exists."
  Rationale: simpler invariant than lazy-init; no smart-http handler
  needs to guard against a missing repo.
- **Pre-receive execution model**: validation runs in the portal's Go
  HTTP handler before invoking `git-receive-pack`. NOT via a shell
  `hooks/pre-receive` script that calls back into the portal. The
  handler uses `go-git` (already SPEC.md-locked) for object walking
  and pack inspection. Rationale: matches Gitea/Forgejo; avoids
  fork-back-into-portal complexity; keeps policy enforcement on the
  request path.
- **Concurrent push handling**: rely on git's native ref locking. No
  portal-level lock layer. Concurrent pushes to different refs proceed
  in parallel; same-ref pushes serialize naturally. Rationale: git
  already solved this; layering portal locking would be both slower
  and a correctness regression.
- **Storage path schema**: lock the v1 layout
  `<storage>/orgs/<org_id>/sessions/<session_id>.git`. No path
  abstraction layer. Future reshapes (sharding, renames) ship as
  explicit one-shot migration tools. Rationale: premature abstraction
  is worse than a future migration.
- **Pack-file size cap**: 50 MB per push by default, configurable via
  portal config (`git.max_push_bytes`). Rejected with `push.size_limit`
  error code. Rationale: protects the portal; default is generous for
  the doc-writing use case; operators can tune.

## Decomposition

Four child features, split by responsibility within the receive-pack
lifecycle:

- **storage** is the foundation — bare repo creation, archived-session
  semantics, on-disk path resolution. Everything else consumes it.
- **pre-receive** is the policy library that runs before
  `git-receive-pack` accepts a push.
- **post-receive** emits events into the portal event log after
  acceptance.
- **smart-http** is the HTTP handler trio that assembles everything,
  including HTTP Basic auth via the foundation tokens feature.

Critical path: `storage → {pre-receive || post-receive} → smart-http`.
Three deep with the middle pair parallelizable. The only cross-epic dep
is `smart-http → epic-portal-foundation-tokens` for the token validator
that maps HTTP Basic password to an account.

### Child features

- `epic-portal-git-storage` — bare-repo init/teardown, on-disk layout,
  archived-session table + stub formatter, atomic creation with sessions
  row — depends on: `[]`
- `epic-portal-git-pre-receive` — in-process Go validation library
  (trailers, scope globs, ref namespace, force-push rejection, size
  limit) using `go-git` — depends on: `[epic-portal-git-storage]`
- `epic-portal-git-post-receive` — commit-arrived event emission into
  the portal event log (table owned by `epic-portal-api`) — depends on:
  `[epic-portal-git-storage]`
- `epic-portal-git-smart-http` — HTTP handler trio (`info/refs`,
  upload-pack, receive-pack), HTTP Basic auth, subprocess invocation,
  streaming, archived-stub response — depends on:
  `[epic-portal-git-storage, epic-portal-git-pre-receive,
  epic-portal-git-post-receive, epic-portal-foundation-tokens]`

### Decomposition risks

- **Pre-receive is the highest-risk feature.** Wire-protocol validation
  has historically been a long tail of edge cases. Mitigation: use
  `go-git` (already locked) for object walking rather than rolling our
  own pack parser; lock the validation test matrix at feature-design
  time (the cartesian of trailer × scope × namespace × force-push ×
  size).
- **Cross-epic dep on the events table.** post-receive writes into the
  `events` table owned by `epic-portal-api`. The design pass on
  post-receive is best sequenced after portal-api's events-table feature
  exists; if running ahead, lock to `docs/PROTOCOL.md > WebSocket event
  types` (which already pins `commit.arrived` payload shape).
- **Streaming discipline.** smart-http must stream gigabyte-scale
  fetches without memory blowup (`io.Pipe` + `http.Flusher`). Subprocess
  error mid-stream needs to map to the git-protocol report-status the
  client expects. Mitigation: design pass references the Gitea/Forgejo
  pattern and produces a fault-injection test plan.
- **Storage atomicity.** "Repo created before session row inserted" is a
  cross-process operation. If row insert fails after repo creation, the
  storage feature must clean up loudly (alert on failure). If row insert
  succeeds but repo creation already happened, the invariant holds.
  Reverse order (insert first, then create repo) is rejected: would
  produce orphan session rows under failure.
