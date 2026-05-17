// Package mcpclient provides a minimal JSON-RPC 2.0 client for calling MCP
// tools via the portal's /mcp HTTP endpoint.
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Client holds the connection parameters for the portal MCP endpoint.
type Client struct {
	// PortalURL is the portal origin, e.g. "https://jamsesh.example.com".
	// Trailing slash is allowed; "/mcp" is appended.
	PortalURL string
	// Token is the Bearer token sent in the Authorization header.
	Token string
	// HTTP is the underlying transport. If nil, http.DefaultClient is used.
	HTTP *http.Client
}

type rpcReq struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// CallTool invokes an MCP tool by name with the given arguments via JSON-RPC
// 2.0 POST to <PortalURL>/mcp. It returns the tool result's
// StructuredContent field, or an error if the call fails or the server
// returns an RPC error.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (json.RawMessage, error) {
	payload, err := json.Marshal(rpcReq{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      name,
			"arguments": args,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("mcpclient: encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.PortalURL+"/mcp", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("mcpclient: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mcpclient: MCP endpoint returned %d", resp.StatusCode)
	}

	var rr rpcResp
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, fmt.Errorf("mcpclient: decoding response: %w", err)
	}
	if rr.Error != nil {
		return nil, fmt.Errorf("mcpclient: RPC error %d: %s", rr.Error.Code, rr.Error.Message)
	}

	// The MCP SDK wraps tool results — extract StructuredContent.
	var wrapper struct {
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if err := json.Unmarshal(rr.Result, &wrapper); err != nil {
		return nil, fmt.Errorf("mcpclient: unwrapping tool result: %w", err)
	}
	return wrapper.StructuredContent, nil
}
