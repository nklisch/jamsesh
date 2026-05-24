---
name: finalize
description: Finalize the jamsesh session — opens the portal finalize UI, or prints the cherry-pick plan with --local
argument-hint: "[--local]"
---

> Finalize is the one operation that stays as its own skill (separate
> from `/jamsesh:jam`). It's a multi-step flow with local-vs-portal
> coordination, so it warrants its own surface.

# Finalize the session

Finalize is the end-of-jam curation step run by a human. It produces a
final commit sequence the human can cherry-pick into their source repo
of record.

- *no args* — opens the portal finalize UI for the driving human.
- `--local` — prints the cherry-pick plan to stdout for manual review.

As an agent, you generally do **not** run finalize yourself unless the
driving human explicitly asks. The `jamsesh` skill (auto-loaded with
this plugin) covers the rest of the session lifecycle.

Run:

```bash
bin/jamsesh finalize $ARGUMENTS
```

Surface the result. Print errors with their exit codes intact.
