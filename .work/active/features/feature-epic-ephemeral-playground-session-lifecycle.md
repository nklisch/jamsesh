---
id: feature-epic-ephemeral-playground-session-lifecycle
kind: feature
stage: drafting
tags: [portal, playground]
parent: epic-ephemeral-playground
depends_on: [feature-epic-ephemeral-playground-cli-first-creation, feature-epic-ephemeral-playground-anon-bearer, feature-epic-ephemeral-playground-reserved-org]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Playground session lifecycle

## Brief

The playground capability core — adds everything between "the substrate
exists" and "users can run a playground session end-to-end." Builds on
the three wave-1 foundation features: extends `jamsesh new` with the
`--playground` flag (and aliases `jamsesh playground new`); adds the
unauthenticated session-creation REST endpoint that targets the reserved
playground org; issues anonymous bearers for the creator and each joiner
via the wave-1 token-service primitive; mints pronounceable 2-word
handles server-side with a small wordlist (256x256 ≈ 65k combinations,
session-scoped uniqueness check, re-roll on collision).

The destruction-trigger logic is the highest-risk piece of this feature:
a background sweep loop (single goroutine in the portal, configurable
interval default 60s) walks active playground sessions and ends any that
have crossed either the idle threshold or the hard-cap threshold. End
performs: revoke all bearers (set `oauth_tokens.revoked_at`), delete
`comments` and `conflict_events` for the session, delete the `sessions`
row (FK cascades `session_members`, `events`, `presence`), delete the
bare repo from disk under `<storage>/orgs/playground/sessions/<id>.git`.

Abuse caps wire in:
- Per-IP session-create rate limit at the REST handler (per-IP token
  bucket, defaults from `reserved-org` env vars)
- Per-session push-throughput cap at `pre-receive` (rolling window byte
  count, rejects when exceeded with `409 playground.throughput_exceeded`)
- Per-session total content-size cap at `pre-receive` (denies pushes
  when the session's accumulated object-storage usage would exceed the
  cap, with `409 playground.size_exceeded`)
- Max concurrent participants per session at the join handler

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 2 critical path** — the single feature in its
  wave; both wave-3 features (`portal-ui`, `plugin-skills`) depend on
  its endpoints existing.

## Foundation references
- `docs/SPEC.md` § Lifecycle § Ephemeral playground sessions — concrete
  defaults for `IDLE_TIMEOUT`, `HARD_CAP`, and abuse caps are pinned in
  this feature's design pass and rolled forward into SPEC.md from
  placeholders to actual numbers
- `docs/ARCHITECTURE.md` § Components — destruction worker is a new
  background-goroutine subsystem inside the portal binary; its
  responsibility line is added to ARCHITECTURE.md by this feature
- `docs/SECURITY.md` — abuse-vector threat model + per-cap rationale
  added by this feature's design pass
- OpenAPI spec — new REST routes for unauthenticated session create
  (`POST /api/playground/sessions`), joiner accept
  (`POST /api/playground/sessions/{id}/join`), and bearer rotation if
  needed; component schemas reused from the existing session shapes

## Mockups
- Inherits parent epic flow:
  `.mockups/flows/playground-onboarding/index.html`
- This feature's user-visible shapes (countdown badges, warning banners,
  destruction confirmation page) are covered in flow steps 03, 06, 07a,
  7b, 7c. No additional feature-tier mocks.
