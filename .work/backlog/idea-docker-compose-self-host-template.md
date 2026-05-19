---
id: idea-docker-compose-self-host-template
created: 2026-05-18
tags: [infra]
---

Ship a turn-key `docker-compose.yml` (and matching `.env.example`) for
self-hosting jamsesh so the path from "I want to try this on my own box"
to "running portal" is as close to one command as possible — ideally
`docker compose up -d` after editing two or three variables. Should
bundle the portal container, Postgres (or sqlite-by-default with a
clear opt-in to Postgres), and reasonable defaults for OAuth callback
URL, ports, TLS termination via a reverse-proxy sidecar (caddy/traefik
for auto-Let's-Encrypt), and a volume for the bare-repo storage.
`docs/SELF_HOST.md` already covers the full operator reference; this
template is the "happy-path quickstart" companion. Worth weighing
single-node vs. clustered profiles, whether to include a healthcheck
loop, and how to keep the template in sync with the release Docker
image tags.
