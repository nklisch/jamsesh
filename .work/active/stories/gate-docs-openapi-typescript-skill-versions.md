---
id: gate-docs-openapi-typescript-skill-versions
kind: story
stage: implementing
tags: [documentation, ui]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: docs
created: 2026-05-18
updated: 2026-05-18
---

# `.claude/skills/openapi-typescript/SKILL.md` pins versions that don't match `frontend/package.json`

## Drift category
repo-skill-staleness

## Location
- Doc: `.claude/skills/openapi-typescript/SKILL.md:13-17`
- Code: `frontend/package.json:14` (`"openapi-fetch": "^0.13.0"`);
  generator is invoked via `npm run generate` CLI in `package.json:7`,
  not via a `openapi-typescript.config.ts` (no such file exists in
  `frontend/`).

## Current doc text
> - `openapi-typescript@~7.13.0` (released 2026-02-11)
> - `openapi-fetch@~0.17.0` (released 2026-02-11)
> …
> Use `openapi-typescript.config.ts` (checked in), not scattered CLI flags

## Reality
- openapi-typescript is on `~7.13.0` (matches).
- openapi-fetch is on `^0.13.0` (skill claim `~0.17.0` is wrong).
- Generator config is CLI flags in `package.json`
  (`openapi-typescript ../docs/openapi.yaml -o src/lib/api/types.gen.ts`);
  there is no `openapi-typescript.config.ts`.

## Required edit
Update the skill's version pins to match `frontend/package.json` (or
update package.json if the skill's pin is the intent). Replace the
"Use `openapi-typescript.config.ts`" passage with a reference to the
CLI invocation in `frontend/package.json`.
