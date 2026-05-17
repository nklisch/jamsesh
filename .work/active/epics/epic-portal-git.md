---
id: epic-portal-git
kind: epic
stage: drafting
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

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->


## Anticipated child features

Provisional — actual decomposition lands when this epic is designed.

- Bare repo hosting (on-disk layout, lifecycle, gc concerns)
- Smart-HTTP serving (CGI wrap of `git-http-backend` or subprocess approach)
- HTTP Basic auth integration with portal token validation
- Pre-receive policy enforcement (trailers, scope globs, ref namespace,
  force-push rejection)
- Post-receive event emission into the portal event log
- Session base-ref creation flow (the special creator-pushes-HEAD pathway)

<!-- Design pass on each child feature will fill in specifics. -->
