# jamsesh

> Multi-agent jamming for codebases — coordinated Claude Code sessions
> producing PR-shaped branches without merge headaches.

License: Apache 2.0 · See [docs/SELF_HOST.md](docs/SELF_HOST.md) for the
full operator guide.

## What it is

Jamsesh is a collaboration substrate where a small team of humans drive Claude
Code instances against a shared git-backed session, producing artifacts together
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

```bash
docker run --rm -p 8443:8443 \
  -e JAMSESH_TLS_MODE=behind_proxy \
  -e JAMSESH_BIND=:8443 \
  -v $(pwd)/data:/data \
  ghcr.io/<owner>/jamsesh:latest

curl http://localhost:8443/healthz
# → {"status":"ok"}
```

This runs the portal in behind-proxy mode (plain HTTP on `localhost:8443`,
suitable for local testing or when sitting behind a TLS-terminating reverse
proxy). Data — the SQLite database and per-session bare repos — lands in
`./data/` on your host.

For TLS, OAuth, database options, and production deployment, see
[docs/SELF_HOST.md](docs/SELF_HOST.md).

## License

Apache 2.0 — see [LICENSE](LICENSE).
