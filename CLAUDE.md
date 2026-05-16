<!-- agile-workflow:start -->
## Agile-Workflow Substrate

Work tracked in `.work/` as markdown items with YAML frontmatter
(`kind, stage, tags, parent, depends_on, release_binding`).
Layout: `.work/active/{epics,features,stories}/`, `.work/backlog/`,
`.work/releases/<version>/`, `.work/archive/`.

**Primary query tool:** `.work/bin/work-view` filters by stage, tag, kind,
parent, and dependency. Common patterns:
- `work-view --ready` — items ready to work (deps satisfied)
- `work-view --stage review` — items waiting on user
- `work-view --parent <id>` / `--blocking <id>` — hierarchy / sequencing
- `work-view --help` for the full flag set

Detailed navigation rules in `.claude/rules/agile-workflow.md` (auto-loaded
when editing `.work/` or `docs/`). Foundation docs in `docs/` describe the
system NOW — never add legacy notes; git history is the audit trail.

Slash commands (user-invokable):
`/agile-workflow:ideate`, `/agile-workflow:epicize`,
`/agile-workflow:autopilot`, `/agile-workflow:release-deploy`.
<!-- agile-workflow:end -->
