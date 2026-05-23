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

## Design decisions

Locked at `--only-questions` time. Feature-design Phase 5 inherits these
as fixed input.

- **Joiner overflow** (session at max participants when URL clicked):
  hard error with retry hint. `POST /api/playground/sessions/{id}/join`
  returns `409 playground.session_full` with a `{ retry_after_seconds,
  alternative: "/playground" }` body; the joiner UI renders a friendly
  "this session is full (5/5)" page, a "try again in a few minutes"
  note, and a CTA to start their own playground. No spectator role,
  no waitlist substrate, no new UI tier — fits the strict-ephemeral,
  low-ceremony philosophy.

- **Idle activity definition**: substantive collaboration only. The
  idle timer resets on (1) any `git push` that lands a commit, (2)
  any `POST /comments`, (3) any `POST /finalize-attempt`. Presence
  WS pings, page loads, tree-view selection events, and other UI
  interactions do NOT reset the timer. Catches real activity; doesn't
  reward zombie browser tabs or a CC plugin background-fetching from
  a closed session. Implementation: the destruction-sweep worker reads
  `last_substantive_activity_at` (new column on `sessions`, updated
  by the three event paths above) rather than `events.created_at`.

- **Bearer-issuance API shape**: single atomic endpoint.
  `POST /api/playground/sessions/{id}/join` accepts
  `{ nickname }` in the body, validates capacity, runs the
  full join transaction (mint anonymous account row, mint
  `session_members` row, mint bearer with TTL synced to session
  hard-cap), returns
  `{ bearer, nickname, session: <session_summary> }` in one
  round-trip. The nickname-suggest UX is client-side: the SPA
  pre-fills the suggestion locally (the JOIN endpoint runs server-
  side collision retry if the proposed nickname is taken). No
  separate suggest/reserve endpoint — keeps the substrate small
  and removes the suggest/confirm race window.

- **Pronounceable-handle wordlist source**: hardcoded in the portal
  binary via Go's `embed` package. Two `.txt` files in
  `internal/portal/playground/wordlist/` — `adjectives.txt`
  (~256 entries) and `animals.txt` (~256 entries). Curated at PR
  time and reviewed for offensive content, accessibility, and
  pronunciation. Combined space ≈ 65k handles; per-session uniqueness
  check (small) plus collision-retry on the JOIN transaction handles
  duplicates. Zero deployment friction, deterministic across portal
  pods, refresh requires a release (rare and appropriate).
