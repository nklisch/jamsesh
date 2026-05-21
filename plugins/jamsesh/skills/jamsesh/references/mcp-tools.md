# MCP tools — full signatures

> Load this when you're about to call one of the jamsesh MCP tools and
> want to verify the parameter shape. Background context lives in the
> `jamsesh` skill (section 7).

Exposed by the `jamsesh` MCP server. All take `session_id` — read it
from your injected context or `/jamsesh:status`.

## `post_comment`

Post a comment on a commit, optionally narrowed to a file and line
range.

```
post_comment(
  session_id: "sess_01j9abc123",
  commit_sha: "a1b2c3d",
  file_path: "internal/auth/token.go",   // optional
  line_range: {start: 42, end: 55},       // optional
  body: "This token validation should also check expiry.",
  addressed_to: "@bob/feature-auth",      // optional — see comments.md
  kind: "suggestion"                       // optional — defaults to fyi
)
```

Use to: flag something specific for a peer or the driving human;
leave a note on a commit that needs follow-up; signal a mode change
to peers.

Returns: `comment_id` (use this with `resolve_comment` later).

## `resolve_comment`

Mark a comment resolved. Removes it from future digests.

```
resolve_comment(
  session_id: "sess_01j9abc123",
  comment_id: "cmt_77xyz",
  resolution_note: "Applied in commit a1b2c3d."  // optional but recommended
)
```

Use to: close an `action-request` you've acted on; answer a
`question` addressed to you (put the answer in `resolution_note`);
clear a stale comment that no longer applies.

## `fork`

Create or move a ref under your namespace, optionally setting its
mode. Server-side ref manipulation — does not touch your working
tree.

```
fork(
  session_id: "sess_01j9abc123",
  target_commit_sha: "a1b2c3d",
  target_ref: "feature-x",           // optional; creates jam/<session>/<user>/feature-x
  mode: "isolated"                    // optional; defaults to session default
)
```

Use when: the driving human asks you to branch off a peer's commit;
you want a new ref under your namespace without disturbing your
current bound ref.

Equivalent to `/jamsesh:fork`. Prefer the MCP tool when you're
already in a tool-use flow.

Returns: the created `ref_name` and its initial `mode`.

## `query_session_state`

On-demand state query for things not in your digest.

```
query_session_state(
  session_id: "sess_01j9abc123",
  include: ["unresolved_comments", "open_conflicts", "draft_tip"],
  filter: {comments_addressed_to: "@alice/main"}  // optional
)
```

Common `include` values:

- `unresolved_comments` — all unresolved comments (not just yours).
- `open_conflicts` — all open conflict events in the session.
- `draft_tip` — current `draft` ref SHA.
- `peer_refs` — list of peer refs with their owners and modes.
- `mode_states` — mode (sync/isolated) for every ref in the session.

Use when: you need state your digest didn't include (e.g. all
unresolved comments in the session, not just yours); you want to
verify the current `draft` tip before rebasing; you're triaging
session-wide health on the driving human's request.
