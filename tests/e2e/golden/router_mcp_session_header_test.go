// Invariant: MCP tool calls carrying Jam-Session-Id: <session_id> are routed
// to the pod that holds the advisory-lock lease for that session, regardless
// of which pod served the MCP initialize handshake.
//
// The router's session-ID extraction for MCP requests is header-based:
// internal/router/extract/extract.go reads the "Jam-Session-Id" header (not
// "Mcp-Session-Id"). The "Mcp-Session-Id" header is the MCP wire-protocol
// session token and is consumed by the portal's MCP SDK for request continuity;
// it does NOT drive router routing. This test sets BOTH headers on every tool
// call so both layers are satisfied.
//
// Routing identity is asserted via cluster.LeaseHolder (Postgres pg_locks) —
// not by inspecting response bodies or per-pod headers.
package golden_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/mcpclient"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestRouterMCPSessionHeader is the golden test for Jam-Session-Id header
// pinning through the router. It verifies that repeated MCP tool calls that
// carry the correct Jam-Session-Id are always served by the same backend pod.
func TestRouterMCPSessionHeader(t *testing.T) {
	// ── Infrastructure ───────────────────────────────────────────────────────
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)

	cluster := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        2,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      true,
		PortalExtraEnv: map[string]string{
			"JAMSESH_EMAIL_PROVIDER":  "smtp",
			"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":  "none",
		},
	})
	require.NotEmpty(t, cluster.RouterURL,
		"mcp-session-header test requires Router: true — RouterURL must not be empty")

	// Auth and org setup use pod 0 directly (shared Postgres; token is cluster-wide).
	pod0 := cluster.Pods[0]
	userEmail := randEmail(t, "mcp-header")
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "MCP Header Routing Org")

	t.Run("mcp_jam_session_id_pins_to_handshake_pod", func(t *testing.T) {
		testMCPJamSessionIDPinsToPod(ctx, t, cluster, pair.AccessToken, orgID)
	})
}

// testMCPJamSessionIDPinsToPod verifies that N≥5 MCP tool calls carrying
// Jam-Session-Id: <jamseshSessionID> are all served by the same pod.
//
// Steps:
//  1. Create a jamsesh session via REST through the router so the session ID is
//     known before the MCP handshake.
//  2. Perform the MCP initialize handshake through the router URL using
//     mcpclient.New, capturing the Mcp-Session-Id.
//  3. Wait for the jamsesh session's lease to be acquired by a pod (the REST
//     create triggers lease acquisition on whichever pod round-robins first).
//  4. Issue N≥5 MCP tool calls (query_session_state) via direct HTTP, setting
//     both Mcp-Session-Id (MCP wire protocol) and Jam-Session-Id (router
//     extraction) headers.
//  5. After each call, assert cluster.LeaseHolder returns the same pod index.
func testMCPJamSessionIDPinsToPod(
	ctx context.Context,
	t *testing.T,
	cluster *portalcluster.Cluster,
	accessToken, orgID string,
) {
	t.Helper()

	// Step 1: Create a jamsesh session via REST so we know the session_id up front.
	// The session creation request goes through the router — the pod that handles
	// it will acquire the advisory lock for this session_id.
	sessionID := createSessionViaRouterURL(ctx, t, cluster.RouterURL, accessToken, orgID,
		fmt.Sprintf("mcp-header-pin-%d", time.Now().UnixNano()))

	// Step 2: Perform the MCP initialize handshake through the router URL.
	// mcpclient.New sends POST /mcp with no session-scoped header, so the router
	// round-robins it — the MCP session may land on a different pod than the
	// jamsesh session's lease holder. That is fine: the invariant is that
	// subsequent tool calls with Jam-Session-Id pin to the lease holder.
	mc := mcpclient.New(t, cluster.RouterURL, accessToken)
	mcpSessionID := mc.MCPSessionID()
	require.NotEmpty(t, mcpSessionID,
		"mcp_jam_session_id_pins_to_handshake_pod: MCP initialize returned empty Mcp-Session-Id")
	t.Logf("mcp_jam_session_id_pins_to_handshake_pod: Mcp-Session-Id=%s", mcpSessionID)

	// Step 3: Wait for the lease to be held. The REST session creation triggers
	// lease acquisition; allow up to 5 s for the portal to acquire the lock.
	firstHolder := cluster.RequireLeaseHolder(ctx, t, sessionID, 5*time.Second)
	t.Logf("mcp_jam_session_id_pins_to_handshake_pod: initial lease holder = pod %d", firstHolder)

	// Step 4+5: Issue N MCP tool calls with both headers set and assert the same
	// pod holds the lease after each call.
	const toolCallCount = 5

	for i := 0; i < toolCallCount; i++ {
		// query_session_state is read-only and idempotent — safe for repeated calls.
		err := routerMCPRequest(ctx, t, cluster.RouterURL, accessToken, mcpSessionID, sessionID,
			mcpToolsCallPayload("query_session_state", map[string]any{
				"session_id": sessionID,
			}))
		require.NoErrorf(t, err,
			"mcp_jam_session_id_pins_to_handshake_pod: tool call %d failed: %v", i+1, err)

		holder := cluster.LeaseHolder(ctx, t, sessionID)
		require.GreaterOrEqualf(t, holder, 0,
			"mcp_jam_session_id_pins_to_handshake_pod: call %d: LeaseHolder returned -1 for session %s — "+
				"no pod holds the advisory lock; possible hashtext portability issue or lock released",
			i+1, sessionID)
		require.Equalf(t, firstHolder, holder,
			"mcp_jam_session_id_pins_to_handshake_pod: call %d: session %s routed to pod %d "+
				"but initial holder was pod %d — Jam-Session-Id header routing violated",
			i+1, sessionID, holder, firstHolder)

		t.Logf("mcp_jam_session_id_pins_to_handshake_pod: call %d/%d → pod %d ✓",
			i+1, toolCallCount, holder)
	}

	t.Logf("mcp_jam_session_id_pins_to_handshake_pod: all %d MCP tool calls pinned to pod %d ✓",
		toolCallCount, firstHolder)
}

// ---------------------------------------------------------------------------
// MCP HTTP helpers local to this file
// ---------------------------------------------------------------------------

// routerMCPRequest sends a single MCP JSON-RPC request to routerURL/mcp with:
//   - Authorization: Bearer <accessToken>
//   - Mcp-Session-Id: <mcpSessionID>   (MCP wire-protocol session continuity)
//   - Jam-Session-Id: <jamSessionID>   (router's consistent-hash extraction key)
//
// It asserts a 2xx response and that the response body contains a valid
// JSON-RPC 2.0 envelope without a top-level error field.
func routerMCPRequest(
	ctx context.Context,
	t *testing.T,
	routerURL, accessToken, mcpSessionID, jamSessionID string,
	payload []byte,
) error {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		routerURL+"/mcp", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Mcp-Session-Id", mcpSessionID)
	req.Header.Set("Jam-Session-Id", jamSessionID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("router returned %d: %s", resp.StatusCode, body)
	}

	// Parse the response: SSE or plain JSON. Extract the JSON-RPC envelope.
	jsonData := extractJSONFromSSE(body)

	var env struct {
		JSONRPC string `json:"jsonrpc"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(jsonData, &env); err != nil {
		return fmt.Errorf("decode JSON-RPC envelope: %w — raw: %s", err, body)
	}
	if env.Error != nil {
		return fmt.Errorf("JSON-RPC error %d: %s", env.Error.Code, env.Error.Message)
	}
	return nil
}

// mcpToolsCallPayload builds a JSON-RPC 2.0 tools/call request body.
func mcpToolsCallPayload(toolName string, args map[string]any) []byte {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	})
	if err != nil {
		panic(fmt.Sprintf("mcpToolsCallPayload: marshal: %v", err))
	}
	return payload
}

// extractJSONFromSSE extracts the JSON payload from an SSE-framed response.
// If the body is not SSE (no "data: " prefix found), the raw body is returned.
func extractJSONFromSSE(raw []byte) []byte {
	lines := bytes.Split(raw, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("data: ")) {
			return bytes.TrimPrefix(line, []byte("data: "))
		}
	}
	return raw
}
