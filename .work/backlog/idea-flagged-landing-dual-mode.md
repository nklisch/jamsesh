---
id: idea-flagged-landing-dual-mode
created: 2026-05-24
tags: []
---

A feature-flagged landing/welcome screen at the portal root, with two shipped variants chosen at deploy time. **Project-page variant** (used by jamsesh.dev itself): a marketing/info page explaining what jamsesh is, with links to the GitHub project, hosted docs, and a "Try the playground" CTA. **Generic-deploy variant** (for self-hosters who enable playground): a neutral landing that lets a visitor choose between starting a playground session or logging in to a durable session. Both replace the current bare entry experience; the flag governs which variant renders, and self-hosters without playground enabled get neither (today's behaviour).
