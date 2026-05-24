---
id: story-foundation-doc-drift-bearer-storage-architecture
kind: story
stage: implementing
tags: [documentation, plugin]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Roll docs/ARCHITECTURE.md forward to reflect per-session bearer storage

## Source

Review finding (blocker) on
`story-epic-ephemeral-playground-plugin-skills-bearer-storage`. The
unified per-session token storage landed in commit `db90b5e`, materially
changing the contract of `${CLAUDE_PLUGIN_DATA}/token` (it may now be a
`MIGRATED_TO_PER_SESSION` stub rather than the canonical access bearer)
and adding `${CLAUDE_PLUGIN_DATA}/sessions/<id>/token` as the new
authoritative per-session path. Per the rolling-foundation principle,
foundation docs must describe the system as it is now.

## What's drifted

In `docs/ARCHITECTURE.md`:

1. **Lines 124–127** (`jamsesh auth` description) state the OAuth flow
   "writes the token to `${CLAUDE_PLUGIN_DATA}/token`." This is still
   correct for the pre-binding auth flow, but the assertion is now
   incomplete — needs a sentence about the per-session fan-out on next
   startup.

2. **Lines 129–133** (`jamsesh mcp-headers`) state it "Reads the user's
   OAuth token from `${CLAUDE_PLUGIN_DATA}/token`". After the migration
   lands, mcp-headers reads from
   `${CLAUDE_PLUGIN_DATA}/sessions/<id>/token` when a session is bound
   and only falls back to the legacy path otherwise. The assertion as
   written is now misleading.

3. **Lines 135–147** (Local state layout diagram) shows the legacy
   single-token layout. The diagram must be extended to show:
   - `token` clarified as the legacy pre-binding token / migration stub
   - `sessions/<session-id>/token` (per-session bearer, mode 0600)
   - `sessions/<session-id>/instance_id` (CC instance binding)
   - other session-scoped files written by `jamsesh new` (`ref`,
     `org_id`, `account_id`, `last_seen_seq`)

## Acceptance criteria

- [ ] `docs/ARCHITECTURE.md` `jamsesh auth` description mentions the
      per-session fan-out behavior triggered on next startup
- [ ] `docs/ARCHITECTURE.md` `jamsesh mcp-headers` description correctly
      describes the per-session-first / legacy-fallback read path
- [ ] Local state layout diagram (lines 135–147) shows the full
      per-session file tree as it exists post-migration
- [ ] No "previously…" or "in v1.x…" language — describe the system as
      it is now (rolling-foundation rule)

## Notes

- This was flagged as a Blocker on `bearer-storage` review, but the
  story's explicit acceptance criteria were all met. The blocker is the
  cross-cutting docs alignment that the story did not include.
- Trivial-sized change (one doc, ~15 lines edited). Single-stride.
