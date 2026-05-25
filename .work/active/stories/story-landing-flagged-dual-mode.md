---
id: story-landing-flagged-dual-mode
kind: story
stage: drafting
tags: [ui, portal]
parent: feature-portal-visitor-entry-pages
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Flagged dual-mode root landing

## Brief

A feature-flagged landing/welcome screen at the portal root, with two
shipped variants chosen at deploy time.

- **Project-page variant** (used by jamsesh.dev itself): a
  marketing/info page explaining what jamsesh is, with links to the
  GitHub project, hosted docs, and a "Try the playground" CTA.
- **Generic-deploy variant** (for self-hosters who enable playground):
  a neutral landing that lets a visitor choose between starting a
  playground session or logging in to a durable session.

Both replace the current bare entry experience. The flag governs which
variant renders, and self-hosters without playground enabled get
neither (today's behaviour) unless the parent feature's design pass
decides otherwise.

The flag mechanism and the default-for-jamsesh.dev question are
strategic calls deferred to the parent feature's design pass.
