---
name: mode
description: Switch the current ref between sync mode (auto-merged into draft) and isolated mode (private exploration)
argument-hint: "sync|isolated"
---

# Switch ref mode

Toggles the mode of your current ref. The `jamsesh` skill (auto-loaded
with this plugin, section 2) covers what each mode means and when to
use it — re-read if you're unsure.

Short version:

- `sync` — the auto-merger tries every commit you push against
  `draft`. Default for normal collaborative work.
- `isolated` — the auto-merger ignores your ref. You accumulate work
  privately. Use for speculative exploration or a large conflict-prone
  refactor you want to land in one batch.

Switching `isolated` → `sync` replays all accumulated commits through
the auto-merger at once. Expect conflict events proportional to how
far `draft` has drifted since you went isolated. The flow for
resolving them is in the `jamsesh` skill section 6 (and
`references/conflicts.md`).

Run:

```bash
bin/jamsesh mode $ARGUMENTS
```

Surface the result. Print errors with their exit codes intact.
