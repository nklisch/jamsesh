package mcpclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jamsesh/cmd/jamsesh/mcpclient"
)

// cannedResult is the JSON-RPC response the test server returns.
// It mirrors the MCP SDK's tool result envelope with StructuredContent.
const cannedResult = `{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [],
    "structuredContent": {"ref": "jam/sess/user/main", "sha": "abc123"}
  }
}`

func TestCallTool_success(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(cannedResult)) //nolint:errcheck
	}))
	defer srv.Close()

	c := &mcpclient.Client{
		PortalURL: srv.URL,
		Token:     "test-token",
		HTTP:      srv.Client(),
	}

	result, err := c.CallTool(context.Background(), "fork", map[string]any{
		"session_id":        "sess1",
		"target_commit_sha": "deadbeef",
	})
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}

	// Verify Authorization header.
	if want := "Bearer test-token"; gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}

	// Verify JSON-RPC payload shape.
	if gotBody["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v", gotBody["jsonrpc"])
	}
	if gotBody["method"] != "tools/call" {
		t.Errorf("method = %v", gotBody["method"])
	}
	params, ok := gotBody["params"].(map[string]any)
	if !ok {
		t.Fatalf("params is not an object: %T", gotBody["params"])
	}
	if params["name"] != "fork" {
		t.Errorf("params.name = %v, want fork", params["name"])
	}

	// Verify structured content is returned.
	var sc struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(result, &sc); err != nil {
		t.Fatalf("unmarshalling result: %v", err)
	}
	if sc.Ref != "jam/sess/user/main" {
		t.Errorf("result.Ref = %q, want jam/sess/user/main", sc.Ref)
	}
	if sc.SHA != "abc123" {
		t.Errorf("result.SHA = %q, want abc123", sc.SHA)
	}
}

func TestCallTool_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &mcpclient.Client{
		PortalURL: srv.URL,
		Token:     "bad-token",
		HTTP:      srv.Client(),
	}

	_, err := c.CallTool(context.Background(), "fork", nil)
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
}

func TestCallTool_rpcError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"invalid session"}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := &mcpclient.Client{
		PortalURL: srv.URL,
		Token:     "tok",
		HTTP:      srv.Client(),
	}

	_, err := c.CallTool(context.Background(), "fork", nil)
	if err == nil {
		t.Fatal("expected RPC error, got nil")
	}
}
