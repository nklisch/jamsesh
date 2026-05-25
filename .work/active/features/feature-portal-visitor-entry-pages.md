---
id: feature-portal-visitor-entry-pages
kind: feature
stage: drafting
tags: [ui, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Portal visitor-facing entry pages

## Brief

The portal today has no curated visitor-facing entry experience.
Two concrete gaps:

- The playground share URL (`/playground/<ulid>`) returns HTTP 200 from
  the server but renders as a 404 inside the SPA — there's no
  client-side route that accepts the share link and helps a visitor
  join.
- The portal root (`/`) is a bare entry with no explanation of what
  jamsesh is, no path into the playground, and no login affordance for
  durable sessions — every fresh visitor lands somewhere uninformative.

This feature scopes the visitor-facing surface as one coherent unit
because the two child stories share design context (visual language,
navigation chrome, "what does a fresh visitor see" framing) and one
underlying mechanism (a deploy-time flag that selects between two
shipped landing variants).

## Child stories

- `story-playground-share-view` — SPA route + view for
  `/playground/:id`. Fetches the playground session, renders either a
  join CTA or auto-binds the visitor as a new participant.
- `story-landing-flagged-dual-mode` — flagged root landing with two
  variants: a project page (jamsesh.dev) and a generic playground/login
  chooser (self-host with playground enabled).

## Strategic decisions deferred to feature-design

The following directional calls are deferred to
`/agile-workflow:feature-design`:

- **Flag mechanism**: env var, compile-time build tag, or runtime
  config file? Must compose with `JAMSESH_PORTAL_URL` and the existing
  self-host config story (see `docs/SELF_HOST.md`).
- **Playground share UX**: auto-bind on visit vs. explicit join CTA.
  Auto-bind is friction-free but commits the visitor irreversibly;
  CTA is explicit but adds a click.
- **Default for jamsesh.dev**: should the project-page variant be the
  hardcoded default for the canonical deployment, or also opt-in via
  the same flag for symmetry?
- **Scope of the generic variant**: minimum viable (two buttons:
  playground / login) vs. richer (also surfaces recent public
  sessions, docs links, etc.).
- **Self-host without playground**: today the portal has no landing at
  all. Do we ship a third variant for that case, or keep the bare
  behaviour and let self-hosters who want a landing just enable the
  flag?
