---
id: bug-release-bump-sed-inplace-strips-exec-bit
created: 2026-05-19
tags: [bug, tooling, release]
---

# `scripts/release-bump.sh` silently strips the executable bit on `bin/jamsesh`

The `sed_inplace` helper in `scripts/release-bump.sh:57-61` writes the
edited content to `${file}.tmp` and then `mv`s it over the original. The
temp file is created with the default umask (typically `0644`), so any
file that had the executable bit set ends up `0644` after the edit.

The only release-bumped file this affects in practice is `bin/jamsesh`,
which must stay `0755` because it is the Claude Code plugin entrypoint
that end-users invoke directly. The `.env.example` and `plugin.json`
edits are unaffected (those files are already non-executable).

Surfaced during the v0.1.2 release pass on 2026-05-19:

- `release-prep: v0.1.2` commit (`a1b7691`, since amended) included
  `mode change 100755 => 100644 bin/jamsesh`.
- Required cancelling the in-progress CI run, restoring the bit with
  `chmod +x bin/jamsesh`, amending the commit, force-pushing main, and
  retagging `v0.1.2`.

## Fix scope (small)

In `scripts/release-bump.sh`, change `sed_inplace` so it preserves the
original file's mode. Two viable approaches:

1. **Capture and restore mode** around the `mv`:
   ```bash
   sed_inplace() {
     local file="$1" pattern="$2" replacement="$3"
     local tmp="${file}.tmp"
     local mode
     mode="$(stat -c '%a' "$file" 2>/dev/null || stat -f '%Lp' "$file")"
     sed "s/${pattern}/${replacement}/" "$file" > "$tmp" \
       && chmod "$mode" "$tmp" \
       && mv "$tmp" "$file"
   }
   ```
   Portable across Linux (`stat -c`) and macOS (`stat -f`).

2. **Edit in place via a here-temp pattern that preserves perms** —
   e.g. `cp -p "$file" "$tmp" && sed ... > "$tmp"` — but this is
   harder to keep correct than (1).

## Acceptance criteria

- [ ] After `scripts/release-bump.sh vX.Y.Z`, `git status --short` shows
      no `mode change` lines for `bin/jamsesh`.
- [ ] `ls -l bin/jamsesh` remains `-rwxr-xr-x` post-bump.
- [ ] The fix works on both Linux and macOS (CI is Linux; maintainers
      may run on macOS).
- [ ] Add a regression check: either a smoke test that runs the script
      against a fixture and asserts mode preservation, or a pre-commit
      assertion that the staged `bin/jamsesh` has mode `100755`.

## References

- Bug surfaced 2026-05-19 during v0.1.2 release.
- Original sed_inplace helper: `scripts/release-bump.sh:57-61`.
- Related: `testing-bin-jamsesh-regression-harness` (a harness around
  `bin/jamsesh` would have caught the broken-perms artifact end-to-end).
