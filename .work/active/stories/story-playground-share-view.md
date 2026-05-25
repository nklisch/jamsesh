---
id: story-playground-share-view
kind: story
stage: drafting
tags: [bug, ui, portal]
parent: feature-portal-visitor-entry-pages
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Playground share-URL view (`/playground/:id`)

## Brief

The playground share URL printed by `jamsesh jam new --playground`
(e.g. `https://jamsesh.dev/playground/01KSEKEBP2X9TVMVBEA85BENVE`)
returns HTTP 200 from the server but renders as a 404 inside the SPA —
the `/playground/:id` client-side route either doesn't exist or doesn't
match the ULID pattern.

The session is alive in the portal (`jamsesh status` lists it; the API
returns 401 for unauthenticated access as expected), so the bug is
purely the missing SPA route / view that should accept the share URL,
fetch the session, and either render a join CTA or auto-bind the
visitor as a new playground participant.

The auto-bind-vs-CTA decision is a strategic call deferred to the
parent feature's design pass.
