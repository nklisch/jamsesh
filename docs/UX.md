# User Experience

How humans interact with jamsesh.

## Interaction model

Two surfaces, one workflow.

**Claude Code** is where the work happens. Humans drive their agents in CC
exactly as they do for any other CC session. The jamsesh plugin adds slash
commands (`/jamsesh:join`, `/jamsesh:status`, etc.), surfaces session context
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

A user wants to start a new jam against their team's repo.

1. From a checkout of the source repo, they run `/jamsesh:create`.
2. The plugin prompts (or accepts as args):
   - Session name (e.g., "Auth design refresh")
   - Goal / manifest (one paragraph: what this session is producing and why)
   - Writable scope (required path globs, e.g., `docs/auth/**`, `specs/auth/**`)
   - Default mode (sync or isolated)
   - Optional: invitees (emails or org members)
3. The local binary calls portal API to create the session.
4. The local binary pushes the current source-repo `HEAD` to
   `jam/<session>/base` on the session remote.
5. Plugin returns a join URL the creator can share with collaborators.
6. The creator's CC instance is already bound to `jam/<session>/<user>/main`
   in sync mode — they can start prompting immediately.

## Flow: joining a session

A collaborator received an invite or a join URL.

1. From a checkout of the source repo, they run `/jamsesh:join <session-id-or-url>`.
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
6. They start prompting their agent normally.

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
6. In CC, run `/jamsesh:sync` or the agent's next prompt — the local checkout
   is updated to reflect the fork.

**From CC:**

1. `/jamsesh:fork <commit-sha> [--as <branch>] [--mode sync|isolated]`
2. The local binary calls the portal `fork` MCP tool.
3. The local checkout is reset to the new parent automatically.

## Flow: switching mode (sync ↔ isolated)

A human realizes their current line of work would be better as a private
exploration (or that an isolated branch is ready to rejoin the trunk).

**To go isolated:**

1. `/jamsesh:mode isolated` in CC.
2. Or click the mode badge on your ref in the portal tree view, choose
   "isolated."
3. Future commits on this ref are not auto-merged. Conflict events stop
   firing for this ref. The tree visually detaches the ref from the trunk.

**To rejoin sync:**

1. `/jamsesh:mode sync`.
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

- `/jamsesh:status` — tree summary, peers, scope, mode, unresolved comments
  addressed to this user, open conflicts addressed to this user.
- The SessionStart context (auto-injected at session start) shows the full
  current state.
- The UserPromptSubmit digest (auto-injected at every turn) shows what's
  changed since last turn.

The portal UI is the rich-visual surface; CC is the textual at-a-glance
surface.

## Portal UI surfaces

The portal UI has these primary surfaces. (Concrete designs land in
`.mockups/screens/` as built.)

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
