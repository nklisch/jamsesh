---
id: docs-readme-cc-plugin-install-instructions
kind: story
stage: implementing
tags: [documentation, plugin]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-20
---

# README: add "Install the Claude Code plugin" section

## Brief

`README.md` is missing a section explaining how end-users install the jamsesh
Claude Code plugin. The section should sit between "Operator quickstart" and
"License" and describe the two-step install flow (add the source repo to CC
marketplace, then install the plugin), followed by a brief note on how
`bin/jamsesh` fetches and caches the native binary on first use.

The marketplace config source shape is already known — the plugin's
`marketplace.json` uses `{ "source": "github", "repo": "nklisch/jamsesh" }`.
What needs verification before the docs can ship is the exact user-facing
slash commands inside Claude Code. The feature spec proposed:

```
/marketplace add nklisch/jamsesh
/plugins install jamsesh
```

These command names and argument shapes need to be confirmed against current
Claude Code docs or a live CC instance before being published. Shipping wrong
install commands would mislead users, so this addition was deferred from
`feature-cc-plugin-wrapper-binary-fetch-docs` until the commands can be
verified. Once confirmed, the README addition is a single-session story:
insert the section with the verified commands and a short description of the
wrapper caching behavior.
