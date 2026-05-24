---
id: feature-epic-ephemeral-playground-session-lifecycle
kind: feature
stage: review
tags: [portal, playground]
parent: epic-ephemeral-playground
depends_on: [feature-epic-ephemeral-playground-cli-first-creation, feature-epic-ephemeral-playground-anon-bearer, feature-epic-ephemeral-playground-reserved-org]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

## Implementation summary (autopilot)

All 5 child stories advanced to `stage: review`:

- `story-...-session-lifecycle-rest-endpoints` — wordlist (~239 adjectives + ~182 animals) + 4 REST handlers + OpenAPI extensions + schema (sessions columns + tombstones table)
- `story-...-session-lifecycle-destruction` — Worker + Destruction with 8-step idempotent cascade; wired into cmd/portal/main.go shutdown via ctx
- `story-...-session-lifecycle-abuse-caps` — wired existing ratelimit infra; CheckPlaygroundCaps in pre-receive (option B: total disk-walk size); activity-reset in 3 substantive-event call-sites (post-receive, comments, finalize)
- `story-...-session-lifecycle-cli-playground-flag` — `jamsesh new --playground` via new PostJSONAnon helper; pushBaseRefWithBearer variant; mutual-exclusion with --org
- `story-...-session-lifecycle-docs` — SPEC.md ephemeral-playground subsection with concrete defaults; SECURITY.md abuse-model section

**Cross-cutting deviations**:
- Bearer issuance split outside the session WithTx (3-step sequence: session TX → bearer issuance → member insert) to avoid SQLite WAL deadlock; partial failure leaves orphaned session that destruction sweep cleans up
- Activity-reset wired in 3 call-sites best-effort (log warning on failure, never fail the substantive event)
- Local `playgroundOrgID` constants per package to avoid import cycles (githttp / comments / sessions all need the check)
- Content-size check via filesystem walk of bare repo (no DB row tracking)
- One pre-existing fixture bug parked: `bug-mcpheaders-stale-fixture-migrated-stub`

**Verification status**: `go build ./...` clean, `go vet ./...` clean, `go test ./...` all packages pass (except the parked mcpheaders fixture, which is a pre-existing flake from the bearer-storage migration).

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

## Substrate confirmed during design

- **Rate-limiting middleware exists** at `internal/portal/ratelimit/`
  with `NewStore(Config{PerMinute: N})` returning a per-key counter
  store. Story 3 (abuse caps) wires existing infra, not new.
- **Background-goroutine pattern** in `cmd/portal/main.go` is
  `go func() { ... }() + wg.Add(N) + wg.Wait()` for graceful
  shutdown coordination. The destruction worker (Story 2) follows
  this pattern.
- **Pre-receive** uses typed `Validate(ctx, ValidateInput)
  (ValidateResult, error)` at `internal/portal/prereceive/validate.go`.
  Cleanly extendable with playground-specific throughput / content-
  size checks.
- **Anon-bearer feature primitives** (wave-1, already designed) provide
  `tokens.Service.IssueAnonymousSessionBearer(ctx, sessionID, nickname, ttl)`
  and `store.RevokeBearersForSession(...)`. This feature consumes both.
- **Reserved-org constants** (wave-1, already designed) provide
  `playground.ReservedOrgID = "org_playground"` for org-scoped queries.

## Architectural choice

**Five-story decomposition** — REST surface (handle gen + endpoints),
destruction subsystem (worker + routine), abuse caps, CLI extension,
docs. The work decomposes cleanly along these axes because each chunk
has its own test surface and most can run in parallel after wave-1
implements.

Why this shape over alternatives:
- **Monolithic feature implementation**: too much surface for one
  agent; lots of cross-file concurrency hazards (destruction routine
  mutates state the REST endpoints also touch); harder to gate-review
  per concern. 5 stories let the implement-orchestrator parallelize.
- **Per-endpoint stories**: too granular — POST sessions / POST join /
  GET status / GET tombstones share substrate (handle gen, anon-bearer
  issuance, session-member insert). Splitting them creates inter-story
  coordination overhead with no parallelism gain.
- **Single "playground" package vs spread across existing packages**:
  prefer the `internal/portal/playground/` package (already provisioned
  by the wave-1 reserved-org feature) as the home for everything
  playground-specific. Anything truly cross-cutting (pre-receive
  extension, REST handler registration) lives in its existing package
  with a playground-aware branch.

## Implementation units

5 stories — each is one of the chunks below. Story files live at
`.work/active/stories/story-epic-ephemeral-playground-session-lifecycle-<slug>.md`.

### Story 1: Handle gen + REST endpoints
**Files** (new unless noted):
- `internal/portal/playground/wordlist/adjectives.txt`
- `internal/portal/playground/wordlist/animals.txt`
- `internal/portal/playground/wordlist/wordlist.go` — embed + Pick
- `internal/portal/playground/handler.go` — REST handlers
- `internal/portal/playground/handler_test.go`
- `docs/openapi.yaml` — extend with the new routes + schemas
- `internal/portal/router/router.go` (modify) — mount playground routes
- `internal/api/openapi/*.gen.go` (regenerated by `make generate`)

#### Wordlist + handle generator

```go
// internal/portal/playground/wordlist/wordlist.go
package wordlist

import (
    _ "embed"
    "math/rand/v2"
    "strings"
)

//go:embed adjectives.txt
var adjectivesRaw string

//go:embed animals.txt
var animalsRaw string

var (
    adjectives = splitNonEmpty(adjectivesRaw)
    animals    = splitNonEmpty(animalsRaw)
)

// Pick returns a fresh pronounceable handle like "amber-otter".
// Random selection uses math/rand/v2 (crypto-strength not required —
// this is a display handle, not a credential; per-session uniqueness
// is enforced at the join transaction).
func Pick() string {
    a := adjectives[rand.IntN(len(adjectives))]
    n := animals[rand.IntN(len(animals))]
    return a + "-" + n
}

func splitNonEmpty(raw string) []string {
    out := make([]string, 0, 256)
    for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
        if line = strings.TrimSpace(line); line != "" {
            out = append(out, line)
        }
    }
    return out
}
```

Wordlist content: curated 256x256 entries. Adjectives skew towards
calm/positive sentiment (`amber`, `quiet`, `swift`, `gentle`,
`bright`, `steady`); animals are common enough to be recognizable but
varied (`otter`, `fox`, `heron`, `lynx`, `wren`, etc). No offensive
combinations — review during implementation.

#### REST handlers

```go
// internal/portal/playground/handler.go
package playground

import (
    "context"
    "errors"
    "net/http"
    "time"

    "<module>/internal/api/openapi"
    "<module>/internal/db/store"
    "<module>/internal/portal/playground/wordlist"
    "<module>/internal/portal/tokens"
)

type Handler struct {
    Store     store.Store
    Tokens    tokens.Service
    Storage   storage.Service // for bare-repo create
    Cfg       Config          // playground-specific knobs from main Config
    Clock     Clock           // per-package clock interface
    Logger    *slog.Logger
}

type Config struct {
    Enabled         bool
    IdleTimeout     time.Duration
    HardCap         time.Duration
    MaxParticipants int
    // (rate-limit / content-size caps are wired separately in Story 3)
}

// CreateSession implements POST /api/playground/sessions
// (no auth — anonymous; receives the creator's local-checkout HEAD push
// on the subsequent git-push to refs/heads/jam/<session-id>/base)
//
// Returns 503 playground.disabled if !Cfg.Enabled.
// Returns 201 with { session, bearer, nickname } on success.
func (h *Handler) CreateSession(ctx context.Context, req openapi.CreatePlaygroundSessionRequestObject) (openapi.CreatePlaygroundSessionResponseObject, error) {
    if !h.Cfg.Enabled {
        return openapi.CreatePlaygroundSession503JSONResponse{...}, nil
    }
    now := h.Clock.Now().UTC()
    sessionID := "sess_" + randID(12)
    nickname := h.uniqueHandle(ctx, sessionID) // collision-retry within session
    ttl := h.Cfg.HardCap

    // Single TX: session row + member row + bearer issuance
    var resp openapi.PlaygroundSessionCreated
    err := h.Store.WithTx(ctx, func(q store.Querier) error {
        _, err := q.CreateSession(ctx, store.CreateSessionParams{
            ID:                          sessionID,
            OrgID:                       playground.ReservedOrgID,
            Name:                        req.Body.Name, // optional; defaults to "playground-<short>"
            Goal:                        req.Body.Goal,
            WritableScope:               req.Body.Scope, // defaults to "**"
            DefaultMode:                 "sync",
            Status:                      "active",
            CreatedAt:                   now,
            LastSubstantiveActivityAt:   now,
            HardCapAt:                   now.Add(h.Cfg.HardCap),
            IdleTimeoutAt:               now.Add(h.Cfg.IdleTimeout),
        })
        if err != nil { return err }

        // Bearer issuance (also creates anonymous accounts row internally)
        rawToken, accountID, expiresAt, err := h.Tokens.IssueAnonymousSessionBearer(ctx, sessionID, nickname, ttl)
        if err != nil { return err }

        // Member row (creator role)
        if err := q.AddSessionMember(ctx, store.AddSessionMemberParams{
            OrgID: playground.ReservedOrgID,
            SessionID: sessionID,
            AccountID: accountID,
            Role: "creator",
            JoinedAt: now,
        }); err != nil { return err }

        resp = openapi.PlaygroundSessionCreated{
            Session: toAPISession(...),
            Bearer:  rawToken,
            ExpiresAt: expiresAt,
            Nickname: nickname,
        }
        return nil
    })
    if err != nil { return nil, err }

    // Bare-repo create happens after the TX commits (matches existing
    // CreateSession pattern in internal/portal/sessions/handler.go).
    if err := h.Storage.CreateRepo(ctx, playground.ReservedOrgID, sessionID); err != nil {
        // Rollback by marking the session as abandoned. The destruction
        // sweep will catch it within the next interval.
        h.Logger.Error("bare-repo create failed for playground session", "session_id", sessionID, "err", err)
        return nil, err
    }

    return openapi.CreatePlaygroundSession201JSONResponse(resp), nil
}

// JoinSession implements POST /api/playground/sessions/{id}/join
// Body: { nickname?: string } — optional; if absent, server picks one.
//
// Returns 503 if !Cfg.Enabled.
// Returns 404 if session doesn't exist or is destroyed.
// Returns 409 playground.session_full if at MaxParticipants cap.
// Returns 200 with { session, bearer, nickname, expiresAt }.
func (h *Handler) JoinSession(ctx context.Context, req openapi.JoinPlaygroundSessionRequestObject) (openapi.JoinPlaygroundSessionResponseObject, error) {
    if !h.Cfg.Enabled {
        return openapi.JoinPlaygroundSession503JSONResponse{...}, nil
    }

    sessionID := req.Id
    sess, err := h.Store.GetSession(ctx, store.GetSessionParams{OrgID: playground.ReservedOrgID, SessionID: sessionID})
    if errors.Is(err, store.ErrNoRows) {
        return openapi.JoinPlaygroundSession404JSONResponse{...}, nil
    }
    if err != nil { return nil, err }

    // Capacity check (read members count from session_members)
    count, err := h.Store.CountSessionMembers(ctx, store.CountSessionMembersParams{
        OrgID: playground.ReservedOrgID, SessionID: sessionID,
    })
    if err != nil { return nil, err }
    if int(count) >= h.Cfg.MaxParticipants {
        retryAfter := 60 // seconds — sane default; tunable later
        return openapi.JoinPlaygroundSession409JSONResponse{
            Error: "playground.session_full",
            Message: fmt.Sprintf("session is full (%d/%d)", count, h.Cfg.MaxParticipants),
            RetryAfterSeconds: &retryAfter,
        }, nil
    }

    // Nickname: client-suggested or server-suggested; collision-retry inside.
    nickname := strings.TrimSpace(req.Body.Nickname)
    if nickname == "" {
        nickname = wordlist.Pick()
    }
    nickname = h.uniqueHandle(ctx, sessionID, nickname) // retries with new suggestion on collision

    // TTL is the remaining hard-cap time
    remaining := time.Until(sess.HardCapAt)
    if remaining <= 0 {
        return openapi.JoinPlaygroundSession410JSONResponse{
            Error: "playground.session_ended",
            Message: "this session has ended",
        }, nil
    }

    rawToken, accountID, expiresAt, err := h.Tokens.IssueAnonymousSessionBearer(ctx, sessionID, nickname, remaining)
    if err != nil { return nil, err }

    err = h.Store.AddSessionMember(ctx, store.AddSessionMemberParams{
        OrgID: playground.ReservedOrgID, SessionID: sessionID,
        AccountID: accountID, Role: "member", JoinedAt: h.Clock.Now().UTC(),
    })
    if err != nil { return nil, err }

    return openapi.JoinPlaygroundSession200JSONResponse{
        Session:   toAPISession(sess),
        Bearer:    rawToken,
        ExpiresAt: expiresAt,
        Nickname:  nickname,
    }, nil
}

// uniqueHandle picks a wordlist handle and retries (max ~10 attempts)
// if it collides with an existing nickname in this session.
// If a candidate is passed in, it's tried first; subsequent retries
// pull fresh from wordlist.Pick().
func (h *Handler) uniqueHandle(ctx context.Context, sessionID string, candidates ...string) string {
    tried := make(map[string]bool, 16)
    for i := 0; i < 10; i++ {
        var nick string
        if i < len(candidates) {
            nick = candidates[i]
        } else {
            nick = wordlist.Pick()
        }
        if tried[nick] { continue }
        tried[nick] = true
        // Check: does any existing session_member.account_id resolve to an
        // account with display_name == nick within this session?
        taken, _ := h.Store.NicknameTakenInSession(ctx, store.NicknameTakenInSessionParams{
            OrgID: playground.ReservedOrgID, SessionID: sessionID, DisplayName: nick,
        })
        if !taken { return nick }
    }
    // Last resort: append random suffix to ensure uniqueness
    return wordlist.Pick() + "-" + randID(4)
}

// GetSession implements GET /api/playground/sessions/{id}
// Bearer-authenticated; the bearer's account must be a session member.
// Returns session summary including remaining time until destruction.
func (h *Handler) GetSession(ctx context.Context, req openapi.GetPlaygroundSessionRequestObject) (openapi.GetPlaygroundSessionResponseObject, error) {
    // ... standard: lookup, membership check, return summary
}

// GetTombstone implements GET /api/playground/sessions/{id}/tombstone
// Returns the destruction summary for a destroyed session (or 404 if
// the session is still active OR the tombstone TTL has elapsed).
// This is what the portal-ui post-destruction page reads from
// (Unit 3 in the portal-ui feature).
func (h *Handler) GetTombstone(ctx context.Context, req openapi.GetTombstoneRequestObject) (openapi.GetTombstoneResponseObject, error) {
    // ...
}
```

#### OpenAPI spec additions

`docs/openapi.yaml` gains 4 routes under `/api/playground/`:
- `POST /api/playground/sessions` → 201 PlaygroundSessionCreated / 503 / 400
- `POST /api/playground/sessions/{id}/join` → 200 PlaygroundJoinResult / 503 / 404 / 409 / 410
- `GET /api/playground/sessions/{id}` → 200 PlaygroundSessionSummary / 401 / 404
- `GET /api/playground/sessions/{id}/tombstone` → 200 PlaygroundTombstone / 404

Component schemas added: `PlaygroundSessionCreated`, `PlaygroundJoinResult`,
`PlaygroundSessionSummary`, `PlaygroundTombstone`, `CreatePlaygroundSessionRequest`,
`JoinPlaygroundSessionRequest`.

After spec edits: run `make generate` to regenerate Go server types
(both REST + WebSocket envelope schemas) and TS client types.

#### Schema additions

The `sessions` table grows two new columns to support destruction logic:
- `last_substantive_activity_at DATETIME NOT NULL` — updated on push/comment/finalize-attempt; read by destruction worker for idle check
- `hard_cap_at DATETIME` (nullable; populated only for playground sessions) — read by destruction worker
- `idle_timeout_at DATETIME` (nullable; populated only for playground sessions) — for symmetric storage; actually a derived field, could be computed as `last_substantive_activity_at + IdleTimeout`. Decision during implementation: store explicitly for cheaper sweep queries (avoids recomputing on every tick), at the cost of needing UPDATE on every activity reset. Choose explicit storage.

Plus a `tombstones` table:
```sql
CREATE TABLE tombstones (
    session_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL,
    members_count INTEGER NOT NULL,
    commits_count INTEGER NOT NULL,
    auto_merges_count INTEGER NOT NULL,
    duration_seconds INTEGER NOT NULL,
    end_reason TEXT NOT NULL,  -- 'idle' | 'hard_cap' | 'manual'
    ended_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL  -- TTL for the tombstone itself, default ended_at + 30 days
);
CREATE INDEX tombstones_expires_idx ON tombstones(expires_at);
```

Migration is one goose file per dialect.

#### Story 1 acceptance criteria

- [ ] Wordlist embed loads at init; `wordlist.Pick()` returns N-distinct
      handles across 1000 calls (~all distinct given 65k space)
- [ ] `POST /api/playground/sessions` with `JAMSESH_PLAYGROUND_ENABLED=false`:
      503 with `error: playground.disabled`
- [ ] `POST /api/playground/sessions` enabled, empty body: creates session
      with default name (`playground-<short>`), goal=`""`, scope=`"**"`,
      mode=`"sync"`; returns bearer + nickname + expires_at
- [ ] `POST /api/playground/sessions/{id}/join` at max participants:
      returns 409 `playground.session_full`
- [ ] `POST /api/playground/sessions/{id}/join` with explicit nickname:
      uses it if not taken; collision-retries with server suggestion if taken
- [ ] `POST /api/playground/sessions/{id}/join` after `hard_cap_at` past:
      returns 410 `playground.session_ended`
- [ ] `GET /api/playground/sessions/{id}/tombstone`: returns 404 while
      session is active; returns the summary after destruction (until
      the tombstone TTL elapses)
- [ ] All routes covered by handler tests using the existing
      `httptest.Server` + chi router pattern; tests run under both
      SQLite and Postgres via `stores(t)` harness
- [ ] OpenAPI spec validates cleanly; `make generate && git diff --exit-code`
      passes (generated types match committed)
- [ ] Bare-repo create failure rollback: if `CreateRepo` errors after
      session insert, session is marked abandoned (destruction sweep
      will clean up)

---

### Story 2: Destruction worker + routine
**Files**:
- `internal/portal/playground/worker.go` — background goroutine
- `internal/portal/playground/destruction.go` — the cascade routine
- `internal/portal/playground/worker_test.go`
- `internal/portal/playground/destruction_test.go`
- `cmd/portal/main.go` (modify) — start the worker
- `db/queries/{sqlite,postgres}/sessions.sql` (extend) —
  `ListExpiredPlaygroundSessions`, `RecordSessionTombstone`, etc.

#### Worker

```go
// internal/portal/playground/worker.go
type Worker struct {
    Store    store.Store
    Storage  storage.Service
    Cfg      Config
    Clock    Clock
    Interval time.Duration
    Logger   *slog.Logger
}

// Run loops until ctx is cancelled. Each tick:
//   1. Query for playground sessions where (now > hard_cap_at OR now > idle_timeout_at) AND status='active'
//   2. For each expired session, invoke Destruction.Destroy(ctx, sessionID, reason)
//   3. Sleep until next tick
//
// Graceful shutdown: ctx cancellation stops the loop; in-flight Destroy
// call completes before return.
func (w *Worker) Run(ctx context.Context) error {
    ticker := time.NewTicker(w.Interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            w.sweep(ctx)
        }
    }
}

func (w *Worker) sweep(ctx context.Context) {
    now := w.Clock.Now().UTC()
    expired, err := w.Store.ListExpiredPlaygroundSessions(ctx, store.ListExpiredPlaygroundSessionsParams{
        OrgID: playground.ReservedOrgID,
        Now:   now,
    })
    if err != nil {
        w.Logger.Error("playground sweep query failed", "err", err)
        return
    }
    for _, sess := range expired {
        reason := w.reasonFor(sess, now)
        if err := w.destruction.Destroy(ctx, sess.ID, reason); err != nil {
            w.Logger.Error("playground destroy failed", "session_id", sess.ID, "err", err)
            // Don't abort the loop — try the next session; this one
            // will be picked up again on the next tick.
        }
    }
}

func (w *Worker) reasonFor(sess store.Session, now time.Time) string {
    if !sess.HardCapAt.Valid { return "manual" } // shouldn't happen for playground
    if now.After(sess.HardCapAt.Time) { return "hard_cap" }
    return "idle"
}
```

#### Destruction routine

```go
// internal/portal/playground/destruction.go
type Destruction struct {
    Store   store.Store
    Storage storage.Service
    Clock   Clock
    Logger  *slog.Logger
    TombstoneTTL time.Duration // default 30 days
}

// Destroy executes the destruction cascade for a playground session.
// The sequence is non-transactional across step boundaries (the bare-
// repo delete is a filesystem op outside the DB TX); each step is
// idempotent so partial failures are safely retried by the next sweep.
//
// Steps:
//   1. Collect summary stats (members, commits, auto-merges, duration)
//      for the tombstone — BEFORE deleting anything
//   2. Collect anon-account-IDs from session_members (those with
//      accounts.is_anonymous = true) — BEFORE deleting anything
//   3. Insert tombstones row (idempotent: ON CONFLICT DO NOTHING)
//   4. Revoke all bearers for the session (UPDATE oauth_tokens
//      SET revoked_at = now WHERE session_id = ? AND revoked_at IS NULL)
//   5. Delete comments + conflict_events for the session
//   6. Delete the sessions row (CASCADE handles session_members,
//      events, presence; CASCADE on oauth_tokens.session_id deletes
//      the bearers too — the revoke in step 4 was defense-in-depth
//      in case the cascade fails)
//   7. Delete the collected anonymous accounts (separate DELETE since
//      session_members cascade already removed the membership link)
//   8. Remove the bare repo on disk via Storage.DeleteRepo
//
// Returns error only on hard failures (DB connection lost, etc).
// Per-step errors are logged but don't abort the cascade — partial
// completion is acceptable since the next sweep will retry from where
// we are now.
func (d *Destruction) Destroy(ctx context.Context, sessionID, reason string) error {
    // ... step-by-step implementation
}
```

#### `cmd/portal/main.go` wiring

```go
if cfg.PlaygroundEnabled {
    worker := &playground.Worker{
        Store:    s,
        Cfg:      pgCfg,
        Clock:    clock.Real{},
        Interval: time.Duration(cfg.PlaygroundDestructionSweepIntervalS) * time.Second,
        Logger:   logger,
    }
    wg.Add(1)
    go func() {
        defer wg.Done()
        if err := worker.Run(workerCtx); err != nil && !errors.Is(err, context.Canceled) {
            logger.Error("playground worker exited with error", "err", err)
        }
    }()
}
```

Where `workerCtx` is derived from the main shutdown context so the
worker stops on graceful shutdown along with HTTP draining and the
auto-merger.

#### Story 2 acceptance criteria

- [ ] Worker ticks at the configured interval (verified by injecting a
      manual clock + asserting tick fires on Clock.Now advancement)
- [ ] Worker correctly identifies expired sessions (hard_cap_at or
      idle_timeout_at past now) via the new query
- [ ] Destruction routine cascade: after Destroy(), session row gone,
      bearers revoked + deleted via FK, anonymous accounts deleted,
      bare repo deleted from disk
- [ ] Tombstones row inserted with correct summary stats
- [ ] Partial failure resilience: inject failure at each step via
      a test-double store; verify next tick completes the destruction
- [ ] Graceful shutdown: ctx cancel stops the worker within one
      `Interval` window
- [ ] Tombstones older than TombstoneTTL are purged by a separate
      sub-routine (or as part of the sweep — implementer's call)

---

### Story 3: Abuse caps
**Files**:
- `internal/portal/playground/ratelimit.go` — wires existing
  `internal/portal/ratelimit` against the playground create endpoint
- `internal/portal/prereceive/playground_caps.go` — throughput +
  content-size checks (extends existing Validator with playground branch)
- `internal/portal/router/router.go` (modify) — mount rate-limit
  middleware on `POST /api/playground/sessions`

#### Per-IP create rate limit

```go
// internal/portal/playground/ratelimit.go
package playground

import "<module>/internal/portal/ratelimit"

func NewCreateRateLimiter(cfg Config) *ratelimit.Store {
    // Convert per-hour cap to per-minute (per-hour / 60 rounded up,
    // since the existing ratelimit pkg uses PerMinute as its unit)
    perMinute := (cfg.CreatePerIPHour + 59) / 60
    if perMinute < 1 { perMinute = 1 }
    return ratelimit.NewStore(ratelimit.Config{PerMinute: perMinute})
}

// Middleware returns http.Handler that consults the store using
// RealIP() from chi/middleware as the key.
func RateLimitMiddleware(store *ratelimit.Store) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ip := chimiddleware.RealIP(r) // or similar — check existing helper
            ok, retryAfter := store.Allow(ip)
            if !ok {
                w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
                http.Error(w, `{"error":"rate_limited","message":"too many session-create attempts","retry_after_seconds":...}`, http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

Mount in `router.go` only on the playground create route, NOT on join
or status. The middleware is bypassed when `cfg.PlaygroundEnabled=false`.

#### Pre-receive throughput + content-size

```go
// internal/portal/prereceive/playground_caps.go
package prereceive

// CheckPlaygroundCaps runs additional validation when the session
// being pushed-to lives in the reserved playground org. Called from
// Validate() (line ~50 of validate.go) when org_id matches the
// playground org constant.
func (v *Validator) CheckPlaygroundCaps(ctx context.Context, in ValidateInput) error {
    if in.OrgID != playground.ReservedOrgID { return nil }
    // 1. Per-session push throughput (rolling 1-hour window)
    pushedBytes, err := v.Store.SumPushedBytesLastHour(ctx, in.SessionID)
    if err != nil { return err }
    if pushedBytes + in.PackfileSizeBytes > v.PlaygroundCfg.MaxContentBytes {
        return &PreReceiveError{Code: "playground.size_exceeded", Message: "..."}
    }
    // 2. Per-session accumulated content size (simpler: storage backend reports total)
    repoSize, err := v.Storage.RepoSizeBytes(ctx, in.OrgID, in.SessionID)
    if err != nil { return err }
    if repoSize + in.PackfileSizeBytes > v.PlaygroundCfg.MaxContentBytes {
        return &PreReceiveError{Code: "playground.size_exceeded", Message: "..."}
    }
    return nil
}
```

`Validator.Validate()` extended to call `CheckPlaygroundCaps` after
the existing ref/scope/trailer checks pass. If the call returns a
`PreReceiveError`, it propagates through to the git client as a
`remote: ERROR:` line per the existing pre-receive contract.

#### Activity-reset wiring

Whenever a "substantive" event lands (commit pushed, comment posted,
finalize-attempt POSTed), the corresponding handler updates the
session's `last_substantive_activity_at` to `now`:

```go
// In commit-arrival event (post-receive):
if isPlayground { _ = h.Store.ResetSessionIdleTimer(ctx, ResetParams{OrgID, SessionID, Now: now, IdleTimeout: cfg.IdleTimeout}) }

// In comment-create handler:
... same ...

// In finalize-attempt handler:
... same ...
```

`ResetSessionIdleTimer` updates both `last_substantive_activity_at`
AND `idle_timeout_at` (= now + IdleTimeout) in one UPDATE. This is
the read-cheap design that the destruction worker depends on.

#### Story 3 acceptance criteria

- [ ] Per-IP create rate limit: 4th create within an hour from same IP
      returns 429 with Retry-After header
- [ ] Rate limit per-IP isolation: createS from different IPs don't
      affect each other's counter
- [ ] Pre-receive throughput cap: a push that would exceed the per-
      session throughput is rejected with `remote: ERROR: playground.size_exceeded`
- [ ] Pre-receive on durable (non-playground) sessions: unchanged behavior
      (no playground cap checks fire)
- [ ] Substantive activity events reset the idle timer correctly
      (verified by post-event SELECT of last_substantive_activity_at)

---

### Story 4: CLI `--playground` flag extension
**Files**:
- `cmd/jamsesh/sessioncmd/new.go` (modify, originally created by
  wave-1 cli-first-creation) — add `--playground` flag handling

```go
// Extend the existing NewCommand from wave-1 cli-first-creation.
// Add a single flag:
&cli.BoolFlag{Name: "playground", Usage: "Create an ephemeral anonymous playground session (no auth required)"},

// In newAction, branch early:
if cmd.Bool("playground") {
    return newPlaygroundAction(ctx, cmd) // separate path; skips org picker, auth, creates via the playground endpoint
}
// ... else durable path as designed in wave-1 ...

func newPlaygroundAction(ctx context.Context, cmd *cli.Command) error {
    pc := buildPlaygroundClient() // no auth header; uses public portal URL
    req := openapi.CreatePlaygroundSessionRequest{
        Name:  cmd.String("name"),  // optional; server defaults
        Goal:  cmd.String("goal"),
        Scope: cmd.String("scope"), // defaults to "**"
    }
    resp, err := portalclient.PostJSON[openapi.PlaygroundSessionCreated](ctx, pc, "/api/playground/sessions", req)
    if err != nil { return err }

    // Write the bearer to per-session token storage immediately
    if err := state.WriteSessionToken(resp.Session.Id, resp.Bearer); err != nil { return err }

    // Same pushBaseRef helper from wave-1, but with the bearer
    // we just received (not the OAuth token from state.ReadToken)
    if err := pushBaseRefWithBearer(ctx, pc, resp.Session.Id, resp.Bearer); err != nil {
        // Per the locked decision: session stays live with base_sha NULL
        return wrapPushError(err, resp.Session, pc.BaseURL)
    }

    // Write session state (org_id="org_playground", account_id=resp.Session.Members[0].AccountId, etc.)
    if err := writeSessionState(resp.Session, ...); err != nil { return err }

    printPlaygroundSummary(resp, pc.BaseURL)
    return nil
}
```

#### Story 4 acceptance criteria

- [ ] `jamsesh new --playground` creates a session via the playground
      endpoint (no auth), pushes HEAD as base, writes per-session state,
      prints share URL + nickname + expires_at
- [ ] `jamsesh new --playground --name "demo"` passes the name through
- [ ] `jamsesh new --playground` (without --org): doesn't prompt for
      org (skips the picker entirely)
- [ ] Mutually-exclusive guard: `--playground --org foo` returns a
      clear error ("--playground and --org are mutually exclusive")
- [ ] Post-create push uses the just-received bearer, NOT the
      account-wide OAuth token (which may not exist for the user)

---

### Story 5: SPEC.md + SECURITY.md roll-forward
**Files**:
- `docs/SPEC.md` — fill in concrete defaults for the previously-empty
  values in the ephemeral-playground section
- `docs/SECURITY.md` — add the abuse-vector threat model

SPEC.md updates: replace placeholders like "exact policy TBD" with
the concrete defaults from this design — idle timeout 30m, hard cap
24h, max participants 5, per-IP create rate 3/hour, max content 50 MiB.

SECURITY.md additions: abuse threat model covering:
- Per-IP rate limit rationale (3/hour balances spam-prevention vs
  legitimate-experimentation)
- Content-size cap as both abuse-prevention and storage-cost guard
- Joiner overflow as DoS-prevention (5 cap prevents one bad-actor
  session from saturating a deployment)
- Anonymous bearer leak blast radius (covered separately in anon-bearer
  feature's SECURITY.md addition; cross-reference)

#### Story 5 acceptance criteria

- [ ] SPEC.md ephemeral-playground section has concrete defaults
- [ ] SECURITY.md has an "Abuse model for playground sessions" section
- [ ] Both docs read cleanly within existing prose (present-tense,
      no "previously" language)

---

## Implementation order

Stories 1 + 2 + 5 are independent — can run in parallel (orchestrator
wave 2a: 3 sub-agents, fits cap exactly).

Stories 3 + 4 depend on Story 1 (REST endpoints must exist before
abuse caps mount on them; CLI must call those endpoints) — wave 2b:
2 sub-agents.

So implementing this feature is 2 sub-waves, 3 + 2 agents.

## Risks

- **Cross-cutting pre-receive change risk**: extending the existing
  `Validator` with a playground branch adds conditional complexity to
  the most-trafficked validation path. Mitigation: the branch fires
  ONLY when `in.OrgID == playground.ReservedOrgID`; durable session
  pushes hit a fast-path `return nil` from `CheckPlaygroundCaps`.
  Test the durable path's regression with the existing pre-receive
  test suite (no test changes needed — the org-id branch is
  transparent to durable sessions).

- **Activity-reset miss = sessions die early**: if the
  `ResetSessionIdleTimer` call is missing from a substantive-event
  handler (e.g. post-receive forgets to call it), playground sessions
  die after 30 min of "real" activity (idle timer never resets).
  Mitigation: integration test that pushes a commit, waits 25 min
  (simulated via injected clock), pushes another commit, then waits
  past the original 30m mark — verifies session still alive because
  second push reset the timer.

- **Wordlist collision in 65k space**: with 5 participants per session
  cap, collision rate is ~5/65000 = 0.008% per join. Per-session retry
  (max 10 attempts) plus random-suffix fallback handles it. Worst case
  user sees a slightly-mangled handle (`amber-otter-a3f2`) for ~0.001%
  of joins. Acceptable.

- **TombstoneTTL collision with re-created session at same ID**: if
  the same session ID is re-used (shouldn't happen — IDs are random,
  but theoretical), a fresh session creation would collide with the
  old tombstone's primary key. Mitigation: tombstone insert uses
  INSERT ... ON CONFLICT DO NOTHING (idempotent); the new session row
  is a separate table. No actual collision possible.

- **Worker as single point of work**: one goroutine sweeps all
  playground sessions. Under load (many sessions expiring simultaneously)
  the worker serializes destructions. Mitigation: destruction is fast
  (<100ms typical), 60s sweep interval has slack for thousands of
  destructions per tick. If this becomes a problem in practice, add
  a worker-pool inside `sweep()` later.

- **Clustered-mode interaction**: under `JAMSESH_DEPLOY_MODE=clustered`,
  multiple portal pods run. The destruction worker runs on every pod.
  Concurrent destruction attempts on the same session row could race.
  Mitigation: rely on the PG-advisory-lock infrastructure already
  used for cross-pod coordination — wrap the per-session destruction
  in a per-session advisory lock acquired by the worker. If a different
  pod holds the lock, this worker skips and tries on the next tick.
  Single-instance deployments (the default) don't need this; the lock
  is a no-op when the lock manager is the `NoopManager` per the
  existing `LeaseManager` pattern.

- **Activity-reset under clustered**: same story — the
  `ResetSessionIdleTimer` UPDATE is a single SQL statement, naturally
  atomic. No special handling needed.
