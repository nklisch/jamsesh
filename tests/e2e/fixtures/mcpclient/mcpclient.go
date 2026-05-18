// Package mcpclient provides typed wrappers around the portal's MCP endpoint
// for use in e2e tests. Each method maps to one of the four MCP tools exposed
// by the portal: post_comment, resolve_comment, fork, and query_session_state.
//
// The portal's MCP endpoint speaks JSON-RPC 2.0 over HTTP POST at /mcp.
// Bearer-token authentication is sent via the Authorization header.
// This implementation mirrors cmd/jamsesh/mcpclient (the production client)
// and adds typed input/output structs for each tool.
//
// MCP session lifecycle:
//
//	Client.Init must be called once to perform the MCP initialize handshake and
//	obtain an Mcp-Session-Id. The session ID is then attached to all subsequent
//	tool calls. This matches the protocol requirement from the portal's MCP SDK.
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Client is a session-aware MCP tool caller. Call Init once to complete the
// MCP handshake before invoking any tool methods.
type Client struct {
	// PortalURL is the portal origin, e.g. "http://localhost:PORT".
	// Trailing slash is allowed; "/mcp" is appended internally.
	PortalURL string
	// Bearer is the access token sent in the Authorization header.
	Bearer string
	// HTTP is the underlying HTTP client. If nil, http.DefaultClient is used.
	HTTP *http.Client
	// mcpSessionID is set by Init after the initialize handshake.
	mcpSessionID string
}

// MCPSessionID returns the Mcp-Session-Id received from the initialize
// handshake. Empty until Init has completed successfully.
func (c *Client) MCPSessionID() string {
	return c.mcpSessionID
}

// New constructs a Client for use in e2e tests, performs the MCP initialize
// handshake immediately, and registers t.Cleanup (stateless; nothing to close).
// Callers may also initialise Client directly and call Init separately.
func New(t *testing.T, portalURL, bearer string) *Client {
	t.Helper()
	c := &Client{
		PortalURL: portalURL,
		Bearer:    bearer,
	}
	if err := c.Init(context.Background()); err != nil {
		t.Fatalf("mcpclient.New: MCP initialize handshake: %v", err)
	}
	return c
}

// Init performs the MCP initialize + notifications/initialized handshake and
// stores the resulting Mcp-Session-Id for subsequent tool calls. It must be
// called once before any tool method.
func (c *Client) Init(ctx context.Context) error {
	// 1. Send initialize request.
	initReq := rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "e2e-test-client", "version": "0.1.0"},
		},
	}
	payload, err := json.Marshal(initReq)
	if err != nil {
		return fmt.Errorf("marshal initialize: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.PortalURL+"/mcp", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build initialize request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.Bearer)

	hc := c.hc()
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("send initialize: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("initialize: got %d: %s", resp.StatusCode, body)
	}

	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		return fmt.Errorf("initialize: no Mcp-Session-Id header in response")
	}
	c.mcpSessionID = sessionID

	// 2. Send notifications/initialized notification (no response expected).
	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	}
	nbody, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("marshal initialized notification: %w", err)
	}

	nreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.PortalURL+"/mcp", bytes.NewReader(nbody))
	if err != nil {
		return fmt.Errorf("build initialized notification request: %w", err)
	}
	nreq.Header.Set("Content-Type", "application/json")
	nreq.Header.Set("Accept", "application/json, text/event-stream")
	nreq.Header.Set("Authorization", "Bearer "+c.Bearer)
	nreq.Header.Set("Mcp-Session-Id", sessionID)

	nresp, err := hc.Do(nreq)
	if err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}
	defer nresp.Body.Close()

	return nil
}

// ---------------------------------------------------------------------------
// Typed arg / result structs (mirror mcpendpoint/tools.go)
// ---------------------------------------------------------------------------

// PostCommentArgs are the arguments for the post_comment MCP tool.
type PostCommentArgs struct {
	SessionID   string  `json:"session_id"`
	CommitSHA   string  `json:"commit_sha"`
	FilePath    *string `json:"file_path,omitempty"`
	LineStart   *int32  `json:"line_start,omitempty"`
	LineEnd     *int32  `json:"line_end,omitempty"`
	Body        string  `json:"body"`
	AddressedTo *string `json:"addressed_to,omitempty"`
	Kind        *string `json:"kind,omitempty"`
}

// PostCommentResult is the result of the post_comment MCP tool.
type PostCommentResult struct {
	CommentID string `json:"comment_id"`
}

// ResolveCommentArgs are the arguments for the resolve_comment MCP tool.
type ResolveCommentArgs struct {
	SessionID      string  `json:"session_id"`
	CommentID      string  `json:"comment_id"`
	ResolutionNote *string `json:"resolution_note,omitempty"`
}

// ResolveCommentResult is the result of the resolve_comment MCP tool.
type ResolveCommentResult struct {
	CommentID string `json:"comment_id"`
	Resolved  bool   `json:"resolved"`
}

// ForkArgs are the arguments for the fork MCP tool.
type ForkArgs struct {
	SessionID       string  `json:"session_id"`
	TargetCommitSHA string  `json:"target_commit_sha"`
	TargetRef       *string `json:"target_ref,omitempty"`
	Mode            *string `json:"mode,omitempty"`
}

// ForkResult is the result of the fork MCP tool.
type ForkResult struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

// QuerySessionStateArgs are the arguments for the query_session_state MCP tool.
type QuerySessionStateArgs struct {
	SessionID string   `json:"session_id"`
	SinceSeq  *int64   `json:"since_seq,omitempty"`
	Include   []string `json:"include,omitempty"`
}

// QuerySessionStateResult is the result of the query_session_state MCP tool.
// Fields are left as json.RawMessage for flexibility; callers may decode
// specific sub-fields as needed.
type QuerySessionStateResult struct {
	Goal               string          `json:"goal,omitempty"`
	Scope              string          `json:"scope,omitempty"`
	DraftTip           string          `json:"draft_tip,omitempty"`
	UnresolvedComments json.RawMessage `json:"unresolved_comments_for_me,omitempty"`
	OpenConflicts      json.RawMessage `json:"open_conflicts_for_me,omitempty"`
	RecentEvents       json.RawMessage `json:"recent_events,omitempty"`
}

// ---------------------------------------------------------------------------
// Typed tool callers
// ---------------------------------------------------------------------------

// PostComment calls the post_comment MCP tool and returns the new comment ID.
func (c *Client) PostComment(ctx context.Context, args PostCommentArgs) (PostCommentResult, error) {
	var result PostCommentResult
	if err := c.callTool(ctx, "post_comment", args, &result); err != nil {
		return PostCommentResult{}, err
	}
	return result, nil
}

// ResolveComment calls the resolve_comment MCP tool.
func (c *Client) ResolveComment(ctx context.Context, args ResolveCommentArgs) (ResolveCommentResult, error) {
	var result ResolveCommentResult
	if err := c.callTool(ctx, "resolve_comment", args, &result); err != nil {
		return ResolveCommentResult{}, err
	}
	return result, nil
}

// Fork calls the fork MCP tool and returns the new ref name and SHA.
func (c *Client) Fork(ctx context.Context, args ForkArgs) (ForkResult, error) {
	var result ForkResult
	if err := c.callTool(ctx, "fork", args, &result); err != nil {
		return ForkResult{}, err
	}
	return result, nil
}

// QuerySessionState calls the query_session_state MCP tool.
func (c *Client) QuerySessionState(ctx context.Context, args QuerySessionStateArgs) (QuerySessionStateResult, error) {
	var result QuerySessionStateResult
	if err := c.callTool(ctx, "query_session_state", args, &result); err != nil {
		return QuerySessionStateResult{}, err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Internal JSON-RPC plumbing
// ---------------------------------------------------------------------------

type rpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// callTool marshals args to a JSON object, sends a tools/call JSON-RPC 2.0
// request to the portal's /mcp endpoint, and decodes the tool output text
// into out. Returns an error on any HTTP-, RPC-, decode-level, or tool-level
// failure.
//
// The portal's MCP handler responds in SSE format:
//
//	event: message
//	data: {"jsonrpc":"2.0","id":2,"result":{...}}
//
// The result object has the standard MCP tool result shape:
//
//	{"content":[{"type":"text","text":"{...output json...}"}],"isError":false}
//
// We extract content[0].text and unmarshal into out.
func (c *Client) callTool(ctx context.Context, name string, args any, out any) error {
	argsMap, err := toMap(args)
	if err != nil {
		return fmt.Errorf("mcpclient.%s: marshalling args: %w", name, err)
	}

	payload, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      name,
			"arguments": argsMap,
		},
	})
	if err != nil {
		return fmt.Errorf("mcpclient.%s: encoding request: %w", name, err)
	}

	endpoint := c.PortalURL + "/mcp"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("mcpclient.%s: building request: %w", name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.Bearer)
	if c.mcpSessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.mcpSessionID)
	}

	resp, err := c.hc().Do(req)
	if err != nil {
		return fmt.Errorf("mcpclient.%s: sending request: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mcpclient.%s: portal returned %d: %s", name, resp.StatusCode, body)
	}

	// Read the entire response body and parse SSE or plain JSON.
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("mcpclient.%s: reading response: %w", name, err)
	}

	// The MCP handler responds in SSE format when Accept includes text/event-stream.
	// Extract the JSON from the "data: " line.
	jsonData := raw
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			jsonData = []byte(strings.TrimPrefix(line, "data: "))
			break
		}
	}

	var rr rpcResponse
	if err := json.Unmarshal(jsonData, &rr); err != nil {
		return fmt.Errorf("mcpclient.%s: decoding response: %w — raw: %s", name, err, raw)
	}
	if rr.Error != nil {
		return fmt.Errorf("mcpclient.%s: RPC error %d: %s", name, rr.Error.Code, rr.Error.Message)
	}

	// The MCP result has shape:
	// {"content":[{"type":"text","text":"{...output json...}"}],"isError":bool}
	var toolResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(rr.Result, &toolResult); err != nil {
		return fmt.Errorf("mcpclient.%s: decoding tool result wrapper: %w — result: %s", name, err, rr.Result)
	}
	if toolResult.IsError {
		// Extract error message from content if available.
		errMsg := string(rr.Result)
		if len(toolResult.Content) > 0 {
			errMsg = toolResult.Content[0].Text
		}
		return fmt.Errorf("mcpclient.%s: tool returned error: %s", name, errMsg)
	}
	if len(toolResult.Content) == 0 {
		return fmt.Errorf("mcpclient.%s: empty content in tool result", name)
	}

	// Decode the text payload into the caller's output struct.
	if err := json.Unmarshal([]byte(toolResult.Content[0].Text), out); err != nil {
		return fmt.Errorf("mcpclient.%s: decoding tool output: %w — text: %s", name, err, toolResult.Content[0].Text)
	}
	return nil
}

// hc returns the HTTP client to use, defaulting to http.DefaultClient.
func (c *Client) hc() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// toMap round-trips v through JSON to produce a map[string]any suitable for
// use as the arguments field in an MCP tool call. Fields with zero-value
// omitempty tags are omitted by the marshaller.
func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
