# ccdriver — Claude Code hook lifecycle simulator

`ccdriver` invokes the jamsesh binary's hook subcommands with crafted JSON
stdin, simulating Claude Code's plugin lifecycle deterministically.

## Usage in e2e specs

```go
d := &ccdriver.Driver{
    BinaryPath: "/path/to/jamsesh",
    DataDir:    t.TempDir(),
    ExtraEnv:   []string{"JAMSESH_PORTAL_URL=http://localhost:8080"},
}

out, err := d.StartSession(ctx, ccdriver.SessionStartInput{
    SessionID:      "ses_test",
    TranscriptPath: "/tmp/transcript.json",
    Cwd:            t.TempDir(),
})
```

Full integration examples land in the golden-path feature.

## Contract drift

Each hook event has a frozen JSON payload at `contract/<event>.json`. The
test suite asserts that the driver's payload builders produce JSON matching
the frozen file. When `go test ./fixtures/ccdriver/...` fails with a
"payload contract drift" message, one of two things is true:

1. **You changed a payload struct unintentionally.** Revert the struct change.
2. **Claude Code's hook protocol changed.** If you have confirmed the new
   shape from the upstream spec (`hooks/hooks.json` + `cmd/jamsesh/hooks/`),
   regenerate all frozen files:

   ```
   cd tests/e2e
   go test ./fixtures/ccdriver/ -update
   ```

   Or update a single file by hand: copy the "got" output from the failing
   test into the corresponding `contract/<event>.json`.

## Wire format

The six hook events and their input/output types:

| Event               | Input type              | Output type              |
|---------------------|-------------------------|--------------------------|
| `session-start`     | `SessionStartInput`     | `SessionStartOutput`     |
| `user-prompt-submit`| `UserPromptSubmitInput` | `UserPromptSubmitOutput` |
| `pre-tool-use`      | `PreToolUseInput`       | `PreToolUseOutput`       |
| `post-tool-use`     | `PostToolUseInput`      | `PostToolUseOutput`      |
| `stop`              | `StopInput`             | `StopOutput`             |
| `session-end`       | `SessionEndInput`       | `SessionEndOutput`       |
