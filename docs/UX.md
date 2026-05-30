# User Experience

How humans interact with jamsesh.

## Interaction model

Two surfaces, one workflow.

**Claude Code** is where the work happens. Humans drive their agents in CC
exactly as they do for any other CC session. The jamsesh plugin adds slash
commands (`/jamsesh:jam`, `/jamsesh:finalize`), surfaces session context
in agent turns via the digest, and pushes work to the session remote on
commit boundaries. Most of a participant's time is here.

**The portal UI (web)** is the awareness surface. Humans open it in a browser
tab and glance at it while their agents work. It shows the live tree, presence,
the converged draft, comments from peers, and conflict events. It's where
humans drop addressed comments, fork from peers' commits, and curate the
final commit sequence at finalize time. It's not where work gets done — it's
where humans see what's happening and react.

Both surfaces are first-class. The portal is not a "lesser" view of CC, and
CC is not a "lesser" view of the portal. They complement.

UI mockups live in `.mockups/` and are generated via the `ux-ui-design`
plugin's `screens`, `flows`, and `palette` skills as concrete surfaces are
designed. This doc captures intent and flow; mockups capture appearance.

## Flow: creating a session

Session creation is CLI-first: the `jamsesh new` subcommand creates the
session, pushes the local HEAD as the base ref, and writes the per-session
state — all in one step from the user's checkout.

### Primary path: agent-driven (inside Claude Code)

A human asks their Claude Code agent to spin up a session. The agent maps
the request to explicit flags and calls the binary — it never sees stdin
prompts.

```
jamsesh new \
  --org <org-id>           # required in non-TTY; use 'jamsesh status --json' to list
  --goal "auth refresh"    # free-text; defaults to empty (editable later in portal)
  --scope "docs/auth/**"   # writable glob; defaults to '**' (permissive)
  --mode sync              # sync (default) or isolated
  --invite alice@x,bob@x   # optional; invites sent post-create
  --open                   # optional; opens portal session view in browser after create
```

On success:
1. A session row is created on the portal with the given params.
2. Local `HEAD` is pushed to `refs/heads/jam/<session-id>/base` on the
   portal-side bare repo (the base SHA is stamped by the portal's
   post-receive hook).
3. Per-session state files are written to
   `${JAMSESH_DATA_DIR}/sessions/<session-id>/` (ref, org_id, account_id,
   last_seen_seq). The `instance_id` binding happens at first
   `/jamsesh:jam join` — not here, since the user may run
   `jamsesh new` from plain bash without a CC instance attached.
4. A success summary is printed with the session URL, org, goal, scope,
   mode, and the `jamsesh join <session-id>` command collaborators can run.
5. If `--open` was passed, the portal session view opens in the default
   browser. Browser-launch failure degrades gracefully: the URL is printed
   and the command still exits 0.

If the push fails the session stays **live** with `base_sha: null`. The CLI
prints the explicit retry command:
```
git push <session-remote-url> HEAD:refs/heads/jam/<session-id>/base
```
The `/jamsesh:jam` skill instructs the agent to retry the push
automatically once before surfacing the error to the human.

### Secondary path: interactive (human in a terminal)

When stdin is a TTY and `--org` is omitted, `jamsesh new` presents a
numbered-list org picker with the most-recently-used org pre-selected
(marked with `*`). The human presses Enter to accept the default or types
a number. All other parameters default sensibly; the human can override
any of them via flags.

```
$ jamsesh new
Which org for this session?
  [1]  Acme Engineering (org-abc)
  [2]* Personal Projects (org-xyz)
Pick a number [1-2, default 2]:
Session created: jam-1748000000 (sess-...)
  URL:    https://jamsesh.example.com/orgs/org-xyz/sessions/sess-...
  ...
```

## Flow: spinning up a playground

Anyone — signed in or not — can start an ephemeral collaboration sandbox
from `/playground` without provisioning an org. The session lives in the
reserved playground org for a hard-capped wall-clock window (default 24h)
or until idle-timeout (default 30 min of no substantive activity), whichever
fires first.

1. From the SPA, the visitor opens `/playground` and submits the create
   form (optional name + goal + nickname). The portal returns the new
   session id and an anonymous bearer scoped to that session.
2. The SPA stores the bearer + nickname in the `auth` rune store's
   in-memory `playgroundContext` field (no localStorage — a tab reload
   intentionally drops the identity), navigates to
   `/playground/s/{id}`, and presents the standard session view with a
   `CountdownBadge` showing remaining session time and a
   `DestructionWarningBanner` that fires inside the warning window.
3. The same flow is available from the CLI as
   `jamsesh new --playground`; the flag binds the new session to the
   playground org and writes the anonymous bearer into the per-session
   token file at `${JAMSESH_DATA_DIR}/sessions/<id>/token`.
   Adding `--open` opens the join page (`/playground/s/{id}/join`) in
   the browser, where a fresh browser participant is minted via the
   `JoinerPicker` (nickname picker). This is a new identity — it does
   not resume the CLI's anonymous session identity.

## Flow: joining a playground

A collaborator received a `/playground/s/{id}/join` URL.

1. They open the URL. The SPA renders the `JoinerPicker` screen with
   `JoinerForm` (nickname input, suggested handle pre-filled from the
   wordlist) and submits to `POST /api/playground/sessions/{id}/join`.
2. The portal mints a fresh anonymous bearer scoped to this session and
   returns it; the SPA stores it in the same in-memory
   `playgroundContext` and navigates to `/playground/s/{id}`.
3. If the session hit `MaxParticipants` (default 5), the portal returns
   `409 playground.session_full` and the picker surfaces a "session full"
   message. If past `hard_cap_at`, the portal returns
   `410 playground.session_ended` and the SPA redirects to
   `/playground/s/{id}/ended` for the post-destruction tombstone view.

## Flow: joining a session

A collaborator received an invite or a join URL.

1. From a checkout of the source repo, they run `/jamsesh:jam join <session-id-or-url>`.
2. If not authenticated to the portal, OAuth flow runs (browser opens; token
   stored locally on completion).
3. The local binary verifies they're a session member (or completes the
   invite if they're using an invite URL).
4. The local binary clones the session remote into a working tree, checks out
   `jam/<session>/<user>/main`, configures the session remote, sets up
   `post-commit` hook and CC plugin state.
5. The SessionStart hook fires, injecting the session goal, writable scope,
   peer ref tips, current draft state, and any unresolved addressed comments
   into the agent's context.
6. If `--open` was passed, the portal session view opens in the browser.
   Browser-launch failure degrades gracefully: the URL is printed and the
   command still exits 0.
7. They start prompting their agent normally.

## Flow: an agent turn

From the human's perspective.

1. Human submits a prompt in CC.
2. The agent's opening context now includes:
   - The standard CC context (the codebase, the user's prompt, etc.)
   - The session digest: peer commits since last turn, new comments
     addressed to this agent, new conflict events, current draft tip.
3. The agent reads, reasons, edits files normally, commits at semantic
   checkpoints (per the skill's commit-quality instructions).
4. Each commit triggers a push (PostToolUse hook). Peers see the work
   appear in their tree views in near-real-time.
5. If a commit auto-merges into draft, peers' next turns will start from a
   richer draft. If a conflict fires, both involved agents see it next turn.
6. The agent yields control. Stop hook auto-commits any remainder, pushes
   once more, marks the turn ended.
7. Human sees the agent's work in CC, glances at the portal tab to see peers'
   activity, decides next prompt.

## Flow: posting a comment

Humans comment via the portal UI; agents comment via the MCP `post_comment`
tool. Same data model.

**Human posting:**

1. In the portal UI, open the session view.
2. Click on a commit in the tree to view its diff or files.
3. Select a line range in a file (or click "comment on commit" for a
   commit-level comment).
4. Compose the comment body. Optionally address (`@user`, `@user/branch`,
   `@all-agents`, `@everyone`, `@auto-merger`) and set kind (question,
   suggestion, action-request, fyi).
5. Submit.

**Agent posting:** the agent calls `post_comment` MCP tool with the same
parameters. Skill instructions teach the agent when and how to use it (e.g.,
"if you want a teammate to consider your point of view, address them
directly with kind `suggestion`").

Addressed comments arrive in the recipient's next turn digest. Anyone in the
session can resolve a comment via the portal UI or MCP `resolve_comment`.

## Flow: forking from a peer

A human sees a commit on a peer's ref that they want to build on.

**From the portal UI:**

1. Click the commit in the tree.
2. Click "Fork from here."
3. Choose: replace your current ref OR create a new sibling ref.
4. If sibling, name it and pick mode (sync or isolated).
5. Confirm.
6. The fork is created server-side. On the agent's next turn, the
   `UserPromptSubmit` hook fetches `session-remote` automatically — the new
   ref is available locally without any manual step.

**From CC:**

1. `/jamsesh:jam` — tell the agent "fork from `<commit-sha>`" (optionally
   naming the branch and mode). The agent invokes
   `jamsesh fork <commit-sha> [--as <branch>] [--mode sync|isolated]`.
2. The local binary calls the portal `fork` MCP tool.
3. The local checkout is reset to the new parent automatically.

## Flow: switching mode (sync ↔ isolated)

A human realizes their current line of work would be better as a private
exploration (or that an isolated branch is ready to rejoin the trunk).

**To go isolated:**

1. `/jamsesh:jam` — tell the agent "switch to isolated mode". The agent
   invokes `jamsesh mode isolated`.
2. Or click the mode badge on your ref in the portal tree view, choose
   "isolated."
3. Future commits on this ref are not auto-merged. Conflict events stop
   firing for this ref. The tree visually detaches the ref from the trunk.

**To rejoin sync:**

1. `/jamsesh:jam` — tell the agent "switch to sync mode". The agent
   invokes `jamsesh mode sync`.
2. The next push triggers auto-merger processing for any commits accumulated
   while isolated. Expect conflicts proportional to drift.
3. Resolve conflicts as they fire (in the digest, addressed to you).

## Flow: finalizing

A team agrees the jam is complete and wants to ship the result.

1. Any participant clicks "Finalize" in the portal UI, or runs
   `/jamsesh:finalize` in CC.
2. The portal opens the finalize view:
   - Default selection: the leaf agent commits reachable from `draft` (the
     auto-merger's merge commits are linearized out server-side).
   - Default finalization mode: **squash into one commit** — matches the
     conventional PR-shipping shape. The human can flip to **preserve all
     commits** if they want multi-author history on the target branch.
   - They can: add commits from isolated refs that should be included,
     remove commits from the default selection, reorder, rename the target
     branch. In squash mode they also edit the composed commit message
     (pre-filled from the session goal + bulleted commit subjects +
     `Co-authored-by` trailers for every contributor).
3. They click "Run locally" (which copies a one-line `jamsesh finalize-run
   <plan-id>` command to clipboard).
4. The portal exposes the same plan via `GET /finalize-plan` so the binary
   can fetch it on demand.
5. They run `jamsesh finalize-run <plan-id>` in their local source-repo
   checkout:
   - The binary fetches the session commits — prefers the user's local
     session checkout (filesystem path tracked at join time); falls back to
     fetching from the portal via HTTPS with an ephemeral token if no local
     checkout is present.
   - It creates the target branch from the session base.
   - In squash mode: `git cherry-pick --no-commit` for each curated commit,
     then one `git commit` with the composed message (author = the human
     running finalize-run, `Co-authored-by` trailers for every contributor).
   - In preserve mode: `git cherry-pick` per commit, preserving authors.
   - Conflicts surface in their local editor.
   - Their normal Claude Code instance (separate from any jamsesh-bound
     CC) helps resolve conflicts with full project context. The user drives
     `git cherry-pick --continue` themselves; re-invoking `jamsesh
     finalize-run` detects mid-pick state and reports what remains.
6. Once the branch is clean: `git push origin <target-branch>`.
7. The human opens whatever PR/MR/issue/announcement they want, on
   whatever forge their team uses. That part is outside jamsesh.
8. After successful push, the human optionally marks the session as
   finalized in the portal (closes it, freezes refs read-only).

## Status awareness in CC

Humans can check session state from CC at any time:

- `/jamsesh:jam` (status) — tell the agent "show session status". The agent
  runs `jamsesh status` and surfaces tree summary, peers, scope, mode,
  unresolved comments addressed to this user, and open conflicts.
- The SessionStart context (auto-injected at session start) shows the full
  current state.
- The UserPromptSubmit digest (auto-injected at every turn) shows what's
  changed since last turn.

The portal UI is the rich-visual surface; CC is the textual at-a-glance
surface.

## Flow: creating your first org

A user has authenticated but has no org memberships yet (the Home surface
shows the empty state).

1. The Home surface (`/`) renders a "Name your org" form.
2. The user enters a name and submits. The SPA calls `POST /api/orgs` with
   `{ name }`. The server derives the slug from the name and creates the org
   with the caller as creator.
3. On success the client calls `auth.addOrg(...)` to append the new org to
   the local auth state (role `creator`), then navigates to
   `/orgs/{new-id}/sessions`.
4. The user is now inside their org and can create a session (see
   **Flow: creating a session**).

If the call fails, an inline error is shown and the form stays open for
correction. No page reload is needed.

## Portal UI surfaces

The portal UI has these primary surfaces. (Concrete designs land in
`.mockups/screens/` as built.)

- **Anonymous visitor at `/`** — what unauthenticated visitors see at the
  root path is controlled by the deploy-time `JAMSESH_LANDING_VARIANT` flag
  (exposed to the SPA via `GET /api/portal/info`, fetched at bootstrap into
  the `portalInfo` rune store). Three variants:
  - `project` — renders `ProjectLanding.svelte` in-place at `/`. The
    Swiss/ITS design (strict 12-col grid, numbered sections 01 HERO / 02
    SCHEMATIC / 03 METHOD, colophon). Used by jamsesh.dev. Mockup:
    `.mockups/screens/feature-portal-visitor-entry-pages/option-1.html`.
  - `auto` (default) — if the deploy has `playground_enabled: true`,
    redirects anonymous `/` to `/playground` (the existing
    `PlaygroundLanding.svelte`). If playground is disabled, redirects to
    `/login`. This variant maximises playground discoverability with zero
    extra config.
  - `login` — redirects anonymous `/` to `/login` unconditionally.
    Available for self-hosters who want auth-only entry even when playground
    is enabled.

  Authenticated visitors at `/` are unaffected by this flag — they always
  see `Home.svelte` (the org picker / org creation flow). The `portalInfo`
  gate is anonymous-only. If the `/api/portal/info` fetch fails at
  bootstrap, the SPA falls back to `login` variant (safe bounce to `/login`)
  and logs a console warning.

- **Home (post-auth landing at `/`)** — the SPA's first non-org-scoped
  surface after sign-in. Renders one of three states driven by `auth.orgs`
  (populated from `GET /api/me`'s `orgs[]`): `null` (loading spinner);
  empty array (welcome heading + inline "name your org" form that calls
  `POST /api/orgs` and navigates the new creator into
  `/orgs/{id}/sessions`); single entry (auto-routes immediately to
  `/orgs/{id}/sessions`, no UI rendered); two or more entries (workspace
  picker with role badges per org + inline "create another org" form).
  Router name: `home` (`frontend/src/lib/router.svelte.ts`).
  Component: `frontend/src/lib/screens/Home.svelte`.
- **Session list** — sessions visible to the user, grouped by status
  (active, finalizing, ended).
- **Session view** — the main work surface for a single session:
  - Tree pane (the git DAG, colored by author, with mode badges)
  - Artifact pane (file viewer at the selected commit, with inline comments
    and a comment composer)
  - Activity feed (chronological events: commits, comments, conflicts)
  - Presence panel (who's online, their refs, their current commits)
  - Session header (goal, scope, default mode, member count)
- **Comment composer** — overlay on the artifact pane for posting
  comments with addressing.
- **Finalize view** — appears when finalize is initiated. Curation interface
  plus the generated cherry-pick plan.
- **Settings** — per-session: invitees, scope (widen), default mode,
  finalize/abandon controls (creator only).
- **Admin** — org-level: members, invitations, billing (hosted),
  configuration (self-host).
- **Playground landing (`/playground`)** — public, no auth required;
  inline create-session form (optional name + goal + nickname). On
  submit, navigates to the new session view with an anonymous bearer in
  the in-memory `playgroundContext`. Component:
  `frontend/src/lib/screens/PlaygroundLanding.svelte`.
- **Joiner picker (`/playground/s/{id}/join`)** — open-join page;
  `JoinerForm` collects a nickname (pre-filled from the wordlist) and
  POSTs to the join endpoint; `JoinerOutcome` renders the post-join
  state (success → navigate to session; 409 full / 410 ended → branch).
  Components: `frontend/src/lib/screens/JoinerPicker.svelte`,
  `frontend/src/lib/components/{JoinerForm,JoinerOutcome}.svelte`.
- **Session tombstone (`/playground/s/{id}/ended`)** — post-destruction
  view for ended playground sessions; shows reason (`idle_timeout` /
  `hard_cap` / `manual`) and ended-at timestamp. Component:
  `frontend/src/lib/screens/SessionTombstone.svelte`.

Session-view chrome present when a session has playground identity:

- **`CountdownBadge`** — persistent badge in the session header showing
  whichever of `idle_timeout_at` or `hard_cap_at` is closer.
- **`DestructionWarningBanner`** — banner that surfaces in the last
  5 min before a destruction deadline (idle or hard-cap), with a
  reason-aware message.
- **`PlaygroundChip`** — marker on session-list rows so playground
  sessions are visually distinct from durable ones.

## Notification surfaces

Notification routing is intentionally minimal in v1. Participants discover
what's happening by:

- The portal UI's live activity feed when they're looking at it.
- The digest injected into each agent turn (the agent surfaces relevant
  pings in its response).
- Browser tab badge when something addressed to them lands while the tab is
  in the background.

Email notifications and webhook integrations are deferred to a later release.

## What we explicitly don't do in the UI

- **No real-time text editing.** The artifact pane is read-only. Changes
  happen via commits.
- **No live cursors.** Presence shows which commit each user is on, not
  what they're typing.
- **No voice/video.** Use whatever your team already uses.
- **No automatic PR opening.** The human pushes and opens the PR themselves
  on their own forge.
- **No issue tracker, wiki, or project management.** Jamsesh is jam-scoped.

These could land in later versions; they're not v1.
