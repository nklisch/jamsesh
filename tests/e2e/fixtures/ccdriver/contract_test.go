package ccdriver_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"jamsesh/tests/e2e/fixtures/ccdriver"
)

// update rewrites frozen contract files when -update is passed to go test.
var update = flag.Bool("update", false, "rewrite frozen contract payloads to match current struct output")

// assertContract marshals v to indented JSON and compares it to the frozen
// file at contract/<name>.json. When -update is set it overwrites the file
// instead of failing.
func assertContract(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(%s): %v", name, err)
	}

	contractPath := filepath.Join("contract", name+".json")

	if *update {
		if err := os.WriteFile(contractPath, got, 0o644); err != nil {
			t.Fatalf("updating %s: %v", contractPath, err)
		}
		t.Logf("updated %s", contractPath)
		return
	}

	want, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v (run `go test -update` to create it)", contractPath, err)
	}
	if string(got) != string(want) {
		t.Errorf(
			"payload contract drift for %s\n\ngot:\n%s\n\nwant:\n%s\n\nIf this drift is intentional (Claude Code's hook protocol changed), regenerate the frozen file:\n  go test ./fixtures/ccdriver/ -update\nOr manually copy the 'got' output above into %s",
			name, got, want, contractPath,
		)
	}
}

func TestPayloadContracts(t *testing.T) {
	t.Run("session-start", func(t *testing.T) {
		in := ccdriver.SessionStartInput{
			SessionID:      "ses_abc123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/workspace/repo",
		}
		assertContract(t, "session-start", in)
	})

	t.Run("user-prompt-submit", func(t *testing.T) {
		in := ccdriver.UserPromptSubmitInput{
			SessionID:      "ses_abc123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/workspace/repo",
		}
		assertContract(t, "user-prompt-submit", in)
	})

	t.Run("pre-tool-use", func(t *testing.T) {
		in := ccdriver.PreToolUseInput{
			SessionID:      "ses_abc123",
			TranscriptPath: "/tmp/transcript.json",
			ToolName:       "Bash",
			ToolInput:      json.RawMessage(`{"command": "ls -la"}`),
		}
		assertContract(t, "pre-tool-use", in)
	})

	t.Run("post-tool-use", func(t *testing.T) {
		in := ccdriver.PostToolUseInput{
			SessionID:      "ses_abc123",
			TranscriptPath: "/tmp/transcript.json",
			ToolName:       "Bash",
			ToolInput:      json.RawMessage(`{"command": "git commit -m \"feat: add feature\""}`),
			ToolResponse: ccdriver.ToolResponse{
				ExitCode: 0,
				Stdout:   "[main abc1234] feat: add feature\n 1 file changed, 10 insertions(+)",
				Stderr:   "",
			},
		}
		assertContract(t, "post-tool-use", in)
	})

	t.Run("stop", func(t *testing.T) {
		in := ccdriver.StopInput{
			SessionID:      "ses_abc123",
			TranscriptPath: "/tmp/transcript.json",
		}
		assertContract(t, "stop", in)
	})

	t.Run("session-end", func(t *testing.T) {
		in := ccdriver.SessionEndInput{
			SessionID:      "ses_abc123",
			TranscriptPath: "/tmp/transcript.json",
		}
		assertContract(t, "session-end", in)
	})
}
