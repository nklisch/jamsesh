---
name: jamsesh
description: Operational primer for jamsesh — dual-mode model, commit trailers, addressed comments, conflict resolution, digest reading, MCP tools
auto-load: true
triggers:
  - "jamsesh"
  - "jam session"
  - "git commit"
  - "session"
---

# Jamsesh — Operational Primer

This skill loads automatically when the jamsesh plugin is active. It contains
everything you need to operate correctly in a session. For design rationale,
see `docs/VISION.md`; for full schema references, see `docs/PROTOCOL.md` and
`docs/UX.md`.

---

## 1. What jamsesh does

Jamsesh is a multi-agent collaboration substrate built on real git. A small
team of humans each drive their own Claude Code instance against a shared
session repository hosted by a portal server. Every change you make is a real
commit on your own branch ref. A server-side auto-merger continuously
integrates non-conflicting work from all sync-mode participants into a shared
`draft` ref, so the artifact converges live as the jam proceeds. When the jam
ends, a human curates the final commit sequence in the portal and runs a
generated cherry-pick script against their local source repo. No shared
agent, no bespoke version control — it's git all the way down, with a thin
social layer on top.

You are one participant. Your commits appear on your ref. Peers' commits
appear on their refs. The digest injected before each of your turns tells you
what changed since you last acted.

---

## 2. Dual mode

Every ref in `jam/<session>/<user>/*` has a mode: **sync** or **isolated**.

**Sync mode** — the default for most sessions. Every commit you push is
immediately tried against the current `draft` tip by the auto-merger. If the
merge succeeds, `draft` advances and all peers start from a richer shared
base on their next turn. If it conflicts, a `conflict.detected` event fires,
addressed to you and the owner of the conflicting draft commit.

**Isolated mode** — private exploration. The auto-merger ignores your commits.
You accumulate work without disturbing `draft` or generating conflict events
for peers. Use isolated mode when:
- You are exploring a risky or speculative approach that may be discarded.
- The human driving you explicitly asks for a branch that should not
  auto-merge until reviewed.
- You expect high conflict probability and want to resolve them in one batch
  rather than turn-by-turn.

**Switching modes** — use the `/jamsesh:mode` skill or call `bin/jamsesh mode
sync|isolated` directly. When you switch from isolated to sync, all commits
accumulated since you went isolated will be pushed and tried by the
auto-merger. Expect conflicts proportional to how far `draft` has drifted.

**Never push directly.** The `PreToolUse` hook intercepts `git push` and
returns `deny`. The `PostToolUse` hook handles pushing after every commit.
Do not attempt to work around this.

---

## 3. Commit trailers

Every commit you create in a session **must** carry three trailers. The
portal's `pre-receive` hook rejects pushes that omit any of them.

**Required on every session commit:**

```
Jam-Session: <session-id>
Jam-Turn: <turn-number>
Jam-Author: <your-user-handle>
```

The `SessionStart` and `UserPromptSubmit` hooks inject the correct values for
`Jam-Session` and `Jam-Author` into your context at the start of each session
and each turn. The turn number increments each time you start a new turn
(each human prompt). Use the value provided in the injected context. If you
are unsure, call `bin/jamsesh status` to retrieve current session state.

**Format your `git commit` messages with trailers in the footer**, separated
from the body by a blank line:

```
feat(auth): add magic-link token validation

Validates the one-time token against the database before exchange,
clearing the token atomically on success.

Jam-Session: sess_01j9abc123
Jam-Turn: 7
Jam-Author: alice
```

**Optional trailers recognized by the system:**

`Resolves-Conflict: <conflict-event-id>` — Include this when your commit is
a resolution of a reported conflict. The auto-merger reads this trailer on
push; if the three-way merge now succeeds, the conflict event is
automatically closed. This is the only way to close a conflict event
programmatically. The conflict event id comes from the `conflict.detected`
event payload in your digest (field `id`).

`Auto-Merger: true` — Set on commits the auto-merger creates. You will see
these in the git log; do not set this yourself.

`Source-Commit: <sha>` — Set by the auto-merger on merge commits, pointing
at the source commit being integrated. Informational; do not set manually.

`Jam-Auto-Commit: true` — Set on commits created automatically by the `Stop`
hook (when it auto-commits a dirty working tree at turn end). You do not set
this; the hook does.

---

## 4. Addressed comments

Comments are how participants direct attention across agent and human
boundaries. An agent posts a comment via the `post_comment` MCP tool (see
Section 7). A human posts via the portal UI. Both end up in the same data
model and both flow into the digest of addressed recipients.

**Addressing targets** (the `addressed_to` field):

- `@<user-handle>` — addressed to that human. Their agents will see it in
  their digests on their next turns.
- `@<user-handle>/<branch>` — addressed to a specific agent instance binding
  on that user's named ref. Use when the comment is about something specific
  to one ref rather than the user in general.
- `@all-agents` — broadcast to all agent instances in the session.
- `@all-humans` — broadcast to all human participants.
- `@everyone` — broadcast to all participants (agents and humans).
- `@auto-merger` — informational addressing to the auto-merger. The
  auto-merger does not act on comments; use this for audit annotations.
- Omit `addressed_to` entirely — the comment is an `fyi` to the session at
  large, visible in the activity feed but not injected into any specific
  digest.

**Comment kinds** (the `kind` field):

- `question` — you need an answer before proceeding. Use sparingly; each
  question creates an obligation for the recipient.
- `suggestion` — you have a recommendation but the recipient can ignore it.
  Good for non-blocking style or approach notes.
- `action-request` — you are asking the recipient to do something specific.
  Stronger than suggestion; the recipient should acknowledge it (resolve the
  comment when acted on).
- `fyi` — informational. No response expected. Default when `kind` is omitted.

**When to post a comment:**

Post a comment when you want a peer (human or agent) to know something that
will not be obvious from the commit diff alone. Avoid excessive commenting —
every addressed comment occupies context in the recipient's next turn. Use
`action-request` only when you genuinely need the recipient to act. Use
`question` only when you are blocked. Prefer `suggestion` or `fyi` for
everything else.

**Resolving comments:** call `resolve_comment` MCP tool when you have acted
on an `action-request` or answered a `question` addressed to you. Resolved
comments are removed from future digests.

---

## 5. Reading the digest

The digest is injected into your context by the `UserPromptSubmit` hook
before every turn. It covers everything that happened since your last turn
(`last_seen_seq` cursor). Structure:

**Commits from peers** — git log excerpts grouped by ref: commit SHA,
author, subject, files changed. Read these to understand what your peers have
produced. The diff is not included; use standard `git show <sha>` or `git
diff <sha>^!` to inspect changes that are relevant to your current task.

**Comments addressed to you** — the full comment body, anchor (commit, file,
line range), kind, and sender. Act on `action-request` comments before
proceeding with your current task unless the human driving you says otherwise.
Resolve the comment after acting.

**Conflict events addressed to you** — see Section 6.

**Mode changes** — if any ref switched modes since your last turn, it
appears here. Relevant when you are about to merge or depend on a peer's ref.

**Session state summary** — current `draft` tip SHA, your bound ref, your
current mode, open conflicts addressed to you (count). Use `/jamsesh:status`
for a fuller picture on demand.

If the digest carries nothing relevant to your current task, proceed normally.
Do not manufacture acknowledgment of empty sections.

---

## 6. Conflict resolution flow

A conflict event means the auto-merger attempted to merge a commit from your
ref into `draft` and encountered a three-way merge conflict. The event is
addressed to you and to the owner of the conflicting draft commit.

**Step 1 — Recognize.** The digest includes a `conflict.detected` entry. It
tells you: the conflict event id, the commit that failed to merge, the
current `draft` tip at the time of the attempt, the common ancestor SHA, and
the conflicted files and line ranges.

**Step 2 — Fetch.** Run `git fetch` (the `UserPromptSubmit` hook has already
done this if you are at turn start, but run it again if you are mid-turn).
Verify you can see the current `draft` tip: `git log --oneline draft` (or
whatever the local tracking ref is named).

**Step 3 — Rebase onto draft.** Rebase your ref onto the current `draft` tip:

```bash
git rebase jam/draft
```

During rebase, you will encounter the same conflicts the auto-merger found.
Resolve them normally (read both sides, apply the correct synthesis, `git add`,
`git rebase --continue`). The conflicted files and line ranges from the event
payload tell you exactly where to look.

**Step 4 — Commit with the resolution trailer.** After a successful rebase,
your rebased commits already carry the required session trailers. Add one
more trailer to the topmost commit (or a new explicit resolution commit):

```
Resolves-Conflict: <conflict-event-id>
```

Use `git commit --amend` to add the trailer to the last commit, or create a
new "resolve conflict" commit with the trailer in its footer.

**Step 5 — The hook pushes.** The `PostToolUse` hook detects your commit and
pushes. The auto-merger retries the merge. If it succeeds, the conflict event
closes automatically and peers' next digests no longer show the open conflict.

If a conflict recurs because `draft` advanced again during your rebase, repeat
from Step 2. Complex multi-party conflicts may require coordinating with the
human driving you or using `/jamsesh:mode isolated` to accumulate a larger
resolution batch before switching back to sync.

---

## 7. MCP tools

Four tools are available via the `jamsesh` MCP server. All require
`session_id` — read the current session id from the injected context or call
`bin/jamsesh status`.

**`post_comment`** — post a comment on a commit, file, or line range.

```
post_comment(
  session_id: "sess_01j9abc123",
  commit_sha: "a1b2c3d",
  file_path: "internal/auth/token.go",    // optional
  line_range: {start: 42, end: 55},        // optional
  body: "This token validation should also check expiry.",
  addressed_to: "@bob/feature-auth",       // optional
  kind: "suggestion"                        // optional, defaults to fyi
)
```

Use when: you want a peer to see something specific in your commit; you are
leaving a note for the human driving you (address to `@<your-user>`); you
want the activity feed to carry a non-code signal.

**`resolve_comment`** — mark a comment resolved.

```
resolve_comment(
  session_id: "sess_01j9abc123",
  comment_id: "cmt_77xyz",
  resolution_note: "Applied in commit a1b2c3d."  // optional
)
```

Use when: you have acted on an `action-request`; you have answered a
`question`; a comment is stale and no longer applies.

**`fork`** — server-side ref manipulation to create or move a ref.

```
fork(
  session_id: "sess_01j9abc123",
  target_commit_sha: "a1b2c3d",
  target_ref: "feature-x",          // optional; creates jam/<session>/<user>/feature-x
  mode: "isolated"                   // optional; defaults to session default
)
```

Use when: the human asks you to branch from a specific peer commit; you need
a new ref under your namespace without touching the current bound ref.

**`query_session_state`** — on-demand query for state not in the digest.

```
query_session_state(
  session_id: "sess_01j9abc123",
  include: ["unresolved_comments", "open_conflicts", "draft_tip"],
  filter: {comments_addressed_to: "@alice/main"}
)
```

Use when: you need state that the digest didn't include (e.g., all unresolved
comments in the session, not just those addressed to you); you want to verify
the current `draft_tip` SHA before rebasing.

---

## 8. Pointers

This skill is an operational primer — it covers the mechanics you need in
every session but does not attempt to be a complete reference.

For deeper context:

- **`docs/VISION.md`** — why jamsesh exists, the problem it solves, and what
  it deliberately is not.
- **`docs/PROTOCOL.md`** — full schema for commit trailers, comment schema,
  conflict event schema, WebSocket event types, all four MCP tool signatures,
  lifecycle hook contracts, local state layout, and HTTP error codes.
- **`docs/UX.md`** — every user-facing flow in detail: creating a session,
  joining, posting comments from both CC and the portal UI, forking from a
  peer, switching modes, and the finalize flow.

If something in this skill conflicts with those docs, the docs win — they are
the canonical source. Report the discrepancy to the session's human.
