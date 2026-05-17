package sessioncmd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/sessioncmd"
)

// cannedForkResp is a canned MCP JSON-RPC response for the fork tool.
const cannedForkResp = `{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [],
    "structuredContent": {"ref": "jam/sess1/user/main", "sha": "cafebabe"}
  }
}`

func setupSession(t *testing.T, sessionID, portalURL string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
	t.Setenv("JAMSESH_PORTAL_URL", portalURL)

	// Create session state directory.
	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("creating session dir: %v", err)
	}

	// Write instance_id so resolveSession can map CC_SESSION_ID -> jamsesh session.
	if err := os.WriteFile(filepath.Join(sessDir, "instance_id"), []byte("cc-inst-1"), 0o600); err != nil {
		t.Fatalf("writing instance_id: %v", err)
	}
	t.Setenv("CC_SESSION_ID", "cc-inst-1")

	// Write a token.
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("test-token"), 0o600); err != nil {
		t.Fatalf("writing token: %v", err)
	}

	return dir
}

func TestForkCommand_basic(t *testing.T) {
	var gotReqBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReqBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(cannedForkResp)) //nolint:errcheck
	}))
	defer srv.Close()

	setupSession(t, "sess1", srv.URL)

	app := &cli.Command{
		Commands: []*cli.Command{sessioncmd.ForkCommand()},
	}

	err := app.Run(context.Background(), []string{"jamsesh", "fork", "deadbeef1234"})
	// Note: git fetch will fail in test (no git repo / no session-remote), so we
	// only care that the MCP call succeeded and the args were correct. The error
	// from git fetch is non-fatal (printed as warning), so the command returns nil.
	if err != nil {
		// If there's an error it should NOT be about the MCP call itself.
		if strings.Contains(err.Error(), "fork MCP call failed") || strings.Contains(err.Error(), "parsing fork result") {
			t.Fatalf("unexpected MCP error: %v", err)
		}
	}

	// Verify the JSON-RPC params.
	params, ok := gotReqBody["params"].(map[string]any)
	if !ok {
		t.Fatalf("params not an object: %T", gotReqBody["params"])
	}
	if params["name"] != "fork" {
		t.Errorf("params.name = %v, want fork", params["name"])
	}
	arguments, ok := params["arguments"].(map[string]any)
	if !ok {
		t.Fatalf("params.arguments not an object: %T", params["arguments"])
	}
	if arguments["session_id"] != "sess1" {
		t.Errorf("arguments.session_id = %v, want sess1", arguments["session_id"])
	}
	if arguments["target_commit_sha"] != "deadbeef1234" {
		t.Errorf("arguments.target_commit_sha = %v, want deadbeef1234", arguments["target_commit_sha"])
	}
}

func TestForkCommand_withFlags(t *testing.T) {
	var gotArgs map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		params := body["params"].(map[string]any)
		gotArgs = params["arguments"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(cannedForkResp)) //nolint:errcheck
	}))
	defer srv.Close()

	setupSession(t, "sess1", srv.URL)

	app := &cli.Command{
		Commands: []*cli.Command{sessioncmd.ForkCommand()},
	}

	err := app.Run(context.Background(), []string{"jamsesh", "fork", "abc123", "--as", "feature-x", "--mode", "isolated"})
	if err != nil {
		if strings.Contains(err.Error(), "fork MCP call failed") || strings.Contains(err.Error(), "parsing fork result") {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if gotArgs["target_ref"] != "feature-x" {
		t.Errorf("arguments.target_ref = %v, want feature-x", gotArgs["target_ref"])
	}
	if gotArgs["mode"] != "isolated" {
		t.Errorf("arguments.mode = %v, want isolated", gotArgs["mode"])
	}
}

func TestForkCommand_missingCommitSHA(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)

	app := &cli.Command{
		Commands: []*cli.Command{sessioncmd.ForkCommand()},
	}
	err := app.Run(context.Background(), []string{"jamsesh", "fork"})
	if err == nil {
		t.Fatal("expected error for missing commit SHA, got nil")
	}
}
