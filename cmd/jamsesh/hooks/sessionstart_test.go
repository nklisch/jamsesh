package hooks_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jamsesh/cmd/jamsesh/hooks"
	"jamsesh/internal/api/openapi"
)

// setupHookEnv creates a temp CLAUDE_PLUGIN_DATA dir, writes credentials and
// session state, and points JAMSESH_PORTAL_URL at the given test server URL.
// Returns the dir path.
func setupHookEnv(t *testing.T, srvURL, sessionID, orgID, ref, accountID string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
	t.Setenv("JAMSESH_PORTAL_URL", srvURL)
	t.Setenv("CC_SESSION_ID", "") // use first-dir fallback

	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("tok-test"), 0o600); err != nil {
		t.Fatalf("writing token: %v", err)
	}

	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatalf("creating session dir: %v", err)
	}
	writes := map[string]string{
		"org_id":        orgID,
		"ref":           ref,
		"account_id":    accountID,
		"last_seen_seq": "0",
		"instance_id":   "inst-test",
	}
	for name, val := range writes {
		if err := os.WriteFile(filepath.Join(sessDir, name), []byte(val), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	return dir
}

// runHook invokes a hook action with JSON stdin and returns the decoded output.
func runHook(t *testing.T, action func(context.Context, *hookCLICmd) error, inputJSON string) map[string]any {
	t.Helper()
	in := strings.NewReader(inputJSON)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	if err := action(ctx, nil); err != nil {
		t.Fatalf("hook action error: %v", err)
	}
	var result map[string]any
	if err := json.NewDecoder(&out).Decode(&result); err != nil {
		t.Fatalf("decoding hook output: %v\nraw: %s", err, out.String())
	}
	return result
}

// hookCLICmd is a type alias to satisfy the cli.Command parameter in hook functions.
// The hooks package imports github.com/urfave/cli/v3 so we need to match the
// signature. We use nil in tests.
type hookCLICmd = struct{}

// additionalContext extracts the additionalContext string from hook output.
func additionalContext(t *testing.T, r map[string]any) string {
	t.Helper()
	v, _ := r["additionalContext"].(string)
	return v
}

func TestSessionStart_noSession(t *testing.T) {
	// No CLAUDE_PLUGIN_DATA → no session → empty output.
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
	t.Setenv("JAMSESH_PORTAL_URL", "http://localhost:9")
	t.Setenv("CC_SESSION_ID", "")

	input := `{"session_id":"cc-sess","transcript_path":"/tmp/t","cwd":"/home/user"}`
	in := strings.NewReader(input)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	if err := hooks.SessionStart(ctx, nil); err != nil {
		t.Fatalf("SessionStart error: %v", err)
	}
	var result map[string]any
	if err := json.NewDecoder(&out).Decode(&result); err != nil {
		t.Fatalf("decoding output: %v", err)
	}
	// additionalContext should be absent or empty when no session is bound.
	if ac, ok := result["additionalContext"]; ok && ac != "" {
		t.Errorf("expected no additionalContext, got %q", ac)
	}
}

func TestSessionStart_happy(t *testing.T) {
	const (
		orgID     = "org-ss-001"
		sessionID = "sess-ss-001"
		accountID = "acct-ss-001"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	mux := http.NewServeMux()

	mux.HandleFunc("/api/me", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%q,"display_name":"Test","email":"t@example.com","orgs":[{"id":%q}]}`, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.Session{
			Id:    sessionID,
			Name:  "My Session",
			Goal:  "Write great code",
			Scope: "src/**",
			OrgId: orgID,
		})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/refs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.RefListResponse{
			Refs: []openapi.Ref{
				{Ref: ref, Sha: "deadbeefcafe12345678", Mode: "sync"},
				{Ref: "jam/" + sessionID + "/peer-001/main", Sha: "aabbccddeeff12345678", Mode: "isolated"},
			},
		})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.CommentListResponse{
			Items: []openapi.Comment{
				{
					Id:       "cmt-001",
					Body:     "Please fix the typo",
					Kind:     openapi.CommentKindActionRequest,
					AuthorId: "peer-001",
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupHookEnv(t, srv.URL, sessionID, orgID, ref, accountID)

	input := `{"session_id":"cc-sess","transcript_path":"/tmp/t","cwd":"/repo"}`
	in := strings.NewReader(input)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	if err := hooks.SessionStart(ctx, nil); err != nil {
		t.Fatalf("SessionStart error: %v", err)
	}

	var result map[string]any
	if err := json.NewDecoder(&out).Decode(&result); err != nil {
		t.Fatalf("decoding output: %v\nraw: %s", err, out.String())
	}

	ac := additionalContext(t, result)
	if !strings.Contains(ac, "My Session") {
		t.Errorf("additionalContext missing session name; got:\n%s", ac)
	}
	if !strings.Contains(ac, "Write great code") {
		t.Errorf("additionalContext missing goal; got:\n%s", ac)
	}
	if !strings.Contains(ac, "src/**") {
		t.Errorf("additionalContext missing scope; got:\n%s", ac)
	}
	if !strings.Contains(ac, ref) {
		t.Errorf("additionalContext missing my ref; got:\n%s", ac)
	}
	if !strings.Contains(ac, "peer-001") {
		t.Errorf("additionalContext missing peer ref; got:\n%s", ac)
	}
	if !strings.Contains(ac, "Please fix the typo") {
		t.Errorf("additionalContext missing comment body; got:\n%s", ac)
	}
}

func TestSessionStart_commentsEndpointCalledWithAddressedTo(t *testing.T) {
	const (
		orgID     = "org-ss-002"
		sessionID = "sess-ss-002"
		accountID = "acct-ss-002"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	var capturedQuery string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"id":%q,"display_name":"T","email":"t@e.com","orgs":[{"id":%q}]}`, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID, func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(openapi.Session{Id: sessionID, Name: "S", Goal: "G", OrgId: orgID})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/refs", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(openapi.RefListResponse{Refs: []openapi.Ref{}})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/comments", func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(openapi.CommentListResponse{Items: []openapi.Comment{}})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupHookEnv(t, srv.URL, sessionID, orgID, ref, accountID)

	in := strings.NewReader(`{"session_id":"cc","transcript_path":"","cwd":""}`)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	_ = hooks.SessionStart(ctx, nil)

	if !strings.Contains(capturedQuery, accountID) {
		t.Errorf("comments endpoint query %q does not contain accountID %q", capturedQuery, accountID)
	}
	if !strings.Contains(capturedQuery, "resolved=false") {
		t.Errorf("comments endpoint query %q missing resolved=false", capturedQuery)
	}
}

func TestSessionStart_noPeerRefs(t *testing.T) {
	const (
		orgID     = "org-ss-003"
		sessionID = "sess-ss-003"
		accountID = "acct-ss-003"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"id":%q,"display_name":"T","email":"t@e.com","orgs":[{"id":%q}]}`, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID, func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(openapi.Session{Id: sessionID, Name: "S", Goal: "G", OrgId: orgID})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/refs", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(openapi.RefListResponse{Refs: []openapi.Ref{}})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/comments", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(openapi.CommentListResponse{Items: []openapi.Comment{}})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupHookEnv(t, srv.URL, sessionID, orgID, ref, accountID)

	in := strings.NewReader(`{"session_id":"cc","transcript_path":"","cwd":""}`)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	if err := hooks.SessionStart(ctx, nil); err != nil {
		t.Fatalf("SessionStart error: %v", err)
	}

	var result map[string]any
	_ = json.NewDecoder(&out).Decode(&result)
	ac := additionalContext(t, result)

	if !strings.Contains(ac, "(none)") {
		t.Errorf("expected '(none)' for empty peer list; got:\n%s", ac)
	}
}
