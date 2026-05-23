---
id: epic-ephemeral-playground
kind: epic
stage: drafting
tags: [playground, portal, ui]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Ephemeral playground sessions

## Brief

Anonymous, ephemeral playground sessions that lower the barrier to evaluating
jamsesh. A prospective user lands on the portal, clicks "Try a playground
session," gets a session URL they can share with one to four collaborators,
and the group can run a real multi-agent jam against a synthesized base ref
for the lifetime of the session window. No OAuth, no org membership, no
commitment. When the window closes (finalize-driven, timeout, or both — exact
trigger decided in epic-design), the session and every row that references it
are destroyed.

The point is first-contact: a prospect feels the substrate — push-per-commit,
auto-merger, addressed comments, conflict events, the converged draft — in a
real jam before deciding whether to stand up a portal, OAuth provider, and
org for production use. The durable substrate is unchanged; playground is a
trial surface that lives alongside it.

This epic was scoped from `idea-ephemeral-jam-playground` and resolves the
SPEC.md deferred item "Public (open-join) sessions."

## Strategic decisions

Locked at scope-time. Epic-design inherits these as fixed framing and
decomposes inside them.

- **Audience**: top-of-funnel trial — optimize for first-touch conversion of
  prospective users. Existing teams keep using auth+org sessions for real
  work; playground is not a parallel mode for them.
- **Multi-tenancy**: reserved system-owned `playground` org, auto-provisioned
  at install time when `JAMSESH_PLAYGROUND_ENABLED=true`. Ephemeral sessions
  live inside this org so SPEC's "every persisted entity carries `org_id`"
  invariant holds without special-casing. No org_id=NULL paths, no
  per-session ephemeral orgs.
- **Persistence**: strictly ephemeral. No claim-to-durable path in v1. Session
  ends, all rows referencing the `session_id` are destroyed. Finalize-out to
  a local source repo via the standard `jamsesh finalize-run` flow remains
  the only way to carry work out of a playground session.
- **Abuse posture**: operator-opt-in. `JAMSESH_PLAYGROUND_ENABLED` defaults to
  `false` for self-host. Hosted jamsesh.com ships it on with sane per-IP
  session-create caps, per-session push-throughput caps, and content-size
  caps. Self-hosters who flip it on tune their own limits.

## Open design questions for epic-design

These are decomposition-level — epic-design Phase 4.x resolves them and emits
child features:

- **Identity model.** Random pronounceable nicknames, anonymous-with-PIN, or
  one-time bearer token issued at join. Affects the join UX, the addressing
  story in `comments`, and whether the bearer is recoverable across browser
  reloads.
- **Git ref namespace shape.** Anonymous participants almost certainly need a
  ref slot to push into. Confirm `jam/<session>/<anon-user-id>/<branch>` is
  the right layout, or whether anonymous identifiers need a separate prefix
  (`jam/<session>/anon-<token>/<branch>`) so `pre-receive` can branch on
  identity kind during scope validation.
- **Anonymous bearer issuance for CC plugin.** Confirm the CC plugin's OAuth
  path can be bypassed for playground — the `jamsesh join` flow needs to
  accept a playground URL, fetch a session-scoped anonymous bearer, and write
  it to `${CLAUDE_PLUGIN_DATA}/token` the same shape as a normal OAuth token.
- **Abuse-vector specifics.** Default values for per-IP session-create cap,
  per-session push-throughput cap, per-session total content-size cap, max
  concurrent anonymous participants per session. Where each limit is enforced
  (router, REST handler, `pre-receive`, finalize gate). Hosted vs. self-host
  defaults.
- **Destruction trigger.** Finalize-driven only, hard wall-clock timeout
  only, idle-timeout only, or some combination. Sets the lifecycle contract
  and the session-end UX (countdown, warning, grace period).
- **Session-base synthesis.** Playground sessions don't have a source repo to
  push HEAD from. Decide what `jam/<session>/base` is seeded with — empty
  tree, scaffolded sample repo, user-selectable starter — and how the
  writable scope is set (probably permissive `**` for playground; revisit if
  abuse demands tighter defaults).

## UI/UX

Per the mockup-first convention and `ux-ui-design:ux-ui-principles` tier rule,
flow-level mocking for the playground onboarding journey (anon landing →
create-session → share-link → join-via-link → in-session → session-end /
destruction confirmation) is the responsibility of `epic-design` Phase 4.6.
The design system tokens are already locked in (`tokens.css` exists), so
palette is not re-invoked.

The playground introduces at least one net-new top-level surface (the
unauthenticated landing/CTA) plus session-end countdown and post-destruction
confirmation pages. Epic-design lists each surface and mocks them as part of
decomposition.

## Foundation roll-forward (done at scope-time)

- `docs/VISION.md` — added playground capability bullet to "What you get";
  extended "Who it's for" to describe the prospective-user / first-contact
  arc.
- `docs/SPEC.md` — qualified the "Multi-tenant by design" hard constraint
  with the reserved `playground` org clause; added "Ephemeral playground
  sessions" subsection under Lifecycle; added the anonymous-bearer auth-model
  bullet; added `JAMSESH_PLAYGROUND_ENABLED` to the env-var list; removed
  "Public (open-join) sessions" from the deferred list and added the explicit
  "Claim-to-durable on playground sessions" deferral in its place.
- `docs/ARCHITECTURE.md` — added the "Reserved orgs" paragraph to the
  data-layer section noting how the auto-provisioned `playground` org keeps
  the `org_id`-in-WHERE pattern unbroken.

Foundation impact on `docs/PRINCIPLES.md`, `docs/SECURITY.md`,
`docs/SELF_HOST.md`, `docs/UX.md`, `docs/PROTOCOL.md`, and the OpenAPI spec
flows out of epic-design — each child feature rolls its slice forward when
it's drafted (anonymous-auth model into SECURITY/PROTOCOL, env-var details
into SELF_HOST, playground surfaces into UX, new REST routes into the OpenAPI
spec).
