---
id: ccdriver-env-test-tautology
kind: story
stage: drafting
tags: [e2e-test, testing, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# ccdriver env-inheritance test passes erroneously when env is empty

## Finding

`tests/e2e/fixtures/ccdriver/driver_test.go > TestRunHookInheritsHostPath`
uses shell scripts that write `{}` to stdout only when the env condition
is satisfied. On the failure path (condition not met), each script writes
nothing and exits 0.

The driver's `runHook` short-circuits when stdout is empty:

```go
if len(stdout) == 0 {
    return out, nil  // no error
}
```

So when the env condition fails (e.g., PATH not inherited), the script
writes nothing → exit 0 → driver returns nil with no error → test
assertion `if err != nil` is FALSE → test passes erroneously.

Verified by inspection during review of
`epic-e2e-tests-golden-path-ccdriver-env-fix`. The fix itself (prepending
`os.Environ()`) is correct, but the test doesn't actually catch a
regression.

## Suggested fix

Two options — pick whichever the implementor prefers:

1. **Script exits non-zero on failure**:
   ```sh
   #!/bin/sh
   cat > /dev/null
   if [ -z "$PATH" ]; then
       exit 1
   fi
   printf '{}'
   ```
   Now if PATH is empty, the script exits 1, `cmd.Output()` returns
   `*exec.ExitError`, driver returns the error, test fails.

2. **Switch to real-binary integration test**: build the actual `jamsesh`
   binary via `go build ./cmd/jamsesh`, invoke a real hook subcommand
   (e.g., `session-end`), assert the subprocess starts and runs (no
   "binary not found" or PATH-related startup errors). This was the
   original intent of the story's acceptance criterion #3.

Option 1 is faster (no go build). Option 2 is more representative.

## Acceptance criteria

- [ ] A regression that removes `os.Environ()` from `runHook` causes
      the test to FAIL (not pass silently)
- [ ] The test continues to verify all three original conditions: PATH
      inherited, ExtraEnv forwarded, CLAUDE_PLUGIN_DATA appended last

## Notes

Discovered during the review of
`epic-e2e-tests-golden-path-ccdriver-env-fix`. The implementation fix
is correct; only the test needs strengthening.
