---
id: dev-docker-compose-docs
kind: story
stage: implementing
tags: [infra, documentation]
parent: dev-docker-compose
depends_on: [dev-docker-compose-setup]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Docs — `make dev` target + README onboarding subsection

After the setup story lands, document the new path so contributors
discover it from the canonical README without reading the substrate.

## Files

- Modify: `/Makefile` — add `dev` target
- Modify: `/README.md` — add "Local development" section
- Possibly modify: `/docs/SELF_HOST.md` — cross-link from the operator
  guide if it's appropriate (this is for SELF-HOST operators, not local
  contributors, so probably skip — judgment call by implementer)

## Target shape

### Makefile `dev` target

Append to `/Makefile`:

```makefile
# dev: bring up the local development stack via docker compose.
# Builds the dev image on first run; subsequent runs reuse the build cache.
# For hot frontend reload, run `cd frontend && npm run dev` in another terminal.
.PHONY: dev dev-down dev-rebuild
dev:
	docker compose up

# dev-down: tear down the dev stack. Use `dev-down-v` to also drop .data/.
dev-down:
	docker compose down

dev-down-v:
	docker compose down -v
	rm -rf .data

# dev-rebuild: rebuild the dev image (use after go.mod / Dockerfile.dev edits).
dev-rebuild:
	docker compose up --build
```

The `.PHONY` line should be merged with the existing `.PHONY` declaration
at the top of the Makefile (which already lists `generate`, `build`, etc.) —
the implementer can either add it inline next to the targets or extend
the existing top-level line. Either is fine.

### README onboarding section

Add a new section in `/README.md` above the existing "Quickstart (Docker)"
section (which is operator-focused; the new section is for contributors):

```markdown
## Local development

The fastest way to spin up a dev environment:

\`\`\`bash
# Terminal 1 — bring up the portal (SQLite, plain HTTP on :8443)
docker compose up

# Terminal 2 — bring up the Vite dev server for the SPA (:5173)
cd frontend && npm run dev
\`\`\`

Then open <http://localhost:5173> in your browser. Editing any `.go`
file rebuilds and restarts the portal binary inside the container via
[`air`](https://github.com/air-verse/air); the Vite dev server hot-reloads
the SPA on `.svelte` / `.ts` edits.

Data — the SQLite database and per-session bare repos — lands in
`./.data/` on your host. To wipe and start fresh: `make dev-down-v` (or
`docker compose down -v && rm -rf .data`).

For the operator-facing production deployment, see [Quickstart (Docker)](#quickstart-docker)
below and [docs/SELF_HOST.md](docs/SELF_HOST.md).
```

The `## Quickstart (Docker)` section title might need to be renamed to
something like `## Operator quickstart` for clarity, since the new
section is itself a Docker quickstart. Implementer's call — preserve
operator-discoverability either way.

## Acceptance criteria

- [ ] `make dev` brings up the dev stack (equivalent to `docker compose up`)
- [ ] `make dev-down` stops cleanly; `make dev-down-v` also clears `.data/`
- [ ] `make dev-rebuild` rebuilds the image (validates that go.mod edits
      pick up correctly)
- [ ] README has a "Local development" section above the operator
      Quickstart that walks a contributor from clone to running portal +
      Vite in two commands
- [ ] The README section explicitly mentions `./.data/` location, the
      `:5173` browse URL, and the two-terminal model
- [ ] No regression to existing Makefile targets (`make build`, `make go-build`,
      `make test-e2e`, etc.)
- [ ] No regression to the existing "Quickstart (Docker)" operator section

## Risk

LOW. Pure documentation + Makefile glue. The only risk is the README
section drifting from reality if the compose config changes later —
mitigated by the cross-reference between the two stories' acceptance
criteria.

## Rollback

`git revert` the commit. The README and Makefile revert to their
pre-feature state; setup story's artifacts are untouched.
