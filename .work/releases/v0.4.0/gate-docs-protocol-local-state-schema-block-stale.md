---
id: gate-docs-protocol-local-state-schema-block-stale
kind: story
stage: done
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# PROTOCOL.md `Local state schema` block omits the per-session bearer storage that landed in this bundle

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/PROTOCOL.md:392-408`
- Code: `cmd/jamsesh/state/state.go` (`writeSessionState`), `cmd/jamsesh/state/migrate.go`

## Current doc text
> ```
> ${CLAUDE_PLUGIN_DATA}/
> ├── token                 user OAuth token (mode 0600, plaintext or system keychain reference)
> ├── refresh_token         OAuth refresh token (mode 0600)
> ├── portal_url            configured portal URL (one line)
> └── sessions/
>     └── <session-id>/
>         ├── ref           the (user/branch) this CC instance is bound to
>         ├── instance_id   the CC session_id this binding belongs to
>         ├── last_seen_seq portal event log cursor
>         └── refs/
>             └── <peer>    last seen SHA for each peer ref (cursor for digest git-log diffs)
> ```

## Reality
`docs/ARCHITECTURE.md:160-177` was rolled forward (per `story-foundation-doc-drift-bearer-storage-architecture`) to show `sessions/<id>/{token, instance_id, ref, org_id, account_id, last_seen_seq}` and the root `token` as a `MIGRATED_TO_PER_SESSION` stub. PROTOCOL.md's parallel block was not rolled forward and still shows the pre-migration layout (no per-session `token`, no `org_id`, no `account_id`) plus the `refs/<peer>` subdirectory that `writeSessionState` does not create.

## Required edit
Replace the tree block at `docs/PROTOCOL.md:396-408` with the layout already canonical in `docs/ARCHITECTURE.md:162-177` (per-session `token`, `instance_id`, `ref`, `org_id`, `account_id`, `last_seen_seq`; clarify root `token` as the account-wide token / migration stub; drop the obsolete `refs/<peer>` subtree).

## Implementation notes

Replaced the `${CLAUDE_PLUGIN_DATA}/` tree block at `docs/PROTOCOL.md:396-410` with the canonical layout from `docs/ARCHITECTURE.md:162-177`: per-session `token`, `instance_id`, `ref`, `org_id`, `account_id`, `last_seen_seq`; root `token` annotated as the account-wide token / `MIGRATED_TO_PER_SESSION` stub; dropped the obsolete `refs/<peer>` subtree.

Verified: Foundation docs are markdown — no build/test step. Edits preserve the rolling-foundation discipline (no "previously" prose, no "in v1.x" notes; assertions replaced in place).
