---
id: epic-cloud-native-deploy-routing-layer
kind: feature
stage: drafting
tags: [infra]
parent: epic-cloud-native-deploy
depends_on: [epic-cloud-native-deploy-operational-polish]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Cloud-Native Deploy — Routing Layer

## Epic context

- Parent epic: `epic-cloud-native-deploy`
- Position in epic: phase-2 clustered-mode component. Independent of
  lease-fencing and object-storage-sync (can be designed and
  implemented in parallel with lease-fencing once operational-polish
  lands). Becomes operationally meaningful only when paired with
  lease-fencing; the soft-coordinator hint cache integrates with
  lease-acquisition events from lease-fencing.

## Foundation references

- `docs/ARCHITECTURE.md` — "System overview" diagram (router becomes
  the new front-door component in clustered mode).
- `docs/SPEC.md` — "Deployment shape" (router is the new optional
  component this feature introduces).
- `docs/SELF_HOST.md` — clustered deploy recipe section this feature
  authors.
- `internal/portal/router/` and `internal/portal/server/server.go` —
  the chi router and HTTP server entry points the router service
  reverse-proxies to.
- `cmd/jamsesh/` (the local plugin binary) — the `mcp-headers`
  subcommand that needs to emit `Jam-Session-Id` to support MCP
  routing.

## Brief

A small consistent-hashing reverse proxy that routes every request to the
portal pod currently leasing the request's `session_id`. Optional
component — only deployed in clustered mode. Single-instance deploys
skip it entirely.

The router extracts `session_id` from each request (URL path for REST /
git / WebSocket, header for MCP), consistent-hashes it into the pod ring,
and reverse-proxies to the winning backend. It probes pod liveness via
`/readyz` (from `epic-cloud-native-deploy-operational-polish`) to keep
the ring fresh.

## Scope

In:
- A new Go binary in this repo (e.g. `cmd/jamsesh-router/`) — single
  small process, no persistent state.
- `session_id` extraction for every endpoint shape:
  - REST: path segment in `/api/sessions/{id}/...`
  - Git smart-HTTP: path segment in `/git/sessions/{id}.git/...`
  - WebSocket: path or query in upgrade URL
  - MCP: `Jam-Session-Id` header (requires the `jamsesh` binary's
    `mcp-headers` subcommand to start emitting this header — small
    coordinated change documented in this feature)
- Consistent-hash ring with virtual nodes for even load distribution.
- Pod-set discovery via k8s API / DNS SRV records / static config
  (operator's choice; ship at least the k8s + static modes).
- `/readyz` probe for ring membership.
- Graceful pod removal: pod goes Not-Ready → router stops sending it
  new requests → pod drains in-flight → pod exits.
- Routing-decision metrics (which pod handled which session, ring
  rebalances, probe failures).
- Operator docs in `docs/SELF_HOST.md` for the clustered deploy recipe.

Out:
- TLS termination. Cluster ingress (LB, k8s Ingress, Cloud Run) does
  this; the router speaks plain HTTP to portal pods.
- Authentication. Auth is per-request and enforced by portal pods.
- Object-storage interactions. Router is stateless; it doesn't know
  about bare repos at all.
- Lease coordination. The router asks "which pod for this session?";
  the lease layer (`epic-cloud-native-deploy-lease-fencing`) answers.
  In v1 the answer comes from the consistent-hash ring (deterministic);
  later iterations may add a hint table for stickiness across ring
  rebalances.

## Design decisions

Inherited from epic (lease acquisition = pull-with-soft-coordinator;
routing as a separate small Go service). Feature-local:

- **Soft-coordinator hint cache lives in the router.** When a pod
  responds 200 to a session request, the router caches `session_id →
  pod` for a short TTL (default 60s). Subsequent requests for the same
  session within the TTL skip the consistent-hash ring and go straight
  to the cached pod. Cache invalidates on 503 from the cached pod or
  on TTL expiry. No persistence — restart loses the cache, falls back
  to consistent hashing. Avoids a coordinator process while still
  giving "hot" sessions stickiness across ring rebalances.
- **Consistent hashing over leader-election.** The ring is the
  authoritative routing fallback. A pod routed a session it doesn't
  currently lease tries `pg_try_advisory_lock` on demand; success →
  serve, failure → 503 with `Retry-After`, router re-dispatches.
- **MCP coordination via `Jam-Session-Id` header**, not body
  inspection. The local `jamsesh mcp-headers` subcommand (already
  emits the auth header) is extended to also emit `Jam-Session-Id`
  based on the current bound session. Small coordinated change
  documented in this feature.

## Foundation-doc impact

- `docs/ARCHITECTURE.md` — when this feature lands at `stage: done`, add
  a "Horizontal scaling" subsection describing the clustered-mode
  topology (router + pods + Postgres + object storage). Don't
  pre-document.
- `docs/SELF_HOST.md` — clustered deploy recipe section.

## Notes for design

The MCP coordination point is the trickiest part. The MCP protocol puts
`session_id` in tool call payloads, not at the transport layer. We have
two viable approaches: (a) the `jamsesh` binary's `mcp-headers`
subcommand emits a `Jam-Session-Id` header based on the current bound
session, OR (b) the router does HTTP-body inspection to extract
session_id from MCP JSON. (a) is cleaner and within our control; pick
that. Coordinate the small `jamsesh mcp-headers` change as part of this
feature.

WebSocket session-id extraction needs to handle both URL-path and query
forms depending on what `wsgateway` currently expects — check during
design.
