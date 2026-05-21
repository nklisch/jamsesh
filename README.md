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

## Install the Claude Code plugin

The jamsesh plugin runs inside Claude Code and gives each agent the `join`,
`status`, `fork`, and `mode` slash commands, plus auto-loading session context
so agents know how to participate in a jam.

Install in two steps from any Claude Code session:

```
claude plugin marketplace add nklisch/jamsesh
claude plugins install jamsesh
```

The plugin ships a small Go wrapper (`bin/jamsesh`) that fetches the right
native binary for your platform on first use and caches it under
`~/.cache/jamsesh/`. That wrapper is what Claude Code invokes when the slash
commands run — no manual binary install required.

> Commands verified against Claude Code CLI (`claude plugins --help`,
> `claude plugin marketplace --help`). If your version differs, run
> `claude plugin --help` to see the current command surface.

## License

Apache 2.0 — see [LICENSE](LICENSE).
