// Property: any JSON body sent to the four MCP tools (post_comment,
// resolve_comment, fork, query_session_state) either yields:
//   - 2xx with a valid tool response, OR
//   - 4xx with an MCP error envelope
//
// But NEVER 5xx (panic / unhandled error).
//
// The harness drives real HTTP POSTs to a live portal container, with a real
// bearer token and a real session. It does not stub the database or the MCP
// SDK's own schema validation — those are in-process. Only HTTP-level
// responses matter for this property: if the portal panics or produces an
// unhandled error it will return 5xx, which this test captures and fails on.
package fuzz_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// seedCase is one entry from testdata/mcp-seed-corpus.json.
type seedCase struct {
	Description string          `json:"description"`
	Tool        string          `json:"tool"`
	Args        json.RawMessage `json:"args"`
}

// rawToolCall is the shape of a tools/call JSON-RPC request.
type rawToolCall struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"params"`
}

// toolCallResult captures the HTTP response status and body for classification.
type toolCallResult struct {
	StatusCode int
	Body       []byte
}

// TestMCPToolInputFuzz is a property-based fuzz harness for the four MCP tools
// exposed by the jamsesh portal. It:
//
//  1. Starts the full stack (postgres, mailhog, portal).
//  2. Signs in via magic link to get a real bearer token.
//  3. Creates an org + session so session-scoped tools have something to act on.
//  4. Runs all hand-curated seed inputs from testdata/mcp-seed-corpus.json.
//  5. Then runs N randomly generated tool calls.
//
// For each call the only assertion is: status < 500. A 5xx response is a
// production bug — the portal should NEVER panic on arbitrary input.
//
// Skip with -short; control random iteration count via MCP_FUZZ_COUNT.
func TestMCPToolInputFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("fuzz: long-running, skip under -short")
	}

	ctx := context.Background()

	// ---------------------------------------------------------------------------
	// Stack setup
	// ---------------------------------------------------------------------------

	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		SMTPHost:  mh.ContainerSMTPHost,
		SMTPPort:  mh.ContainerSMTPPort,
	})

	// Sign in as the fuzz user.
	fuzzEmail := randEmail(t, "fuzz")
	bearer := authflow.SignInViaMagicLink(ctx, t, p, mh, fuzzEmail)

	// Create an org + session so session-scoped tool calls have a real target.
	orgID := authflow.CreateOrg(ctx, t, p, bearer.AccessToken, "Fuzz Org")
	sessionID := createFuzzSession(ctx, t, p, bearer.AccessToken, orgID)

	// callTool sends a raw MCP tools/call request and returns the HTTP result.
	// It performs its own initialize handshake to keep each call independent.
	callTool := func(tool string, argsJSON []byte) (toolCallResult, error) {
		return callRawTool(ctx, p.URL, bearer.AccessToken, tool, argsJSON)
	}

	// Replace placeholder UUIDs in seed corpus args with the real IDs.
	replacePlaceholders := func(raw json.RawMessage) json.RawMessage {
		if raw == nil {
			return raw
		}
		s := strings.ReplaceAll(string(raw),
			"00000000-0000-0000-0000-000000000001", sessionID)
		return json.RawMessage(s)
	}

	// ---------------------------------------------------------------------------
	// Phase 1: seed corpus
	// ---------------------------------------------------------------------------

	seedData, err := os.ReadFile("testdata/mcp-seed-corpus.json")
	if err != nil {
		t.Fatalf("fuzz: read seed corpus: %v", err)
	}
	var seeds []seedCase
	if err := json.Unmarshal(seedData, &seeds); err != nil {
		t.Fatalf("fuzz: parse seed corpus: %v", err)
	}

	for i, seed := range seeds {
		seed := seed // capture
		t.Run(fmt.Sprintf("seed_%02d_%s", i, sanitizeName(seed.Description)), func(t *testing.T) {
			args := replacePlaceholders(seed.Args)
			result, err := callTool(seed.Tool, args)
			if err != nil {
				// Transport / infrastructure errors are not a portal bug.
				t.Logf("transport error (seed %d %q): %v", i, seed.Description, err)
				return
			}
			if result.StatusCode >= 500 {
				t.Errorf("PANIC detected: seed=%d desc=%q tool=%s args=%s status=%d response=%s",
					i, seed.Description, seed.Tool, args, result.StatusCode, result.Body)
			}
		})
	}

	// ---------------------------------------------------------------------------
	// Phase 2: random property iterations
	// ---------------------------------------------------------------------------

	iterations := 200
	if v := os.Getenv("MCP_FUZZ_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			iterations = n
		}
	}

	// Use a deterministic seed derived from the current second so runs are
	// reproducible within a test invocation (logged below) but vary across runs.
	seed64 := time.Now().UnixNano()
	t.Logf("fuzz: random seed = %d (rerun with MCP_FUZZ_SEED=%d to reproduce)", seed64, seed64)
	if v := os.Getenv("MCP_FUZZ_SEED"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed64 = n
			t.Logf("fuzz: using provided seed %d", seed64)
		}
	}
	rng := mrand.New(mrand.NewPCG(uint64(seed64), 0xdeadbeef))

	for i := 0; i < iterations; i++ {
		i := i // capture
		tool, argsJSON := generateRandomToolCall(rng, sessionID)
		t.Run(fmt.Sprintf("rand_%04d", i), func(t *testing.T) {
			result, err := callTool(tool, argsJSON)
			if err != nil {
				t.Logf("transport error (rand iter %d): %v", i, err)
				return
			}
			if result.StatusCode >= 500 {
				t.Errorf("PANIC detected: iter=%d tool=%s args=%s status=%d response=%s",
					i, tool, argsJSON, result.StatusCode, result.Body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Raw HTTP MCP caller (no typed wrappers — sends whatever JSON we give it)
// ---------------------------------------------------------------------------

// callRawTool performs the MCP initialize handshake, then sends a raw
// tools/call JSON-RPC request with argsJSON as the arguments. It returns the
// HTTP status and body without any error wrapping so the caller can inspect 4xx
// vs 5xx independently.
//
// It opens a fresh HTTP session per call so each random iteration is
// independent; we accept the per-call overhead since correctness > speed here.
func callRawTool(ctx context.Context, portalURL, bearer, toolName string, argsJSON json.RawMessage) (toolCallResult, error) {
	hc := &http.Client{Timeout: 30 * time.Second}
	mcpURL := portalURL + "/mcp"

	// 1. Initialize handshake.
	initPayload, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "fuzz-client", "version": "0.0.1"},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpURL, bytes.NewReader(initPayload))
	if err != nil {
		return toolCallResult{}, fmt.Errorf("build init request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := hc.Do(req)
	if err != nil {
		return toolCallResult{}, fmt.Errorf("send init: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode >= 500 {
		return toolCallResult{StatusCode: resp.StatusCode}, nil
	}
	mcpSessionID := resp.Header.Get("Mcp-Session-Id")

	// 2. notifications/initialized (fire-and-forget).
	notifPayload, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})
	nreq, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpURL, bytes.NewReader(notifPayload))
	if err == nil {
		nreq.Header.Set("Content-Type", "application/json")
		nreq.Header.Set("Accept", "application/json, text/event-stream")
		nreq.Header.Set("Authorization", "Bearer "+bearer)
		if mcpSessionID != "" {
			nreq.Header.Set("Mcp-Session-Id", mcpSessionID)
		}
		nresp, nerr := hc.Do(nreq)
		if nerr == nil {
			io.Copy(io.Discard, nresp.Body) //nolint:errcheck
			nresp.Body.Close()
		}
	}

	// 3. Build the tools/call payload. argsJSON may be nil (null) or arbitrary
	// JSON — we embed it verbatim so we can send malformed shapes.
	argsField := argsJSON
	if argsField == nil {
		argsField = json.RawMessage(`null`)
	}

	callPayload, _ := json.Marshal(rawToolCall{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}{
			Name:      toolName,
			Arguments: argsField,
		},
	})

	treq, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpURL, bytes.NewReader(callPayload))
	if err != nil {
		return toolCallResult{}, fmt.Errorf("build tool call request: %w", err)
	}
	treq.Header.Set("Content-Type", "application/json")
	treq.Header.Set("Accept", "application/json, text/event-stream")
	treq.Header.Set("Authorization", "Bearer "+bearer)
	if mcpSessionID != "" {
		treq.Header.Set("Mcp-Session-Id", mcpSessionID)
	}

	tresp, err := hc.Do(treq)
	if err != nil {
		return toolCallResult{}, fmt.Errorf("send tool call: %w", err)
	}
	defer tresp.Body.Close()
	body, _ := io.ReadAll(tresp.Body)

	return toolCallResult{StatusCode: tresp.StatusCode, Body: body}, nil
}

// ---------------------------------------------------------------------------
// Random input generator
// ---------------------------------------------------------------------------

var toolNames = []string{
	"post_comment",
	"resolve_comment",
	"fork",
	"query_session_state",
}

// garbageToolNames are used occasionally to test unknown-tool handling.
var garbageToolNames = []string{
	"does_not_exist",
	"",
	"__proto__",
	"admin",
	"../../etc/passwd",
}

// generateRandomToolCall picks a tool and generates a random JSON argument
// object for it. It exercises:
//   - valid-ish shapes (correct field names, plausible types)
//   - missing required fields
//   - wrong types (string where int expected, array where object expected)
//   - boundary values (empty string, huge string, negative integers, MaxInt64)
//   - unicode, null bytes, control characters
//   - unknown tool names (garbage)
func generateRandomToolCall(rng *mrand.Rand, realSessionID string) (tool string, argsJSON []byte) {
	// 5% chance: garbage tool name.
	if rng.IntN(100) < 5 {
		tool = garbageToolNames[rng.IntN(len(garbageToolNames))]
		// Still generate a plausible args map.
		args := map[string]any{"session_id": realSessionID}
		b, _ := json.Marshal(args)
		return tool, b
	}

	tool = toolNames[rng.IntN(len(toolNames))]

	switch tool {
	case "post_comment":
		argsJSON = genPostCommentArgs(rng, realSessionID)
	case "resolve_comment":
		argsJSON = genResolveCommentArgs(rng, realSessionID)
	case "fork":
		argsJSON = genForkArgs(rng, realSessionID)
	case "query_session_state":
		argsJSON = genQuerySessionStateArgs(rng, realSessionID)
	default:
		b, _ := json.Marshal(map[string]any{"session_id": realSessionID})
		argsJSON = b
	}
	return tool, argsJSON
}

func genPostCommentArgs(rng *mrand.Rand, sessionID string) []byte {
	args := map[string]any{}

	// session_id: sometimes real, sometimes garbage, sometimes missing.
	switch rng.IntN(4) {
	case 0:
		args["session_id"] = sessionID
	case 1:
		args["session_id"] = randString(rng, 0, 64)
	case 2:
		args["session_id"] = randWrongType(rng)
	// case 3: omit (missing required field)
	}

	// commit_sha: sometimes real-ish hex, sometimes garbage, sometimes missing.
	switch rng.IntN(4) {
	case 0:
		args["commit_sha"] = randHex(rng, 40)
	case 1:
		args["commit_sha"] = randString(rng, 0, 256)
	case 2:
		args["commit_sha"] = randWrongType(rng)
	// case 3: omit
	}

	// body: sometimes present, sometimes missing.
	if rng.IntN(3) != 0 {
		args["body"] = randString(rng, 0, 8192)
	}

	// Optional fields.
	if rng.IntN(3) == 0 {
		args["file_path"] = randFilePath(rng)
	}
	if rng.IntN(4) == 0 {
		args["line_start"] = randLineNum(rng)
	}
	if rng.IntN(4) == 0 {
		args["line_end"] = randLineNum(rng)
	}
	if rng.IntN(4) == 0 {
		args["addressed_to"] = randAddressedTo(rng)
	}
	if rng.IntN(4) == 0 {
		args["kind"] = randCommentKind(rng)
	}

	b, _ := json.Marshal(args)
	return b
}

func genResolveCommentArgs(rng *mrand.Rand, sessionID string) []byte {
	args := map[string]any{}

	switch rng.IntN(4) {
	case 0:
		args["session_id"] = sessionID
	case 1:
		args["session_id"] = randString(rng, 0, 64)
	case 2:
		args["session_id"] = randWrongType(rng)
	}

	switch rng.IntN(4) {
	case 0:
		args["comment_id"] = randUUID(rng)
	case 1:
		args["comment_id"] = randString(rng, 0, 64)
	case 2:
		args["comment_id"] = randWrongType(rng)
	}

	if rng.IntN(3) == 0 {
		args["resolution_note"] = randString(rng, 0, 1024)
	}

	b, _ := json.Marshal(args)
	return b
}

func genForkArgs(rng *mrand.Rand, sessionID string) []byte {
	args := map[string]any{}

	switch rng.IntN(4) {
	case 0:
		args["session_id"] = sessionID
	case 1:
		args["session_id"] = randString(rng, 0, 64)
	case 2:
		args["session_id"] = randWrongType(rng)
	}

	switch rng.IntN(4) {
	case 0:
		args["target_commit_sha"] = randHex(rng, 40)
	case 1:
		args["target_commit_sha"] = randString(rng, 0, 256)
	case 2:
		args["target_commit_sha"] = randWrongType(rng)
	}

	if rng.IntN(3) == 0 {
		args["target_ref"] = randRefName(rng)
	}
	if rng.IntN(4) == 0 {
		args["mode"] = randForkMode(rng)
	}

	b, _ := json.Marshal(args)
	return b
}

func genQuerySessionStateArgs(rng *mrand.Rand, sessionID string) []byte {
	args := map[string]any{}

	switch rng.IntN(4) {
	case 0:
		args["session_id"] = sessionID
	case 1:
		args["session_id"] = randString(rng, 0, 64)
	case 2:
		args["session_id"] = randWrongType(rng)
	}

	if rng.IntN(3) == 0 {
		args["since_seq"] = randSeq(rng)
	}
	if rng.IntN(3) == 0 {
		args["include"] = randIncludeList(rng)
	}

	b, _ := json.Marshal(args)
	return b
}

// ---------------------------------------------------------------------------
// Value generators
// ---------------------------------------------------------------------------

// randString returns a random string of length in [minLen, maxLen).
// The string may contain unicode, null bytes, and control characters.
func randString(rng *mrand.Rand, minLen, maxLen int) string {
	length := minLen
	if maxLen > minLen {
		length = minLen + rng.IntN(maxLen-minLen)
	}
	if length == 0 {
		return ""
	}

	// Pick a character set based on rng.
	switch rng.IntN(5) {
	case 0:
		// ASCII printable only.
		const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 !@#$%^&*()-_=+[]{};:'\",.<>?/\\|`~"
		b := make([]byte, length)
		for i := range b {
			b[i] = charset[rng.IntN(len(charset))]
		}
		return string(b)
	case 1:
		// Unicode code points including emoji.
		runes := []rune("abcABC 🎉🔥💥⚡🌍αβγδ中文日本語한국어العربية")
		b := make([]rune, length)
		for i := range b {
			b[i] = runes[rng.IntN(len(runes))]
		}
		return string(b)
	case 2:
		// Binary / control characters including null bytes.
		b := make([]byte, length)
		for i := range b {
			b[i] = byte(rng.IntN(32)) // 0x00–0x1f (control range)
		}
		return string(b)
	case 3:
		// All zeros (null bytes).
		return strings.Repeat("\x00", length)
	default:
		// Mix of valid and boundary characters.
		b := make([]byte, length)
		for i := range b {
			b[i] = byte(rng.IntN(256))
		}
		return string(b)
	}
}

// randHex returns a random hex string of exactly n characters.
func randHex(rng *mrand.Rand, n int) string {
	const hexChars = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = hexChars[rng.IntN(len(hexChars))]
	}
	return string(b)
}

// randUUID returns a random UUID-shaped string (not necessarily valid).
func randUUID(rng *mrand.Rand) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rng.Uint32(), rng.IntN(0xffff), rng.IntN(0xffff),
		rng.IntN(0xffff), rng.Uint64()&0xffffffffffff)
}

// randWrongType returns an unexpected JSON type: array, object, boolean, number, or null.
func randWrongType(rng *mrand.Rand) any {
	switch rng.IntN(6) {
	case 0:
		return []any{1, 2, 3}
	case 1:
		return map[string]any{"nested": "object"}
	case 2:
		return true
	case 3:
		return false
	case 4:
		return nil
	default:
		return rng.IntN(1000000)
	}
}

// randLineNum returns a line number value — valid, zero, negative, or huge.
func randLineNum(rng *mrand.Rand) any {
	switch rng.IntN(5) {
	case 0:
		return rng.IntN(10000) + 1 // valid 1-based
	case 1:
		return 0
	case 2:
		return -rng.IntN(1000) // negative
	case 3:
		return int64(^uint64(0) >> 1) // MaxInt64
	default:
		return randString(rng, 0, 10) // wrong type
	}
}

// randSeq returns a since_seq value: valid, zero, negative, huge, or wrong type.
func randSeq(rng *mrand.Rand) any {
	switch rng.IntN(5) {
	case 0:
		return int64(rng.IntN(1000))
	case 1:
		return int64(0)
	case 2:
		return int64(-rng.IntN(100))
	case 3:
		return int64(^uint64(0) >> 1)
	default:
		return randString(rng, 0, 10)
	}
}

// randFilePath returns a file path: valid-ish, path traversal, empty, or huge.
func randFilePath(rng *mrand.Rand) string {
	switch rng.IntN(5) {
	case 0:
		return "src/main.go"
	case 1:
		return "../../../../etc/passwd"
	case 2:
		return ""
	case 3:
		return strings.Repeat("a/", 500) + "file.go"
	default:
		return randString(rng, 0, 256)
	}
}

// randRefName returns a git ref name: valid, path traversal, or garbage.
func randRefName(rng *mrand.Rand) string {
	switch rng.IntN(4) {
	case 0:
		return "my-feature-branch"
	case 1:
		return "../../refs/heads/master"
	case 2:
		return "refs/heads/" + randString(rng, 1, 64)
	default:
		return randString(rng, 0, 128)
	}
}

// randForkMode returns a fork mode value: valid or garbage.
func randForkMode(rng *mrand.Rand) any {
	switch rng.IntN(4) {
	case 0:
		return "sync"
	case 1:
		return "isolated"
	case 2:
		return ""
	default:
		return randString(rng, 0, 32)
	}
}

// randAddressedTo returns an addressed_to value.
func randAddressedTo(rng *mrand.Rand) string {
	switch rng.IntN(5) {
	case 0:
		return "@all-agents"
	case 1:
		return "@everyone"
	case 2:
		return "user@example.com"
	case 3:
		return ""
	default:
		return "@" + randString(rng, 0, 64)
	}
}

// randCommentKind returns a comment kind value: valid or garbage.
func randCommentKind(rng *mrand.Rand) string {
	valid := []string{"question", "suggestion", "action-request", "fyi"}
	switch rng.IntN(3) {
	case 0:
		return valid[rng.IntN(len(valid))]
	case 1:
		return ""
	default:
		return randString(rng, 0, 32)
	}
}

// randIncludeList returns an include list for query_session_state.
func randIncludeList(rng *mrand.Rand) any {
	valid := []string{"goal", "scope", "draft_tip", "unresolved_comments", "open_conflicts", "recent_events"}
	switch rng.IntN(4) {
	case 0:
		// Pick a random subset.
		n := rng.IntN(len(valid) + 1)
		if n == 0 {
			return []string{}
		}
		result := make([]string, n)
		for i := range result {
			result[i] = valid[rng.IntN(len(valid))]
		}
		return result
	case 1:
		// All valid.
		return valid
	case 2:
		// Wrong type for the whole field.
		return randWrongType(rng)
	default:
		// Garbage strings in the list.
		return []string{randString(rng, 0, 32), randString(rng, 0, 32)}
	}
}

// ---------------------------------------------------------------------------
// Test infrastructure helpers
// ---------------------------------------------------------------------------

// randEmail returns a unique-per-run email address safe for parallel tests.
func randEmail(t *testing.T, prefix string) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("randEmail: rand.Read: %v", err)
	}
	return prefix + "-" + hex.EncodeToString(b) + "@example.com"
}

// createFuzzSession calls POST /api/orgs/{orgID}/sessions and returns the new
// session's ID. Copied from golden tests to avoid a cross-package dependency on
// test-only helpers.
func createFuzzSession(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID string) string {
	t.Helper()
	body := map[string]string{
		"name":         "fuzz-session",
		"goal":         "Fuzz harness session",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("createFuzzSession: marshal: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", p.URL, orgID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("createFuzzSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("createFuzzSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("createFuzzSession: status %d (want 201): %s", resp.StatusCode, respBody)
	}
	var s struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("createFuzzSession: decode: %v\nbody: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("createFuzzSession: empty id in response")
	}
	return s.ID
}

// sanitizeName converts a human-readable description into a safe test name.
func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	name := b.String()
	if len(name) > 60 {
		name = name[:60]
	}
	return name
}
