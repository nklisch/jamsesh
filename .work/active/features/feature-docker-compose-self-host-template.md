---
id: feature-docker-compose-self-host-template
kind: feature
stage: drafting
tags: [infra]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Docker Compose self-host template

## Brief

Ship a turn-key `docker-compose.yml` (and matching `.env.example`) for
self-hosting jamsesh so the path from "I want to try this on my own box" to
"running portal" is as close to one command as possible — ideally `docker
compose up -d` after editing two or three variables in `.env`.

The template bundles the portal container, a TLS-terminating reverse proxy
sidecar (auto-LE), and a volume for bare-repo storage. Database backend is
chosen via compose profile (sqlite single-file by default, Postgres opt-in).
Reasonable defaults for OAuth callback URL, ports, and storage paths so the
operator edits a tiny `.env` rather than a full compose file.

`docs/SELF_HOST.md` already covers the full operator reference; this template
is the "happy-path quickstart" companion. Once shipped, `SELF_HOST.md` gains a
quickstart section pointing at the template and `README.md` gains a one-liner.

## Strategic decisions

Locked at scope time so feature-design inherits the framing. Each is
reversible at design time if it turns out wrong — call them out then.

- **Storage default: SQLite, with a Postgres profile** — matches the
  "happy-path quickstart" framing from the idea body. SQLite is the
  zero-friction default for a single-node operator just trying jamsesh on
  their box. Postgres opt-in via `--profile postgres` for operators who
  already run a database or want HA-ready storage. Rationale: getting to
  "portal running" should not require a separate DB container by default.
- **Profile coverage: single-node primary, clustered deferred** — the
  template ships single-node only in v1. Clustered self-hosters (multiple
  portal replicas + router + Postgres + object storage) are an
  operationally distinct shape that the CND architecture targets at
  Kubernetes, not compose. A future `docker-compose.cluster.yml` companion
  is fine but isn't bound here. Rationale: scope discipline; compose for
  the quickstart audience, k8s manifests for the clustered audience.
- **TLS sidecar: Caddy** — Caddy's automatic HTTPS (Let's Encrypt + ACME +
  cert renewal with zero config beyond a domain name) is the lowest-friction
  fit for the "edit two or three variables" target. Traefik is more
  configurable but adds surface area we don't need at the quickstart tier.
  An operator who prefers Traefik can swap the sidecar; the portal contract
  is just "something terminates TLS in front of me on 8443".

## Mockups

N/A — infrastructure/config only, no UI surface.

<!-- Design and Implementation Notes sections accumulate as feature-design
and implement-orchestrator advance this item. -->
