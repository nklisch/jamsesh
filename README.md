# jamsesh

> Multi-agent jamming for codebases — coordinated Claude Code sessions
> producing PR-shaped branches without merge headaches.

License: Apache 2.0 · See [docs/SELF_HOST.md](docs/SELF_HOST.md) for the
full operator guide.

## What it is

Jamsesh is a collaboration expirence where a small team of humans drive their Claude's against a shared git-backed session, producing artifacts together
in a live jam. Each human-agent pair gets their own namespace of refs to push
to. A server-side auto-merger continuously integrates non-conflicting work into
a shared draft ref, making the artifact converge live. Conflicts surface as
structured events that agents can act on in-session.

When the jam is done, you get a finalized branch you cherry-pick into your own
source repo — on your own terms, with your own Claude Code instance. The portal
never touches your source repo. Everything is real git: diff-able, recoverable,
and attributable.

## Local development

The fastest way to spin up a dev environment:

```bash
# Terminal 1 — bring up the portal (SQLite, plain HTTP on :8443)
docker compose up

# Terminal 2 — bring up the Vite dev server for the SPA (:5173)
cd frontend && npm run dev
```

Then open <http://localhost:5173> in your browser. Editing any `.go`
file rebuilds and restarts the portal binary inside the container via
[`air`](https://github.com/air-verse/air); the Vite dev server hot-reloads
the SPA on `.svelte` / `.ts` edits.

Data — the SQLite database and per-session bare repos — lands in
`./.data/` on your host. To wipe and start fresh: `make dev-down-v` (or
`docker compose down -v && rm -rf .data`).

For the operator-facing production deployment, see
[Operator quickstart](#operator-quickstart) below and
[docs/SELF_HOST.md](docs/SELF_HOST.md).

## Operator quickstart

The fastest way to run jamsesh on your own host:

```bash
git clone https://github.com/nklisch/jamsesh
cd jamsesh/deploy/compose
cp .env.example .env
$EDITOR .env       # set JAMSESH_DOMAIN + OAuth or email creds
docker compose up -d
```

This brings up the portal behind a Caddy reverse proxy with automatic
HTTPS. See [`deploy/compose/README.md`](deploy/compose/README.md) for the
template reference and [docs/SELF_HOST.md](docs/SELF_HOST.md) for TLS,
OAuth, database options, and production deployment details.

To kick the tires locally without a domain or TLS:

```bash
docker run --rm -p 8443:8443 \
  -e JAMSESH_TLS_MODE=behind_proxy \
  -e JAMSESH_BIND=:8443 \
  -v $(pwd)/data:/data \
  ghcr.io/nklisch/jamsesh:latest

curl http://localhost:8443/healthz
# → {"status":"ok"}
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
