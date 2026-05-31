---
id: gate-patterns-v0.5.0
kind: story
stage: done
tags: [patterns]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: patterns
created: 2026-05-31
updated: 2026-05-31
---

# Patterns extracted for v0.5.0

## New patterns codified
- `opaque-token-hash-at-rest` — opaque credentials return raw random tokens to callers but store and look up only SHA-256 hex hashes in `token_hash` fields.
- `fragment-secret-handoff` — secret browser handoff links carry raw tokens only in URL fragments and SPA screens scrub the fragment immediately after reading it.
- `session-scoped-portal-client` — session-bound CLI portal calls construct `portalclient.Client{BaseURL, SessionID}` and wire refresh only for durable credentials.
- `nullable-timestamp-transition-guard` — terminal transitions set nullable `*_at` fields through `UPDATE ... WHERE *_at IS NULL`, using affected rows or `RETURNING` when caller must know who won.

## Inconsistencies flagged
- `openapi-fetch-middleware-client` needs a documented exception for intentionally unauthenticated exchange endpoints such as `ResumeExchange.svelte`.

## Rejected pattern candidates
- `git-http-extraheader-credential` was not codified because `gate-security-git-extraheader-argv-credential-leak` flagged the same structure as a High-severity credential exposure. Do not teach this as a reusable pattern until that security item is resolved.

## Pattern files written
- `.agents/skills/patterns/opaque-token-hash-at-rest.md`
- `.agents/skills/patterns/fragment-secret-handoff.md`
- `.agents/skills/patterns/session-scoped-portal-client.md`
- `.agents/skills/patterns/nullable-timestamp-transition-guard.md`
- `.agents/skills/patterns/SKILL.md` (updated index)
- `.agents/rules/patterns.md` (generated hook-loaded digest)

