# Principles

Standing principles for jamsesh. These outlive features. When a design decision
is in tension, the principle wins.

## Git is the substrate

Every version, every fork, every diff is a real git operation against a real
repo. We do not implement a parallel version-control system. The portal observes
git; it never simulates it.

**Consequence:** Diffs, merges, line tracking, gc, recovery — all free from git.
Debugging is `git log`. There is never a "portal says X, git says Y" class of
bug because the portal derives its view from git.

## Portal owns social, git owns version

Comments, presence, addressing metadata, autonomy hints, session goals — these
live in the portal's database. Refs, commits, trees, merges — these live in
git. Neither tries to do the other's job.

**Consequence:** The portal can be restarted, restored, or replaced without
losing version history. The git side can be inspected with standard tooling
without losing social context (the portal joins them at read time).

## The portal never touches the source remote

The session remote and the source remote are separate. The portal hosts the
session remote; the source remote belongs to the user. The portal holds no
credentials for the source remote, makes no calls against any source forge API,
and never pushes to source.

**Consequence:** The source repo stays sacrosanct. The portal's blast radius
under any breach is the in-flight session content only. Self-host needs no
forge integration to function.

## Forge-agnostic by construction

Because the portal never touches the source remote, jamsesh works against any
git remote that accepts a push — GitHub, GitLab, Bitbucket, Gitea, self-hosted,
plain SSH. There is no per-forge code path.

**Consequence:** No "we support GitHub but not GitLab" scoping debates. Adding
support for a new forge takes zero work.

## All agent-to-agent signals are visible

Agents communicate via the artifact (commits), via addressed comments, and via
structured events (conflict notifications) — all of which are visible to every
participant in the session. There are no private agent backchannels.

**Consequence:** Full auditability. No hidden coordination. Humans can always
see what their agent saw and why it did what it did.

## Liveness via continuous integration, not deferred reconciliation

The auto-merger continuously folds non-conflicting work into a shared draft ref
as it arrives. Conflicts surface mid-session for the relevant agents to address
while they're still in flow. Finalize is mostly a curation step, not a merge
ordeal.

**Consequence:** The artifact converges as the jam happens. Long sessions
don't accumulate a giant final merge.

## Pull-only on the client

The local jamsesh binary fetches state on its own cadence — git fetch at turn
start, digest API call, etc. Nothing pushes events into the local client. The
portal never reaches into a client's context.

**Consequence:** Clients can drop offline and recover by fetching. No
firewall holes for push channels. Local trust boundary stays clean.

## Local-first for anything touching local files

Operations that touch the user's local filesystem (git checkouts, file edits,
finalization cherry-picks) happen in the user's local environment with their
local agent. The portal coordinates state; it does not act on local files.

**Consequence:** The human's tooling — LSP, formatters, tests, CI hooks —
works against real paths. Reconciliation benefits from full project context.
Trust stays on the user side.

## One human, one identity, multiple agents

A portal account maps to exactly one git author identity. That identity owns a
namespace of refs in each session. A human may drive multiple Claude Code
instances against multiple refs in the same session simultaneously.

**Consequence:** Attribution is clean — a commit's author is unambiguous.
Parallel exploration is a first-class capability without inventing alter-egos.

## Recovery is `git fetch`

Plugin crash, network drop, portal restart, terminal close — recovery is
always `git fetch` (and a portal-state pull). Nothing is lost that was pushed.
Nothing is hidden in a custom state file.

**Consequence:** Failure modes stay simple. The system never accumulates
hidden state that can desync from git.
