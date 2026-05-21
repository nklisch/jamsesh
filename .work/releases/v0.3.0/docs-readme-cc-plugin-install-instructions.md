---
id: docs-readme-cc-plugin-install-instructions
kind: story
stage: done
tags: [documentation, plugin]
parent: null
depends_on: []
release_binding: v0.3.0
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

## Implementation notes

**Verification path:** Used the live `claude` CLI binary (`which claude` →
`/home/nathan/.local/bin/claude`). Ran `claude plugins --help` and
`claude plugin marketplace --help` to confirm the exact command surface.

**Verified commands:**
- `claude plugin marketplace add nklisch/jamsesh` — the `marketplace add`
  subcommand takes a `<source>` arg; GitHub shorthand (`user/repo`) is
  accepted per the help text ("URL, path, or GitHub repo").
- `claude plugins install jamsesh` — `plugins install|i <plugin>` installs
  from available marketplaces.

**Link adjustments:** `docs/CC_PLUGIN.md` does not exist. Rather than link
to a non-existent doc, the "see…" line was dropped. The plugin manifest
(`.claude-plugin/plugin.json`) is the canonical reference but it's not a
user-facing doc, so no link was added.

**Section placement:** Lines 77–98 in `README.md`, between
`## Operator quickstart` and `## License`.

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Section sits cleanly between "Operator quickstart" and "License". Commands verified against the actual local `claude` CLI (`claude plugin marketplace add ...`, `claude plugins install ...`) — the version-sensitivity footnote in the section acknowledges that other CC versions may differ, which is honest documentation. Wrapper-binary caching note is accurate per the existing `bin/jamsesh` design. Voice matches the rest of the README. No `release_binding`, so archiving.
