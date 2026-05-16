---
description: Agile-workflow substrate navigation rules
paths: ['.work/**', 'docs/**']
---

# Agile-Workflow Substrate Navigation

## Folder structure
.work/active/{epics,features,stories}/  in-flight, scoped
.work/backlog/                           parked, unscoped
.work/releases/<version>/                shipped bundles
.work/archive/                           done items not bound to a release

## Item kinds
epic     multi-feature arc; has children    parent of features
feature  design + implementation unit       parent of stories
story    single-session unit                leaf or has tasks
task     checklist line in parent body      not its own file
release  version bundle in releases/        binds items via release_binding

## Stages
epic     drafting → implementing → review → done
feature  drafting → implementing → review → done
story    implementing → review → done       (often skips drafting)
task     [ ] → [x]
release  planned → quality-gate → released

## Frontmatter
id, kind, stage, tags[], parent, depends_on[], release_binding,
gate_origin, created, updated

## Querying with work-view (primary tool)

`.work/bin/work-view` is the canonical query tool — use it instead of
hand-grepping frontmatter. Filters compose with AND semantics; combine
freely. Run `--help` for the authoritative flag list.

### Filters
--stage <stage>      drafting | implementing | review | done | released
--tag <tag>          repeatable; AND across tags
--kind <kind>        epic | feature | story | release
--parent <id>        direct children of given item
--release <version>  items with release_binding: <version>
--gate <name>        items produced by gate <name>
--ready              stage:implementing AND all depends_on done
--blocked            stage:implementing AND unmet dependencies
--blocking <id>      items that depend on <id>

### Output modes
(default tabular)    columns: ID  KIND  STAGE  TAGS  PARENT
--paths              one file path per line (pipe-friendly)
--cat                full item bodies, separated by ---
--count              match count only

### Common queries

# Items ready to work right now
.work/bin/work-view --ready

# Items awaiting user review
.work/bin/work-view --stage review

# All children of an epic
.work/bin/work-view --parent <epic-id>

# Children of an epic that are still blocked
.work/bin/work-view --parent <epic-id> --blocked

# Read full bodies of every item bound to a release
.work/bin/work-view --release v1.2.0 --cat

# Security-tagged items currently implementing
.work/bin/work-view --stage implementing --tag security

# Items that would unblock if <id> finishes
.work/bin/work-view --blocking <id>

# Pipe paths into another tool
.work/bin/work-view --ready --paths | xargs grep -l 'TODO'

## Fallback: raw substrate access

When work-view doesn't fit (e.g. searching item bodies, not frontmatter):

# Search inside item bodies
grep -rn '<phrase>' .work/active/

# Item history
git log -p -- .work/active/features/<id>.md

# Recent substrate changes
git log --since='1 day ago' -- .work/

## Session start checklist
1. cat .work/CONVENTIONS.md            project-specific overrides
2. .work/bin/work-view --stage review  items waiting on user
3. .work/bin/work-view --ready         items ready to work
4. Identify your work: explicit user ask, or pick the next ready item

## Stage transition discipline
- Update `stage:` and let PostToolUse hook auto-bump `updated:`
- Commit after each stage transition (one commit per item per transition)
- Do not pre-populate stages; advance only as work completes

## Foundation docs (rolling-forward principle)
docs/ holds standing context: VISION.md, SPEC.md, ARCHITECTURE.md, etc.
- Foundation docs describe the system as it is NOW
- Never add "previously this was…" or "note: in v1.2 we…"
- When implementation changes a foundation-doc assertion, update the doc
- Git history is the audit trail; the doc is the present
