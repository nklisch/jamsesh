# Conflicts — full resolution flow

> Load this when you have an open `conflict.detected` event in your
> digest, or you've gone isolated → sync and expect a batch of them.
> Background context lives in the `jamsesh` skill (sections 1, 2, 6).

A conflict event fires when the auto-merger attempted a three-way merge
of your commit into the current `draft` tip and the merge failed. The
event is addressed to you and to the owner of the conflicting draft
commit.

## Event payload fields

- `id` — the conflict event id. You need this for the
  `Resolves-Conflict` trailer.
- `failing_commit` — your commit that didn't merge cleanly.
- `draft_tip` — the `draft` SHA at the time of the merge attempt.
- `merge_base` — common ancestor SHA between `failing_commit` and
  `draft_tip`.
- `files` — conflicted files and (where computable) the conflicted
  line ranges.
- `addressees` — usually `[you, owner-of-conflicting-draft-commit]`.

## Step-by-step

### 1. Fetch

```bash
git fetch
```

Session lifecycle hooks fetch at turn start, but if you're mid-turn,
fetch again before rebasing.

### 2. Verify you see the current draft tip

```bash
git log --oneline jam/draft
```

The local tracking ref name may vary by client setup; run
`/jamsesh:status` if unsure.

### 3. Rebase your ref onto draft

```bash
git rebase jam/draft
```

You'll hit the same conflicts the auto-merger did. Resolve them as
normal:

- Inspect both sides of each conflict marker.
- Apply the synthesis that's correct for both the peer's intent and
  your intent. When unsure, prefer preserving the peer's structural
  changes and re-applying your semantic change on top.
- `git add <resolved-file>`
- `git rebase --continue`

### 4. Trailer the resolution

After the rebase finishes, add `Resolves-Conflict` to the topmost
commit that constitutes the resolution. Two equivalent options:

**Option A — amend the last commit:**

```bash
git commit --amend --trailer "Resolves-Conflict: <event-id>"
```

(Keep the existing `Jam-Session`, `Jam-Turn`, `Jam-Author` trailers
intact.)

**Option B — create a dedicated resolution commit:**

```bash
git commit --allow-empty -m "resolve conflict <event-id>

Resolves-Conflict: <event-id>
Jam-Session: <session-id>
Jam-Turn: <turn>
Jam-Author: <your-handle>
"
```

### 5. Push happens automatically

The `PostToolUse` hook detects the commit and pushes. The auto-merger
retries the merge. On success, the conflict event closes and peers'
next digests no longer carry it.

## When conflicts recur

If `draft` advanced again during your rebase (a peer's commit landed
between your fetch and push), a fresh `conflict.detected` event will
appear in your next digest. Repeat from step 1.

## When conflicts pile up

If you're getting hammered with conflict events on a contentious file:

- Consider `/jamsesh:mode isolated` to stop the auto-merger from
  trying every commit. Do a larger resolution pass, then switch back.
- Coordinate with the driving human or peers via `post_comment` on the
  conflicting files (`kind: action-request`).
- For multi-party conflicts where each side has independent value,
  sometimes the right move is to let two refs co-exist until finalize
  curates the final sequence.

## Edge cases

- **Conflict on a file you didn't touch.** Means a peer's recent
  commit and a hook-applied autoformat (or similar) collided. Same
  flow — rebase, accept the peer's version, push.
- **Conflict event but rebase says "already up to date".** Either the
  conflict resolved itself (a peer also rebased) or the event is
  stale. Run `/jamsesh:status` to verify. If still open, post a
  `question` comment addressed to the driving human.
- **Rebase produces zero conflicts but the auto-merger still reports
  one.** Usually means `draft` advanced during your rebase. Fetch
  again, rebase again, then trailer.
