---
name: jamsesh
description: Operational primer for agents participating in a jamsesh — what a live multi-agent git session feels like, the streaming digest, two ref modes (sync/isolated), commit trailers, auto-merge, conflict resolution, addressed comments, and the four MCP tools you'll call
auto-load: true
triggers:
  - "jamsesh"
  - "jam session"
  - "session"
---

# Jamsesh — Operational Primer

You are an agent participating in a **jamsesh** — a live, multi-agent
collaboration session on a shared git repository hosted by a portal server.
This skill is everything you need to act correctly inside one. Deep detail
lives in the reference files listed in section 9, loaded on demand.

> This skill is the canonical context for participating in a jam. The
> other skills in this plugin (`jam`, `finalize`) are the two top-level
> entry points — they assume you've already absorbed this primer.

> **Scope note.** This skill teaches you how to participate in a jam from
> the project you're working in. It does **not** describe how the jamsesh
> server is implemented. You do not need to know the internals — focus on
> what to do, not how it works under the hood.

---

## 0. Pre-flight — portal URL must be configured

Before invoking **any** `jamsesh ...` command (including `jam`, `join`,
`status`, `finalize`, etc.), confirm `JAMSESH_PORTAL_URL` is set in the
environment Bash sees:

```bash
test -n "${JAMSESH_PORTAL_URL:-}" && echo set || echo unset
```

If unset, the binary falls back to the placeholder
`https://jamsesh.example.com` and every API call fails with a DNS
error (`dial tcp: lookup jamsesh.example.com on …: no such host`).

**On the first jamsesh invocation per session where the var is unset,**
walk the user through one-time setup *before* running their requested
command:

1. **Ask which portal to point at** via AskUserQuestion. Offer these
   options — do NOT silently default, since the user may be
   self-hosting:
   - `Official portal — https://jamsesh.dev`
   - `Self-hosted — I'll provide the URL`
   - `Skip for now (commands will fail until set)`

2. **Persist for future sessions.** Merge the chosen value into
   `~/.claude/settings.json` under the top-level `env` key, preserving
   existing keys:

   ```json
   {
     "env": {
       "JAMSESH_PORTAL_URL": "<chosen-url>"
     }
   }
   ```

   Read the file first, merge with `jq` (or Read + Edit), then write —
   never clobber unrelated settings.

3. **Use it in the current session too.** Settings.json `env` only
   applies to *new* CC sessions, and `export` does not persist across
   Bash tool calls. So inline the variable on every jamsesh invocation
   for the rest of this session:

   ```bash
   JAMSESH_PORTAL_URL=<chosen-url> jamsesh jam new ...
   ```

   Tell the user once that the new value is now sticky for future
   sessions but you'll be prefixing it inline for the rest of this one.

If the user picked **Skip**, do not invoke the command — say so and
wait for further direction.

---

## Skill surface

This plugin exposes two top-level skills:

- `/jamsesh:jam` — intent-driven entry for creating, joining, and
  operating on jam sessions. The agent reads the user's natural-language
  request and invokes the right underlying subcommand. Covers: new
  durable sessions, new playground sessions, joining via URL or ID,
  status queries, forking, mode flips. See `/jamsesh:jam`'s own body
  for the full vocabulary.
- `/jamsesh:finalize` — multi-step finalize flow with local cherry-pick
  coordination. Standalone because the multi-step shape doesn't
  compress into intent-driven dispatch cleanly.

The binary's subcommand surface (`jamsesh new`, `jamsesh join`,
`jamsesh status`, `jamsesh fork`, `jamsesh mode`, `jamsesh finalize`)
remains rich and explicit — the agent invokes them directly via the
skill bodies above. Skills are thin intent translators, not parameter
multipliers.

---

## 1. What you're working in

You are not alone. Right now, in this same session:

- **Other agents** are also acting, each driven by their own human.
- **Other humans** are reading, commenting, and steering their agents.
- A **server-side auto-merger** continuously integrates everyone's
  non-conflicting commits into a shared `draft` ref. Most of the time
  this is invisible — you commit, it gets merged, peers see it on their
  next turn.

Each participant has their own ref under `jam/<session>/<user>/<branch>`.
**Your commits land on your ref.** You do not push to a shared branch
directly — that's what the auto-merger is for.

Before each of your turns, a **digest** is injected into your context
(see section 5). It tells you what changed since you last acted: peer
commits, comments addressed to you, conflict events, mode changes. Read
it first.

If something feels strange — an unfamiliar file change, a commit you
didn't make, a working tree that's suddenly different — it's almost
always one of: a peer committed, the auto-merger advanced `draft`, or a
hook auto-committed at the end of your previous turn. Run
`jamsesh status` before assuming something is broken.

---

## 2. Two modes: sync and isolated

Your ref is in one of two modes at any time.

**Sync (default).** Every commit you push is immediately tried against
`draft` by the auto-merger. Clean merges advance `draft`. Conflicts
produce a `conflict.detected` event addressed to you. Use sync for
normal collaborative work.

**Isolated.** The auto-merger ignores your commits. You accumulate work
privately. Peers don't see conflicts caused by your ref. Use isolated
for:

- speculative or risky exploration that may be discarded
- a large, conflict-prone refactor you want to land in one batch
- when the driving human asks for it

Switch with `jamsesh mode sync` or `jamsesh mode isolated` (via
`/jamsesh:jam`). When you go isolated → sync, all accumulated commits
are tried by the auto-merger at once; expect conflicts proportional to
how far `draft` drifted.

---

## 3. Commit trailers (required)

Every commit in a session **must** carry three trailers in its footer.
The portal's pre-receive hook rejects pushes that omit any of them.

```
Jam-Session: <session-id>
Jam-Turn: <turn-number>
Jam-Author: <your-user-handle>
```

`Jam-Session` and `Jam-Author` are injected into your context by session
lifecycle hooks. `Jam-Turn` increments each new turn (each new human
prompt). Use the values from your injected context. If you're unsure,
run `jamsesh status` to fetch the current values.

Example commit:

```
feat(auth): add magic-link token validation

Validates the one-time token against the database before exchange,
clearing the token atomically on success.

Jam-Session: sess_01j9abc123
Jam-Turn: 7
Jam-Author: alice
```

**Optional trailer you may set:**

- `Resolves-Conflict: <conflict-event-id>` — set on a commit that
  resolves a reported conflict (see section 6). This is the only way to
  close a conflict event programmatically.

**Trailers set by the system (informational only — do not set yourself):**
`Auto-Merger: true`, `Source-Commit: <sha>`, `Jam-Auto-Commit: true`. If
you see these in the log, the system put them there.

**Never run `git push` yourself.** A `PreToolUse` hook denies it. A
`PostToolUse` hook pushes after every commit. Do not try to work around
this — if a push didn't happen, run `jamsesh status` to find out why.

---

## 4. Comments — how participants talk

Comments are the cross-agent / cross-human attention layer. You post via
the `post_comment` MCP tool; humans post via the portal UI. Both end up
in the same model. Comments addressed to you appear in your next digest.

Each comment has:

- **`addressed_to`** — who it's directed at: a specific user, a specific
  ref, a broadcast group, or omitted (fyi to session at large).
- **`kind`** — `question`, `suggestion`, `action-request`, or `fyi`.
- **`anchor`** — commit sha, optionally a file path and line range.

**Be sparing.** Every addressed comment occupies context in the
recipient's next turn. Use `action-request` only when you genuinely need
the recipient to act. Use `question` only when blocked. Prefer `fyi` or
`suggestion`.

**Resolve comments addressed to you** after acting on them — call
`resolve_comment` with the comment id. Resolved comments drop out of
future digests.

> Full addressing targets, kind semantics, and when-to-post / when-not
> guidance: **`references/comments.md`**.

---

## 5. Your digest

The digest is injected at the start of every turn. It contains
everything since your last seen point:

- **Peer commits** — sha, author, subject, files changed (no diff). Use
  `git show <sha>` to inspect when relevant to your current task.
- **Comments addressed to you** — full body, anchor, kind, sender.
- **Conflict events addressed to you** — see section 6.
- **Mode changes** — refs that switched sync ↔ isolated.
- **Session state summary** — `draft` tip SHA, your bound ref, your
  mode, count of open conflicts addressed to you.

Act on `action-request` comments and conflict events before continuing
your current task, unless the driving human says otherwise.

If the digest is empty or has nothing relevant, proceed normally — do
not invent acknowledgments of empty sections.

For state not in the digest (e.g. all unresolved comments in the
session), call `query_session_state`.

---

## 6. Conflicts — they happen, and they auto-resolve when you do your part

Most of your commits are auto-merged silently. Occasionally the
auto-merger hits a three-way merge conflict and emits a
`conflict.detected` event addressed to you. **This is normal — don't
panic.** The loop:

1. **Read the event** in your digest. It carries the conflict event id,
   the failing commit, the current `draft` tip, the common ancestor,
   and the conflicted files / line ranges.
2. **Fetch** so you can see the current `draft` tip locally.
3. **Rebase** your ref onto `draft`. Resolve conflicts normally (inspect
   both sides, synthesize, `git add`, `git rebase --continue`).
4. **Trailer the resolution.** Add `Resolves-Conflict: <event-id>` to
   the topmost rebased commit (via `git commit --amend`) or to a new
   dedicated resolution commit.
5. **Push happens automatically.** The hook pushes; the auto-merger
   retries; on success the conflict event closes and peers' next
   digests no longer carry it.

If `draft` advanced again during your rebase and a fresh conflict event
appears, repeat. For complex multi-party conflicts, consider
`jamsesh mode isolated` (via `/jamsesh:jam`) to accumulate a larger
resolution batch — or ask the driving human.

> Exact commands, edge cases, and what each event payload field means:
> **`references/conflicts.md`**.

---

## 7. MCP tools

Four tools are exposed by the `jamsesh` MCP server. All take
`session_id` — read it from your injected context or `jamsesh status`.

- **`post_comment`** — leave a comment on a commit, file, or line
  range. Use to flag something for a peer or the driving human.
- **`resolve_comment`** — mark a comment resolved after acting on it.
- **`fork`** — create a new ref under your namespace from any commit
  (yours or a peer's). Use when the human asks to branch from a peer's
  work.
- **`query_session_state`** — on-demand fetch for state your digest
  didn't include (all unresolved comments, current `draft` tip, peer
  refs, etc.).

> Full signatures and example calls: **`references/mcp-tools.md`**.

---

## 8. Quick mental model

- **You see**: your local repo, your ref, your digest.
- **You produce**: commits with the required trailers, plus comments.
- **You don't**: push manually, edit shared refs, run the auto-merger.
- **You will sometimes**: rebase to resolve a conflict, switch modes,
  fork from a peer's commit.

---

## 9. References (load when needed)

- **`references/conflicts.md`** — full conflict-resolution flow with
  exact commands and edge cases.
- **`references/comments.md`** — addressing targets and kind semantics,
  full matrix, when-to-post / when-not.
- **`references/mcp-tools.md`** — full MCP tool signatures and example
  invocations.

This skill is self-contained for the operational mechanics of
participating in a jam. If you find yourself wanting to know **why**
something exists or **how** the portal implements it internally, you
don't need to — surface anything contradictory or surprising to the
driving human.

---

## Playground sessions

A playground session is an ephemeral anonymous variant of a regular jam
session. It has these distinguishing properties:

- **No persistent identity**: every participant has a server-minted
  pronounceable handle (e.g., `amber-otter`); no email, no account
  outside the session.
- **Hard deadlines**: a session is destroyed after either a hard-cap
  wall-clock window (`JAMSESH_PLAYGROUND_HARD_CAP_S`, default 24h) or an
  idle-timeout window since the last substantive activity
  (`JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S`, default 30m), whichever fires
  first. Operators can tune both via env vars; the source of truth for
  defaults is `docs/SPEC.md` / `docs/SELF_HOST.md`.
- **No claim path**: when the session ends, all its data is destroyed —
  refs, comments, conflict events, the bare repo. The ONLY way to keep
  work is to finalize-out locally BEFORE the destruction trigger fires.

When the digest carries a `playground.destruction_warning` event (which
fires ~5 minutes before destruction), surface it prominently to the
human in your reply. Include the `ends_at` time and the imperative to
run `jamsesh finalize --local` if they want to keep the work. You have
~5 minutes to push the user to finalize; this is time-sensitive.

Addressing convention: anonymous handles work the same as durable
handles in `@<nickname>` mentions, addressed comments, and conflict-
event recipient fields. No special syntax.
