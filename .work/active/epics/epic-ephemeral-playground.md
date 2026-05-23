---
id: epic-ephemeral-playground
kind: epic
stage: implementing
tags: [playground, portal, ui, plugin]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Ephemeral playground sessions (CLI-first creation)

## Brief

Anonymous, ephemeral playground sessions that lower the barrier to evaluating
jamsesh, paired with a unification of session creation onto a CLI-first
pattern (`jamsesh new` and `jamsesh new --playground`) that brings durable
sessions and playground sessions under the same mental model.

A prospect installs the Claude Code plugin (`claude plugin marketplace add
nklisch/jamsesh && claude plugins install jamsesh`), runs `jamsesh new
--playground` (or the namespaced alias `jamsesh playground new`) in their
local checkout, and gets back a session URL they can share with
collaborators. The CLI pushes their local HEAD as the session base ref,
mints them a pronounceable anonymous handle (e.g. `amber-otter`), and the
group runs a real multi-agent jam — push-per-commit, auto-merger, addressed
comments, conflict events — for the lifetime of the session window.

When the window closes (idle-timeout OR hard wall-clock cap, whichever
first), the session and every row that references its `session_id` are
destroyed. Finalize-out to a local source repo via the standard `jamsesh
finalize-run` flow remains the only way to carry work out.

This epic was scoped from `idea-ephemeral-jam-playground` and resolves the
SPEC.md deferred item "Public (open-join) sessions." Epic-design folded in
the CLI-first creation unification (originally floated as a follow-up scope
item) because the playground's local-push-as-base requirement made the
durable side's two-step "create-in-portal-then-join-from-CLI" pattern feel
arbitrary by comparison.

## Strategic decisions

Locked at scope-time and epic-design-time. Child features inherit these as
fixed framing and decompose inside them.

- **Audience**: top-of-funnel trial — optimize for first-touch conversion of
  prospective users. Existing teams keep using auth+org sessions for real
  work; playground is not a parallel mode for them.
- **Multi-tenancy**: reserved system-owned `playground` org, auto-provisioned
  at install time when `JAMSESH_PLAYGROUND_ENABLED=true`. Ephemeral sessions
  live inside this org so SPEC's "every persisted entity carries `org_id`"
  invariant holds without special-casing. No `org_id=NULL` paths, no
  per-session ephemeral orgs.
- **Persistence**: strictly ephemeral. No claim-to-durable path in v1. Session
  ends, all rows referencing the `session_id` are destroyed (FK
  `ON DELETE CASCADE` from `sessions` already handles `session_members`,
  `events`, `presence`; the destruction routine explicitly removes
  `comments`, `conflict_events`, and the bare repo on disk).
- **Abuse posture**: operator-opt-in. `JAMSESH_PLAYGROUND_ENABLED` defaults to
  `false` for self-host. Hosted jamsesh.com ships it on with sane per-IP
  session-create caps, per-session push-throughput caps, and content-size
  caps. Self-hosters who flip it on tune their own limits.
- **Identity model**: server-mints a pronounceable 2-word handle on first
  hit (`amber-otter`, `quiet-fox`, `swift-heron`, etc.) and exposes it in
  the UI for self-rename. Joiners may keep the suggestion or pick a custom
  handle (2-24 chars, letters / digits / dashes) at the join screen.
  Re-roll button offers a fresh server suggestion. Same handle is used in
  ref namespace, `@mention` addressing, and presence.
- **Destruction trigger**: **idle timeout** (default 30 min of no activity)
  **+ hard wall-clock cap** (default 24h since creation), whichever fires
  first. No finalize-driven destruction — finalize lets the human carry
  work out, then natural timeout cleans up the session. Both timers are
  visible to participants in the chrome countdown badge. Operator-tunable
  defaults via env vars.
- **Anonymous bearer data model**: extend the existing `oauth_tokens` table
  rather than create a parallel table. Add an `anonymous_session_bearer`
  value to the existing `kind` column and a nullable `session_id` FK column.
  Anonymous identities also get an `accounts` row marked
  `is_anonymous: true` so the existing `session_members.account_id` FK and
  membership-check middleware work unchanged. Bearer expires on session
  destruction (the destruction sweep revokes via `oauth_tokens.revoked_at`
  before deleting the session row).
- **Base ref**: creator pushes from their local repo checkout — same as
  durable sessions. No synthetic base. The unified `jamsesh new` /
  `jamsesh new --playground` CLI handles the push-as-base in one step.
- **Creation entry point unification (folded in at epic-design time)**:
  session creation moves to a CLI-first pattern. `jamsesh new` is the
  primary creation command for both durable and playground sessions.
  The portal UI's "New session" surface is reworked to be either a
  CLI-prompt-output generator (the portal collects the inputs, the user
  runs the printed CLI command locally) OR co-exists as an alternative for
  users who prefer the form — the exact shape is decided in
  `feature-epic-ephemeral-playground-portal-ui`. Both creation paths
  ultimately funnel through the same backend session-creation routine.

## Decomposition

Six child features cover the work. The CLI-first unification adds two
features (1 and 6) on top of the original four playground-specific arcs.
The dependency graph parallelizes into three waves:

- **Wave 1** (no deps, all parallel): `cli-first-creation`,
  `anon-bearer`, `reserved-org`
- **Wave 2** (depends on all of wave 1): `session-lifecycle`
- **Wave 3** (each depends on session-lifecycle, parallel with each other):
  `portal-ui`, `plugin-skills`

### Child features

- `feature-epic-ephemeral-playground-cli-first-creation` — `jamsesh new`
  Go subcommand that drives durable session creation end-to-end from the
  local checkout (interactive prompts for goal / scope / mode / org-pick;
  flag overrides for all of those; pushes local HEAD as base after the
  portal accepts the create call). Refactors the portal's session-create
  REST handler if needed to accept the CLI's create-then-push pattern as a
  first-class flow (today's flow assumes the join step seeds base). Updates
  the `/jamsesh:new` SKILL.md. **No playground concerns** — Feature 4 adds
  the `--playground` flag handling on top of this foundation.
  Depends on: `[]`.

- `feature-epic-ephemeral-playground-anon-bearer` — extend `oauth_tokens`
  with the `anonymous_session_bearer` kind value and a nullable
  `session_id` FK; add `accounts.is_anonymous` boolean; add
  `tokens.Service.IssueAnonymousSessionBearer(ctx, sessionID, nickname)`;
  ensure existing `tokens.Validate`, REST middleware, MCP `verifyToken`,
  and git Basic-auth resolution work unchanged for anonymous bearers.
  Roll the bearer model section forward in `docs/SECURITY.md` and
  `docs/PROTOCOL.md`. Depends on: `[]`.

- `feature-epic-ephemeral-playground-reserved-org` — config knob
  (`JAMSESH_PLAYGROUND_ENABLED`, `JAMSESH_PLAYGROUND_IDLE_TIMEOUT`,
  `JAMSESH_PLAYGROUND_HARD_CAP`, abuse-cap env vars); idempotent
  startup hook in `cmd/portal/main.go` that seeds the reserved `playground`
  org row when the flag is true; reserved-org guard rails that reject any
  REST attempt to delete or rename the playground org. Update `docs/SELF_HOST.md`
  with the env-var reference. Depends on: `[]`.

- `feature-epic-ephemeral-playground-session-lifecycle` — the playground
  capability core. Adds the `--playground` flag handling to `jamsesh new`
  (bypasses auth, targets the reserved playground org); pronounceable-handle
  generator (server-side, with collision retry); destruction-trigger background
  worker (per-session idle timer + hard-cap sweep); destruction routine that
  revokes all bearers, deletes comments + conflict_events explicitly, deletes
  the session row (FK cascades the rest), and removes the bare repo from disk;
  abuse caps (per-IP session-create rate limit at REST handler, per-session
  push-throughput cap at `pre-receive`, per-session total content-size cap).
  Roll lifecycle and abuse sections forward in `docs/SPEC.md` (concrete
  default values) and `docs/SECURITY.md` (abuse threat model). Depends on:
  `[feature-epic-ephemeral-playground-cli-first-creation,
    feature-epic-ephemeral-playground-anon-bearer,
    feature-epic-ephemeral-playground-reserved-org]`.

- `feature-epic-ephemeral-playground-portal-ui` — unauthenticated portal
  landing surface (CTA + CC plugin install one-liner + share-link viewer);
  joiner nickname picker with server-suggested handle + reroll; auth-gate
  exemption in `App.svelte` for playground routes; anonymous-mode chip +
  countdown badge added to `SessionViewShell` chrome (visible only when the
  session's org is the reserved playground org); idle / hard-cap warning
  banners (rendered ~5 min before destruction); post-destruction confirmation
  page; rework of `NewSessionDrawer` to align with the CLI-first pattern
  (either output the CLI command after collecting inputs, or stay as an
  alternative form — designed per `## Mockups` flow). Roll relevant `docs/UX.md`
  flows forward. Depends on:
  `[feature-epic-ephemeral-playground-session-lifecycle]`.

- `feature-epic-ephemeral-playground-plugin-skills` — `/jamsesh:new` SKILL.md
  (wraps `jamsesh new`); `/jamsesh:playground:new` SKILL.md (namespaced
  shortcut wrapping `jamsesh new --playground`); update `/jamsesh:join`
  SKILL.md + binary subcommand to accept playground URLs (recognizes the
  `/playground/s/<token>` URL shape, fetches the anonymous bearer, writes
  it to `${CLAUDE_PLUGIN_DATA}/sessions/<session-id>/token`); update the
  auto-loaded `skills/jamsesh/SKILL.md` to teach agents about playground
  semantics (ephemeral, no persistent identity, destruction trigger,
  finalize-to-keep-work imperative). Depends on:
  `[feature-epic-ephemeral-playground-cli-first-creation,
    feature-epic-ephemeral-playground-session-lifecycle]`.

### Decomposition risks

- **Critical path through `session-lifecycle`**. Wave 2 has only one feature
  and waves 1 and 3 have multiple parallelizable features, so the lifecycle
  feature is the single longest pole. Mitigation: scope it tightly when
  feature-design runs — destruction-trigger correctness and abuse caps are
  the high-risk areas; everything else inside it is fairly mechanical.

- **`cli-first-creation` may shift `portal-ui` scope mid-implementation**.
  Whether the durable-side `NewSessionDrawer` stays, mutates, or becomes
  CLI-output-only depends on what `cli-first-creation` actually exposes. The
  `portal-ui` feature has `session-lifecycle` as its declared dependency but
  is also conceptually downstream of `cli-first-creation`. Mitigation: when
  designing `portal-ui`, re-read the `cli-first-creation` feature body for
  the resolved shape; add an explicit depends_on edge if it turns out to
  matter for ordering.

- **Abuse-cap defaults are guesses without hosted operating experience**.
  Defaults like "per-IP 3 session-creates/hour" are placeholders. The first
  real abuse pattern after launch may demand a hot-fix. Mitigation: make all
  caps env-var-tunable (no recompile required); add a `playground.abuse`
  Prometheus metric label so spikes are visible.

- **Pronounceable-handle collision space**. With 2-word handles drawn from
  256x256 wordlists, ~65k unique combinations. With session caps at 5
  concurrent participants, collision rate within a session is negligible;
  across sessions, names will repeat — fine because they're session-scoped.
  Mitigation: name uniqueness check is per-session, not global.

- **Reserved-org guard rails are easy to forget**. Any new "delete org" or
  "rename org" REST handler added in unrelated work could accidentally allow
  destroying the playground org. Mitigation: gate at the data layer
  (`org_protected: true` boolean column on `orgs` set on the playground
  row at provisioning time), not at handler level — defense in depth.

## Mockups

- **Flow**: `.mockups/flows/playground-onboarding/index.html` — 7-step
  hybrid topology (sequential primary path, modal cross-jump from creator
  session to share-link, terminal loop from post-destruction back to
  prospect landing).
- **Steps**: `01-prospect-landing` → `02-create-cli-output` →
  `03-creator-session` → `04-share-link` → `05-joiner-nickname-picker` →
  `06-joiner-session` → `07-session-end` (three stacked sub-states: idle
  warning, hard-cap warning, post-destruction).
- **Inherits**: `.mockups/design-system/tokens.css` (Quiet Slate +
  Geist); visual language anchored on
  `.mockups/screens/epic-portal-ui-session-view-shell/option-1.html`.
- **Signed off**: 2026-05-23.
- **Inheritance directive for child features**: `portal-ui` and
  `plugin-skills` features reference this flow by path; feature-design
  Phase 4.6 fallback should NOT re-mock the playground onboarding journey
  — only mock individual surfaces if a feature introduces a screen state
  not captured here (none anticipated).

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

Foundation roll-forward owned by child features (drafted into each feature
body during its design pass): `docs/SECURITY.md` and `docs/PROTOCOL.md`
(anon-bearer + abuse-vector specifics), `docs/SELF_HOST.md` (env var
reference), `docs/UX.md` (playground flows + the CLI-first unification of
durable session creation), the OpenAPI spec (new REST routes for anonymous
bearer issuance, playground session creation, destruction status).
