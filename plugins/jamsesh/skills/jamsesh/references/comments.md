# Comments — full addressing and kind reference

> Load this when you're about to post a comment that needs deliberate
> addressing, or when triaging a comment in your digest. Background
> context lives in the `jamsesh` skill (section 4).

Comments are the cross-agent / cross-human attention layer. Posted via
the `post_comment` MCP tool (agents) or the portal UI (humans). Both
end up in the same model.

## Addressing targets (`addressed_to`)

- `@<user-handle>` — addressed to that human. All of their agent
  instances see it in their next digest.
- `@<user-handle>/<branch>` — addressed to a specific agent instance
  bound to that user's named ref. Use when the comment is ref-specific
  rather than user-general.
- `@all-agents` — broadcast to every agent instance in the session.
- `@all-humans` — broadcast to every human participant.
- `@everyone` — broadcast to all participants (agents and humans).
- `@auto-merger` — informational annotation for audit. The auto-merger
  does not act on comments; this is just a paper trail.
- *omit* — `fyi` to the session at large. Appears in the activity feed
  but is not injected into any specific recipient's digest.

## Kinds (`kind`)

- `question` — you need an answer before proceeding. Creates an
  obligation on the recipient. Use sparingly — agents that ask too
  many questions burn peer context.
- `suggestion` — you have a recommendation; the recipient may ignore
  it. Good for non-blocking style or approach notes.
- `action-request` — you're asking the recipient to do something
  specific. Stronger than suggestion; the recipient should
  `resolve_comment` it when acted on.
- `fyi` — informational, no response expected. Default when `kind` is
  omitted.

## When to post

Post when a peer (human or agent) needs to know something that isn't
obvious from the commit diff:

- "I refactored the auth layer; rebase before continuing your token work."
- "The token field name changed — your endpoint will need an update."
- "I'm going isolated for an experiment, ignore my ref until I switch back."
- "Found a latent bug while reading your file — not in your scope but
  flagging for whoever picks it up."

## When not to post

Don't:

- post commentary that the diff already says (e.g. "I added a helper")
- post running self-narration ("I'm now going to think about X")
- broadcast `action-request` to `@all-agents` unless every agent really
  must act
- leave stale comments around — resolve them when no longer applicable

## Resolving

Call `resolve_comment` with the comment id when:

- you've acted on an `action-request` addressed to you
- you've answered a `question` addressed to you (best practice:
  include the answer in `resolution_note`)
- the comment is stale and no longer applies (note why in
  `resolution_note`)

Resolved comments drop out of future digests.

## Triage cheat sheet

When your digest shows comments addressed to you, work through them in
this order:

1. **`action-request`** — act, then resolve. If you can't act in this
   turn, post a brief `fyi` acknowledging and resolve the original.
2. **`question`** — answer, then resolve with the answer in
   `resolution_note`.
3. **`suggestion`** — apply if it improves the change, ignore if not.
   No resolution required, but resolving with a note is polite.
4. **`fyi`** — read, no action required.
