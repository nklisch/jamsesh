# Architecture

How jamsesh is organized.

## System overview

```
┌─────────────────────────────────────────────────────────────┐
│                        Claude Code                          │
│                                                             │
│  ┌─────────────────┐  ┌──────────────────────────────────┐  │
│  │ Hooks call into │  │ MCP client (HTTPS) points at     │  │
│  │ plugins/jamsesh │  │ portal MCP endpoint with user    │  │
│  │ /bin/jamsesh    │  │ OAuth token via headersHelper    │  │
│  └────────┬────────┘  └─────────────┬────────────────────┘  │
└───────────┼─────────────────────────┼───────────────────────┘
            │                         │
            ▼                         ▼
┌──────────────────────────┐   ┌──────────────────────────┐
│  Local `jamsesh` binary  │   │                          │
│  (Go, in plugin's bin/)  │   │      Portal (Go)         │
│                          │   │                          │
│  • Hook subcommands      │   │  • REST API (HTTPS)      │
│  • Skill / slash command │   │  • MCP endpoint (HTTPS)  │
│    subcommands           │   │  • Git smart-HTTP        │
│  • Local git operations  │   │    (per-session bare     │
│  • OAuth + token storage │◄──┤     repos on disk)       │
│    in CLAUDE_PLUGIN_DATA │   │  • Auto-merger workers   │
│  • Talks portal API + git│   │  • Playground destroyer  │
└──────────────────────────┘   │  • WS gateway (UI)       │
                               │  • SQLite | Postgres     │
                               └──────────────────────────┘
                                            ▲
                                            │
                                 ┌──────────┴──────────┐
                                 │   Portal UI (web)   │
                                 │   WebSocket + REST  │
                                 └─────────────────────┘
```

The portal is one Go binary. The local client is one Go binary inside the
Claude Code plugin package. Claude Code talks to both via plugin hooks (stdio)
and MCP (HTTPS to portal).

## Components

### Portal

A single Go binary that bundles several subcomponents sharing one process and
one data store:

**REST API** — endpoints over HTTPS. Auth via the user's OAuth bearer token
issued at plugin OAuth-flow time. Every operation that touches session state
takes a `session_id` argument and is authorized server-side against the
authenticated user's session memberships. Routes are org-scoped per the
multi-tenancy invariant.

**MCP endpoint** — HTTPS-MCP server (`type: streamable-http`) exposing the
four jamsesh tools to Claude Code clients. Same Bearer-auth as the REST API.
Tool calls include `session_id` so the portal applies session-scoped permission
checks.

**Git smart-HTTP** — serves `git-upload-pack` (fetch) and `git-receive-pack`
(push) for the session bare repos hosted on disk under
`<storage>/orgs/<org-id>/sessions/<session-id>.git`. Spawns `git-upload-pack`
and `git-receive-pack` as subprocesses with `--stateless-rpc` to serve
smart-HTTP. Pre-receive validation runs in-process (Go) before the
receive-pack spawn — see `internal/portal/githttp/`. HTTP Basic auth is
enforced at the chi router before the git subprocess runs, using the user
OAuth token as password. Pre-receive validates pushed ref names against the
authenticated user's namespace, the writable scope, and required commit
trailers.

**Auto-merger workers** — background goroutines triggered by `post-receive`
events. Use `go-git` in-process to attempt three-way merges of incoming
sync-mode commits into the session's `draft` ref. Emit `merge.succeeded` or
`conflict.detected` events accordingly.

**Playground destruction worker** — single background goroutine (started
when `JAMSESH_PLAYGROUND_ENABLED=true`) that sweeps active playground
sessions on a configurable interval
(`JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S`, default 60s). For each session past its idle or hard-cap deadline, runs the
destruction cascade — record tombstone, revoke bearers, delete session rows
(FK cascades members + events + presence + bearers), delete anonymous
accounts, remove the bare repo from disk. Idempotent across steps;
partial-failure resumption on the next tick. Periodic tombstone-TTL purge
runs every 60th tick.

**WebSocket gateway** — pushes events (commits, comments, conflicts, presence
changes, mode changes) to connected portal UI clients. Per-session
subscriptions.

**Data store** — SQLite by default, Postgres for scale. sqlc-generated query
packages. Stores accounts, sessions (metadata, goal, scope, mode), members,
OAuth tokens (refresh tokens + revocations), comments (with addressing),
conflict events, presence snapshots, event log.

### The `jamsesh` binary

Distributed in the Claude Code plugin's `bin/` directory, automatically added
to the Bash tool's PATH. Single Go binary with the following subcommand
surface:

**Hook subcommands** — invoked by CC's hook system, called with JSON on stdin,
returning JSON on stdout:

- `jamsesh hook session-start` — emits `additionalContext` describing the
  session goal, writable scope, current draft tip, peer ref tips, the user's
  refs and modes, and unresolved addressed comments.
- `jamsesh hook user-prompt-submit` — `git fetch` from session remote, calls
  `GET /api/orgs/{orgID}/sessions/{sessionID}/digest?since=<seq>` on the portal, formats the
  combined output as `additionalContext`, advances local `last_seen` cursors.
- `jamsesh hook pre-tool-use` — gates Bash invocations. Returns
  `permissionDecision: deny` for `git push` and `git config remote.*`.
- `jamsesh hook post-tool-use` — on successful `git commit` calls, performs
  `git push` to the session remote. This is the push-per-commit mechanism.
- `jamsesh hook stop` — auto-commits any uncommitted remainder with a
  skill-instructed generic message, performs a final `git push`, notifies the
  portal of `turn.ended` via REST.
- `jamsesh hook session-end` — clears in-memory caches, optionally posts a
  presence-offline event.

**Slash command subcommands** — invoked by CC skills (skills ARE slash
commands in CC's plugin model). The plugin exposes two slash commands:
`/jamsesh:jam` (intent-driven dispatch for create, join, status, fork,
and mode operations) and `/jamsesh:finalize`. Each skill at
`skills/<name>/SKILL.md` contains body text that instructs Claude to
run the appropriate `jamsesh` subcommand:

- `jamsesh jam new [--org <id>] [--goal <text>] [--scope <glob>] [--mode sync|isolated] [--invite <emails>]`
- `jamsesh jam new --playground` — ephemeral anonymous variant (no org needed)
- `jamsesh jam join <session-id-or-url> [--as <branch>] [--from <commit>]`
- `jamsesh status` — prints tree summary, peers, scope, mode, unresolved
  comments addressed to this user
- `jamsesh fork <commit> [--as <branch>] [--mode sync|isolated]`
- `jamsesh mode sync|isolated` — flips the current ref's mode
- `jamsesh finalize` — opens the portal finalize UI in the browser; with
  `--local` it fetches the cherry-pick plan and prints it

**Auth subcommand:**

- `jamsesh auth` — initiates OAuth flow against the configured portal URL,
  writes the account-wide OAuth token to `${CLAUDE_PLUGIN_DATA}/token`. On the
  next binary invocation (e.g. the next `jamsesh` call), the startup migration
  fans the token out into `${CLAUDE_PLUGIN_DATA}/sessions/<id>/token` for every
  session directory that exists, then replaces `token` with a
  `MIGRATED_TO_PER_SESSION` stub to indicate that per-session files are now
  canonical.

**Internal subcommand for MCP auth:**

- `jamsesh mcp-headers` — invoked by CC's MCP `headersHelper` at connection
  time. When the current CC instance has a bound jamsesh session (matched by
  `CLAUDE_SESSION_ID` against `sessions/<id>/instance_id`), reads the bearer
  from `${CLAUDE_PLUGIN_DATA}/sessions/<id>/token` and emits both
  `Authorization: Bearer <token>` and `Jam-Session-Id: <id>`. Falls back to the
  legacy account-wide `${CLAUDE_PLUGIN_DATA}/token` when no session binding
  exists or the per-session token file is absent.

**Local state layout** under `${CLAUDE_PLUGIN_DATA}/`:

```
${CLAUDE_PLUGIN_DATA}/
├── token                                   account-wide OAuth token (mode 0600);
│                                           contains "MIGRATED_TO_PER_SESSION" once
│                                           per-session files are canonical
├── refresh_token                           OAuth refresh token (mode 0600)
├── portal_url                              configured portal URL
└── sessions/
    └── <session-id>/
        ├── token                           per-session bearer (mode 0600)
        ├── instance_id                     CC session ID bound at join time
        ├── ref                             which user/<branch> this CC is bound to
        ├── org_id                          org that owns this session
        ├── account_id                      authenticated user's account ID
        └── last_seen_seq                   digest cursor
```

### Claude Code plugin package

```
jamsesh/
├── .claude-plugin/
│   └── plugin.json                manifest (name, version, author, etc.)
├── bin/
│   └── jamsesh                    Go binary (multi-arch via marketplace)
├── skills/
│   ├── jamsesh/SKILL.md           auto-loaded skill teaching the agent
│   ├── jam/SKILL.md               /jamsesh:jam command
│   └── finalize/SKILL.md          /jamsesh:finalize command
├── hooks/
│   └── hooks.json                 SessionStart, UserPromptSubmit, PreToolUse,
│                                  PostToolUse, Stop, SessionEnd
└── .mcp.json                      jamsesh MCP server config with headersHelper
```

The auto-loaded `skills/jamsesh/SKILL.md` is what teaches the agent how
jamsesh works: the dual-mode model, addressed-comment semantics,
commit-trailer conventions, how to read the digest, how to use the four MCP
tools, conflict resolution patterns. The skill loads automatically when the
plugin is enabled.

### Portal frontend (SPA)

A Svelte 5 single-page application served by the portal binary. The SPA's
`auth` rune store (`frontend/src/lib/auth.svelte.ts`) maintains two
orthogonal identity states that can coexist in the same tab:

**Durable authenticated-user state** — `token` and `refresh` are persisted
to `localStorage` under `jamsesh.token` / `jamsesh.refresh`. `currentUser`
and `orgs` are in-memory caches loaded from `/api/me` on first use. This is
the standard OAuth session; it survives page reloads.

**Ephemeral playground context** — `_playgroundContext = $state<PlaygroundContext | null>(null)`
holds the anonymous-mode identity `{sessionId, bearer, nickname}` for a
playground session. It is **in-memory only** — never written to localStorage.
A tab reload drops the playground identity, which is intentional: the
server-side bearer is destroyed when the playground session ends (or when
the destruction sweep revokes it), so retaining a stale bearer across
reloads would be misleading. The store exposes this state via
`auth.playgroundContext` (read) and `auth.setPlaygroundContext(ctx | null)`
(write). `auth.isAuthenticated` reflects only the OAuth token; a tab in
playground-only mode has `isAuthenticated === false` and a non-null
`playgroundContext`.

The `auth` export follows the **wrapper-object rune store** pattern (private
`$state` vars, `export const auth = { get ... }` facade) required by Svelte
5's prohibition on exporting raw rune values from a module.

## Data flow: a turn

A single turn from one human-agent pair's perspective.

1. **Human submits a prompt** to Claude Code.
2. **`UserPromptSubmit` hook fires.** `jamsesh hook user-prompt-submit`:
   - `git fetch` from the session remote (pulls new commits across all visible
     refs and the current draft tip).
   - Calls `GET /api/orgs/{orgID}/sessions/{sessionID}/digest?since=<seq>` on the portal. Returns
     new addressed comments (especially those addressed to this agent), new
     conflict events, session-goal updates, mode changes, presence updates.
   - Formats both into a context block: peer commit activity from git log,
     social digest from portal output, current state (goal, draft tip, your
     refs and modes, open conflicts addressed to you).
   - Returns `{"additionalContext": "<formatted block>"}` on stdout.
   - Advances local `last_seen` cursors.
3. **Claude reads the injected context and the human's prompt, reasons, acts.**
   Reads files via standard CC tools (no MCP wrapping of local git ops).
   Optionally calls MCP tools to post comments, fork, or query state.
4. **Agent commits.** Calls `git add` / `git commit` directly. The skill
   teaches it commit-message format and trailers (`Jam-Session`, `Jam-Turn`,
   `Jam-Author`, optionally `Resolves-Conflict`).
5. **`PostToolUse` hook fires after each Bash tool call.** When the call was
   `git commit` and it succeeded, `jamsesh hook post-tool-use` runs `git push`
   to the session remote. This is the push-per-commit mechanism.
6. **Pre-receive validates.** On the portal side, for every push:
   - All commits carry required trailers (`Jam-Session`, `Jam-Turn`, `Jam-Author`).
   - All changed paths fall within the session's writable scope.
   - The pushed ref is in the user's namespace (`jam/<session>/<user>/*`).
   - No force-pushes on shared refs (`base`, `draft`).
   - HTTP Basic auth identifies the user; ref-namespace match enforced.
   Rejection messages list offending commits or paths.
7. **`post-receive` processes the push.**
   - Emits `commit.arrived` events into the event log.
   - WebSocket gateway fans events out to subscribed UI clients.
   - For each commit on a sync-mode ref, the auto-merger picks it up.
8. **Auto-merger runs.** See dedicated section.
9. **Agent does more work or yields.** Loop steps 4–8 for each subsequent
   commit in the turn.
10. **`Stop` hook fires** when the agent yields control to the human:
    - `jamsesh hook stop` auto-commits any dirty working tree with a generic
      message, performs a final push, and POSTs `turn.ended`.

## The auto-merger

The heart of the continuous-integration model. Runs server-side, in-process,
triggered by `post-receive` on any sync-mode ref.

**Per commit arriving on a sync ref:**

1. Resolve the commit's parent in the user's ref history.
2. Resolve the current tip of `jam/<session>/draft`.
3. Find the common ancestor of the commit and the draft tip.
4. Run a three-way merge via `go-git`:
   - The new commit's tree (theirs)
   - The draft tip's tree (ours)
   - The common ancestor's tree (base)
5. **If the merge succeeds:** create a merge commit with the new commit and
   the draft tip as parents, advance `draft` to point at it, emit
   `merge.succeeded`. The merge commit carries `Auto-Merger: true` and
   `Source-Commit: <sha>` trailers.
6. **If the merge conflicts:** do not advance `draft`. Emit
   `conflict.detected` with structured payload: source commit SHA, draft tip
   SHA, common ancestor SHA, conflicted file paths, conflicted line ranges.
   The source commit stays on the user's ref.

**Isolated refs are skipped entirely.** Their commits never trigger
auto-merger work and never end up in draft.

**Resolution flow:** when an agent or human resolves a conflict, they make a
new commit on their ref that incorporates both sides (typically by rebasing
their work onto draft and resolving conflicts in their editor). The commit
includes `Resolves-Conflict: <event-id>` in its trailer. When pushed, the
auto-merger processes it like any other commit. If the three-way merge
succeeds, the conflict event is closed automatically by checking the trailer.

## Dual mode

Each ref under `jam/<session>/<user>/*` has a mode: `sync` or `isolated`.

**Default:** new refs inherit the session's default mode. The session creator
picks the default at creation.

**Sync refs** are auto-merger candidates. Every commit is tried against draft
on push.

**Isolated refs** are private exploration. The auto-merger ignores them.
Conflict events are not emitted against them. They do not contribute to draft
unless explicitly promoted by switching mode.

**Switching mode:**

- `jamsesh mode isolated` — flips the current bound ref from sync to isolated.
  Future commits are not auto-merged. Already-merged commits remain in draft.
- `jamsesh mode sync` — flips from isolated to sync. The next push (and a
  catch-up push of accumulated isolated commits) is processed by the
  auto-merger. Expect conflicts proportional to drift.

**Tree visualization:** the portal UI colors sync refs as part of the trunk
fan and isolated refs as visually detached branches. Mode changes appear as
labeled events.

## Multi-agent per human

A human owns the namespace `jam/<session>/<user>/*` and may have multiple
refs under it. Each Claude Code instance binds to exactly one ref at join:

```
/jamsesh:jam join <session>                              # binds to <user>/main
/jamsesh:jam join <session> --as <branch>                # binds to <user>/<branch>
/jamsesh:jam join <session> --as <branch> --from <ref>   # creates from specific parent
```

Each CC instance has its own working tree and tracks its own ref binding via
`${CLAUDE_PLUGIN_DATA}/sessions/<jamsesh-session-id>/instance_id`, which holds
the `CLAUDE_SESSION_ID` of the CC instance that joined. The per-session bearer
at `sessions/<jamsesh-session-id>/token` is also scoped to that binding.
Concurrent instances under the same user push to different refs in the same
namespace — there's no contention.

A user's two refs may both be sync (both auto-merging into draft) or any
mix. The auto-merger treats them as independent inputs.

## Turn contract

A turn is everything between two human prompts to one Claude Code instance.

**Within a turn:**

- The agent edits files normally.
- The agent commits as many times as it sees semantic boundaries.
- After each commit, `PostToolUse` triggers `jamsesh hook post-tool-use`,
  which pushes to the session remote.
- The agent may NOT run `git push` directly — `PreToolUse` returns
  `permissionDecision: deny`.
- The agent may NOT modify `git config remote.*` — also denied.

**At turn end:**

- The `Stop` hook fires.
- `jamsesh hook stop` auto-commits any dirty tree with a generic message,
  pushes one more time, and POSTs `turn.ended`.

**Why push-per-commit + turn-end push:** peers see your work as it lands.
The turn-end push is still a meaningful sync point — `turn.ended` marks it
for presence indicators.

## Reconciliation (local)

Finalize is curation, not server-side merging. The portal does not perform
cherry-picks and does not host a conflict resolver.

**Flow:**

1. Human hits "finalize" in the portal UI or runs `/jamsesh:finalize`.
2. The portal UI presents the tree and curation interface:
   - Default selection is the leaf agent commits reachable from `draft`
     (auto-merger merge commits are linearized out server-side via a
     first-parent walk)
   - The human can add or remove commits from isolated refs
   - The human picks **finalization mode** — squash into one commit
     (default, matches typical PR-shipping) or preserve all commits
     (multi-author history on the target branch)
   - The human orders the final sequence, names the target branch, and
     (in squash mode) edits the composed commit message
3. The portal generates a **finalize plan** delivered to the human as a
   one-line command `jamsesh finalize-run <plan-id>`. The binary fetches
   the plan body via `GET /api/orgs/{orgID}/sessions/{sessionID}/finalize-plan` and runs it locally. The
   plan body is a mode-aware shell sequence; in squash mode:

   ```bash
   # Commit source: local-first (filesystem path to the user's session
   # checkout, tracked at join time), HTTPS-fallback only when no local
   # checkout is present.
   git fetch <local-session-checkout-path>          # or HTTPS w/ ephemeral token
   git checkout -b <target-branch> <base-sha>
   git cherry-pick --no-commit <commit-1> <commit-2> ... <commit-N>
   git commit --author="<runner>" -F <heredoc-message-with-coauthors>
   ```

   In preserve mode the final two lines become `git cherry-pick <commit-1>
   <commit-2> ...` and authors are kept per-commit.

4. Conflicts during the cherry-pick surface in the human's local environment —
   their editor, their LSP, their test runner. They invoke their normal
   Claude Code (their own session, with full project context) to help
   resolve. The user drives `git cherry-pick --continue` / `--abort`
   themselves; re-invoking `jamsesh finalize-run <plan-id>` detects mid-pick
   state via `git status` and reports what remains. The binary never
   drives `--continue` itself.
5. Human pushes the resulting branch to their source remote:
   `git push origin <target-branch>`
6. PR/MR is the human's choice — outside jamsesh.

## Recovery

Failure modes and recovery:

- **Local terminal closed mid-turn.** Pushed work is durable. Unpushed
  uncommitted work is lost (same as any git workflow). Rejoin via
  `/jamsesh:jam join`; the SessionStart hook fetches and rehydrates context.
- **Network drop mid-push.** Git pushes are atomic — either succeeded (and
  `post-receive` events fired) or didn't. The next push catches up.
- **`jamsesh` binary crash.** Restart Claude Code; `/jamsesh:jam join` again.
  Token and state are persistent on disk.
- **Portal restart.** Active sessions remain in the database. Reconnected
  WebSocket clients re-subscribe. No data loss.
- **Portal data loss.** Bare repos contain version history. Social state
  (comments, conflict events) would be lost but the technical artifact
  survives. Recoverable via standard SQLite/Postgres backup practice.

**General principle:** `git fetch` after any failure restores version state;
a portal API call restores social state. Nothing important lives only in
client memory.

## Horizontal scaling (clustered mode)

Clustered mode is production-ready. The router service, per-session Postgres
leases, fencing tokens, object-storage durability, and hydration handoff are
all shipped. See §14 of `docs/SELF_HOST.md` for operator details.

Single-instance jamsesh is a single portal pod: one Go process, one data store,
one storage volume. For horizontal scale-out a second binary — `jamsesh-router`
— sits in front of multiple portal pods and implements consistent-hash sticky
routing.

### Router binary as the front-door consistent-hash reverse proxy

`jamsesh-router` is a stateless Go binary (`cmd/jamsesh-router`). It:

1. **Extracts the session ID** from every incoming request — from the URL path
   for REST, WebSocket, and Git requests; from the `Jam-Session-Id` header for
   MCP connections.
2. **Hashes the session ID** onto a consistent-hash ring of healthy portal pod
   addresses. The ring uses virtual nodes (default 150 per pod) to keep
   distribution even as pods are added or removed.
3. **Reverse-proxies the request** to the chosen pod using
   `net/http/httputil.ReverseProxy`. WebSocket upgrades pass through
   transparently.
4. **Falls back to round-robin** for requests without a session ID (e.g.
   `/healthz`, `/auth/*`).
5. **Retries on 503** — if the chosen pod returns 503 the router invalidates
   its hint-cache entry for the session and retries once against the ring's
   next preference.
6. **Maintains a soft-coordinator hint cache** — a bounded LRU (10 000 entries,
   5-minute TTL) that remembers which pod served a session last. On cache hit
   the router checks the pod is still in the ring before using the hint, to
   recover cleanly from pod replacement.

Pod discovery uses a static configured list of backend pods (`JAMSESH_ROUTER_STATIC_PODS`),
probed on a configurable interval. The discovery loop calls each pod's `/readyz`
and publishes only the healthy subset to the ring.

The router is intentionally decoupled from session semantics; it does not read
the database and holds no durable state.

### Per-session leases via Postgres advisory locks

Each portal pod acquires a Postgres advisory lock keyed on the session ID
before beginning any write to the session's bare repo or event log. The lock
is held for the duration of the operation and released immediately after.
Advisory locks are lightweight (no table row required) and visible across
connections, which makes them suitable for cross-pod coordination without an
external lock service.

The lock acquisition strategy is non-blocking: a pod that cannot acquire the
lock within a short window returns 503, triggering the router's retry logic.
The retry routes to the next ring preference, which is likely a pod not
currently holding the lock.

### Fencing tokens for split-brain protection

Postgres advisory locks alone guard against concurrent writers in steady state.
Under network partition or clock skew a pod might believe it holds a lease it
has actually lost. Fencing tokens — monotonically increasing integers stamped
on every write — let the storage layer reject stale writes: if a write arrives
with a token lower than the current stored token, it is rejected.

Fencing tokens are issued by Postgres (monotonically increasing integers stored
in a dedicated table) and carried by every lease handle. The `objectstore.Syncer`
embeds the fencing token in object metadata on every upload; a write from a
stale pod is rejected by the manifest's conditional-write check, which compares
the token against the stored value.

### Bare-repo dual-layer storage

Bare repos in clustered mode occupy a two-layer structure:

- **Local-FS working cache** — the pod that holds the lease for a session
  runs `git-receive-pack` against its local disk, exactly as in single-instance
  mode. The local copy provides full git performance (no object-storage latency
  on the critical path for reads and merge operations).
- **Object-storage system of record** — after every `post-receive`, the
  `objectstore.Syncer` uploads all new loose objects, any new or updated pack
  files, and an updated session manifest to the configured object-storage
  backend (AWS S3, Cloudflare R2, GCS, Azure Blob, or any S3-compatible
  service). The manifest is a per-session JSON object written with conditional
  writes (`If-Match` ETag / `ifGenerationMatch` for GCS) to maintain a single
  linearizable history.

This sync is **synchronous and fail-stop**: the git client does not receive a
success response until all objects and the manifest are durable in object
storage. RPO=0 for any push that is acknowledged.

Every upload carries the current lease fencing token in object metadata. If a
stale pod (whose lease was superseded by a newer pod) attempts a write, the
manifest's conditional-write check detects the stale token and the write is
rejected. The push then fails and the git client retries.

### Hydration handoff

When the consistent-hash ring rebalances and a session moves to a new pod, the
`objectstore.LifecycleManager` pre-hydrates the local bare-repo cache as part
of lease acquisition — before the router directs any push traffic to the pod.
The `Hydrator` fetches all objects and pack files listed in the session manifest
from object storage and writes them to local disk using a bounded worker pool
(`JAMSESH_HYDRATION_WORKERS`, default 8). The pod is push-ready before the
first client request arrives; there is no one-push latency on pod transition.

## Data layer (multi-tenancy)

Every persisted entity carries `org_id`. Every API route is org-scoped.
sqlc-generated queries enforce this by including `org_id` in every WHERE
clause where it applies.

**Tables (high-level):**

- `orgs` — top-level tenant
- `accounts` — users within an org
- `oauth_tokens` — user access tokens, refresh tokens, revocation flags
- `sessions` — session metadata (name, goal, scope, default mode, status,
  base sha, created_at, ended_at)
- `session_members` — account ↔ session with role (creator | member)
- `comments` — body, addressing metadata (recipient, kind), anchor
  (commit, file, line range), resolved_at
- `conflict_events` — source/draft/ancestor SHAs, file ranges, status,
  resolving_commit_sha (filled when a `Resolves-Conflict` trailer matches)
- `events` — chronological event log feeding the digest and WebSocket
  gateway
- `presence` — per-(session, user, ref) last-active timestamp and current
  commit SHA
- `invites` — pending invitations with one-time-use tokens

The data layer is the only place where org_id boundaries are enforced. All
queries are generated by sqlc against schema files; cross-org leakage is
structurally impossible if queries follow the org_id-in-WHERE convention.

**Store interface partitioning.** `internal/db/store/store.go` declares a set
of domain sub-interfaces (`CommentStore`, `OrgStore`, `AccountStore`,
`SessionStore`, `LeaseStore`, `PlaygroundSessionStore`, etc.) and an umbrella
`Store` interface that composes them all. The adapter
(`internal/db/store/{sqlite,postgres}_adapter.go`) implements the umbrella.
Production consumers — handlers, services, and workers — accept the narrowest
sub-interface union that reflects their actual data-layer dependency; for
multi-domain consumers a lowercase composed interface (e.g. `accountsStore`)
is declared package-privately in the consumer's own package rather than in
`internal/db/store/`. `cmd/portal/main.go` passes the full adapter to each
constructor, satisfying whatever narrow interface the consumer declares at its
boundary. `WithTx` callbacks receive `TxStore` — the umbrella is retained
there because transaction scope is a separate concern from consumer scope.
This keeps each consumer's data-layer dependency visible at its signature and
keeps test mocks small (5–10 methods rather than the full surface).

**Reserved orgs.** When `JAMSESH_PLAYGROUND_ENABLED=true`, the portal
auto-provisions a single system-owned `playground` org at startup (idempotent
on subsequent boots). Ephemeral playground sessions live inside this org, so
their rows in `sessions`, `session_members`, `comments`, `conflict_events`,
`events`, and `presence` carry the playground org's `org_id` and pass the
standard `org_id`-in-WHERE filters without special-casing. The
session-destruction sweep targets rows by `session_id`; the org row itself
persists across session lifecycles.

The `orgs` table carries an `org_protected` boolean column (migration 00017,
`NOT NULL DEFAULT false`). The playground org is provisioned with
`org_protected=true`. Any `PATCH /api/orgs/{orgID}` request targeting a
protected org — including rename, delete, and `session_invite_policy`
mutations — is rejected with `409 org.protected` before any mutation is
attempted. No other org is protected by default.

## Membership model

Every persisted entity carries `org_id`. Every API route is org-scoped.
Two membership tables exist independently:

- **`org_members`** — the canonical org-level membership. Created by
  `CreateOrg` (creator role) and by `AcceptOrgInvite` (member role).
- **`session_members`** — per-session membership for the actor and any
  invitees. Created when a session is created (creator) and when an
  `AcceptSessionInvite` succeeds (member role).

The relationship between the two is governed by per-org policy:
`orgs.session_invite_policy`:

- **`members_only`** (default) — `AcceptSessionInvite` rejects unless the
  accepting account is already in `org_members` for the same org. Session
  membership implies org membership.
- **`open`** — `AcceptSessionInvite` succeeds regardless of org
  membership. The invitee becomes a session-scoped guest: in
  `session_members` for that session, but never auto-added to
  `org_members`. `handlerauth.RequireOrgMember` correctly keeps such
  guests out of org-scoped operations.

The gate fires at invite-accept time, not at every request. Once a
session_members row exists, the policy was enforced at the perimeter
and downstream handlers trust the membership.
