---
id: gate-cruft-pushbase-headsha-discard-pattern
kind: story
stage: implementing
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# Dead `headSHA` computations in `pushBaseRef` / `pushBaseRefWithBearer`

## Confidence
Medium

## Category
defensive try/catch

## Location
`cmd/jamsesh/sessioncmd/new.go:269-274, 437-442`

## Evidence
```go
headSHA, err := runGitOutput("rev-parse", "HEAD")
if err != nil {
    return fmt.Errorf("repo has no commits yet (nothing to push as base): %w", err)
}
headSHA = strings.TrimSpace(headSHA)
_ = headSHA // used for validation; actual push uses HEAD refspec
```

## Removal
The validation work is the `err != nil` check; capturing, trimming, then discarding `headSHA` adds nothing. Replace the four-line block with a single line that discards the value at capture: `if _, err := runGitOutput("rev-parse", "HEAD"); err != nil { return fmt.Errorf(...) }`. Drop the `strings.TrimSpace` and `_ = headSHA` and the misleading "used for validation" comment from both `pushBaseRef` and `pushBaseRefWithBearer`. If `strings` becomes unused elsewhere in the file, drop the import.
