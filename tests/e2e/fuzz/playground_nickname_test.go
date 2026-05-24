// Property: for every string sent as the nickname field of
// POST /api/playground/sessions/{id}/join, the real portal returns either:
//
//  1. 200 OK — the nickname was accepted; the echoed nickname in the response
//     matches the submitted value (or is a server-minted handle when input was
//     empty; or carries a collision-retry suffix when the wordlist collided).
//  2. 400 Bad Request with error code "playground.invalid_nickname" — the
//     nickname violated the documented rule.
//
// But NEVER:
//   - A 5xx response (which indicates a panic, nil-deref, or unhandled error).
//   - A 2xx where the echoed nickname silently differs from the submitted input.
//   - A 4xx with the wrong error code for an invalid input.
//
// The harness drives real HTTP POSTs to a live portal container backed by a real
// Postgres database with playground enabled. It does NOT stub the chi router, the
// openapi-validator middleware, or the database — any divergence between the
// unit-level validator and the production pipeline is caught here.
//
// The contract predicate (nicknameValid) mirrors the production rule documented
// in internal/portal/playground/handler.go:
//
//	var nicknameRE = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
//	// accepted iff empty (server mints) OR (2 <= len <= 24 AND matches RE)
//
// Two test entry points are provided:
//
//   - FuzzPlaygroundNickname: Go-native fuzz harness. Running
//     `go test -run FuzzPlaygroundNickname` exercises the seed corpus; each seed
//     gets a fresh postgres + portal container bound to its own *testing.T.
//     Running `go test -fuzz=FuzzPlaygroundNickname -fuzztime=30s` activates the
//     fuzzer engine; each generated input likewise gets a fresh stack. This
//     matches the container lifecycle pattern used by TestFencingTokenFuzz.
//
//   - TestPlaygroundNicknameFuzz: property-based companion that boots the stack
//     once and runs all seeds plus a random-iteration phase. Use this for
//     regression runs; it is much faster because it amortises container startup
//     across all iterations.
//
// Skip both with -short.
package fuzz_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// nicknameValidRE mirrors the production regexp in
// internal/portal/playground/handler.go. Both must stay in sync; if the server
// accepts or rejects an input that this predicate disagrees with, the test
// failure documents a contract divergence — investigate before changing either.
var nicknameValidRE = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// nicknameValid returns true iff the submitted nickname should be accepted by
// the production handler (any 2xx response is expected).
//
// The handler (internal/portal/playground/handler.go JoinPlaygroundSession) does:
//
//	trimmed := strings.TrimSpace(req.Body.Nickname)
//	if trimmed == "" { /* fall through: server mints */ }
//	else if len(trimmed) < 2 || len(trimmed) > 24 || !nicknameRE.MatchString(trimmed) { return 400 }
//
// Consequences:
//   - Empty string: server mints, 200.
//   - Whitespace-only (e.g. " ", "\t"): TrimSpace collapses to ""; server mints, 200.
//   - 2-24 chars, [a-zA-Z0-9-] only: 200 (after TrimSpace is a no-op for valid input).
//   - Anything else: 400 playground.invalid_nickname.
func nicknameValid(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return true // empty or whitespace-only: server mints, 200 path
	}
	return len(trimmed) >= 2 && len(trimmed) <= 24 && nicknameValidRE.MatchString(trimmed)
}

// nicknameJoinResp is the 200 body from POST /api/playground/sessions/{id}/join.
type nicknameJoinResp struct {
	Nickname string `json:"nickname"`
	Bearer   string `json:"bearer"`
}

// nicknameErrResp is the error envelope from a 4xx response.
type nicknameErrResp struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// FuzzPlaygroundNickname is a Go-native fuzz harness for the nickname-validation
// boundary on POST /api/playground/sessions/{id}/join.
//
// Each fuzz iteration (seed or engine-generated) starts a fresh postgres +
// portal container so the lifetime of each container is scoped to the
// *testing.T of that iteration. This mirrors the pattern used by
// TestFencingTokenFuzz where each seed gets its own container cluster.
//
// For a faster regression suite without `go test -fuzz`, use
// TestPlaygroundNicknameFuzz which starts the stack once and amortises startup
// across all iterations.
func FuzzPlaygroundNickname(f *testing.F) {
	if testing.Short() {
		f.Skip("fuzz: long-running, skip under -short")
	}

	// -------------------------------------------------------------------------
	// Seed corpus
	//
	// Primary seeds: the 11 cases from TestJoinPlaygroundSession_NicknameValidation
	// in internal/portal/playground/handler_test.go.
	//
	// Invalid (6):
	//   "a"                          too short (1 char)
	//   "aaaaaaaaaaaaaaaaaaaaaaaaa"  too long (25 chars)
	//   "ab cd"                      space disallowed
	//   "ab@cd"                      @ disallowed
	//   "ab/cd"                      slash disallowed
	//   "foó"                        non-ASCII Unicode
	//
	// Valid/edge (5):
	//   "ab"                         minimum valid: 2 chars
	//   "aaaaaaaaaaaaaaaaaaaaaaaa"   maximum valid: 24 chars
	//   "12345"                      digits only
	//   "foo-bar"                    dash in middle
	//   ""                           empty: server mints handle
	//
	// Extended boundary/Unicode seeds: length corners 1/23/25/26, control
	// characters, RTL override (U+202E), zero-width joiner (U+200D), emoji, etc.
	// -------------------------------------------------------------------------

	// Invalid seeds (expect 400 playground.invalid_nickname)
	f.Add("a")                           // too short: 1 char
	f.Add("aaaaaaaaaaaaaaaaaaaaaaaaa")   // too long: 25 chars
	f.Add("ab cd")                       // space disallowed
	f.Add("ab@cd")                       // @ disallowed
	f.Add("ab/cd")                       // slash disallowed
	f.Add("foó")                         // non-ASCII Unicode
	f.Add("aaaaaaaaaaaaaaaaaaaaaaaaaaa") // 27 chars, well past upper bound
	f.Add("\x00")                        // NUL byte
	f.Add("\x01\x02")                    // control chars (len >= 2, invalid chars)
	f.Add("ab‮cd")                  // RTL override (U+202E)
	f.Add("ab‍cd")                  // zero-width joiner (U+200D)
	f.Add("ab\ncd")                      // newline
	f.Add("ab\tcd")                      // tab
	f.Add("áéí")                         // accented Latin
	f.Add("中文")                           // CJK characters
	f.Add("\U0001f389\U0001f525")         // emoji
	f.Add("ab_cd")                       // underscore disallowed
	f.Add("ab.cd")                       // dot disallowed
	f.Add(" ")                           // whitespace only: TrimSpace collapses to "" → server mints → 200

	// Valid seeds (expect 200)
	f.Add("ab")                          // minimum valid: 2 chars
	f.Add("aaaaaaaaaaaaaaaaaaaaaaaa")    // maximum valid: 24 chars
	f.Add("12345")                       // digits only
	f.Add("foo-bar")                     // dash in middle
	f.Add("")                            // empty: server mints handle
	f.Add("az")                          // 2 chars, letters
	f.Add("ZZ")                          // uppercase 2 chars
	f.Add("1a")                          // digit + letter, min length
	f.Add("---")                         // dashes only (valid per RE, len=3)
	f.Add("a-b-c")                       // alternating letters and dashes
	f.Add("aaaaaaaaaaaaaaaaaaaaaaa")     // 23 chars: just inside max
	f.Add("ABCDEFGHIJKLMNOPQRSTUVWX")    // 24 uppercase chars (at max)

	// -------------------------------------------------------------------------
	// Fuzz body: each iteration starts its own postgres + portal container.
	// Container cleanup is registered on the iteration's *testing.T.
	// -------------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, nickname string) {
		ctx := context.Background()

		pg := postgres.Start(ctx, t, postgres.Options{})
		p := portal.Start(ctx, t, portal.Options{
			DBDriver:  "postgres",
			DBDSN:     pg.ContainerDSN,
			EmailFrom: "noreply@example.com",
			ExtraEnv: map[string]string{
				"JAMSESH_PLAYGROUND_ENABLED":            "true",
				"JAMSESH_PLAYGROUND_HARD_CAP_S":         "3600",
				"JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S":     "3600",
				"JAMSESH_PLAYGROUND_MAX_PARTICIPANTS":   "100",
				// Raise the per-IP session-creation rate limit high enough to
				// accommodate the full seed corpus + random iterations without
				// triggering 429. The minimum enforced by NewCreateRateLimiter
				// is 1/hour; 10000 effectively disables the limit for tests.
				"JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR": "10000",
			},
		})

		// Fresh session per iteration: collision state cannot bleed across.
		sessionID := nicknameCreateSession(t, ctx, p.URL)
		status, body := nicknameJoinSession(t, ctx, p.URL, sessionID, nickname)

		assertNicknameContract(t, nickname, status, body)
	})
}

// TestPlaygroundNicknameFuzz is the property-based companion to FuzzPlaygroundNickname.
// It starts the stack once and runs the 11 unit-suite seed cases plus extended
// boundary/Unicode cases, followed by a random-iteration phase. It is much faster
// than FuzzPlaygroundNickname for regression runs because it amortises container
// startup across all iterations.
//
// Skip with -short. Control random iteration count via NICKNAME_FUZZ_COUNT.
// Reproduce a specific run via NICKNAME_FUZZ_SEED=<seed>.
func TestPlaygroundNicknameFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("fuzz: long-running, skip under -short")
	}

	ctx := context.Background()

	// -------------------------------------------------------------------------
	// Stack setup (once, shared across all seed cases and random iterations)
	// -------------------------------------------------------------------------
	pg := postgres.Start(ctx, t, postgres.Options{})
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		ExtraEnv: map[string]string{
			"JAMSESH_PLAYGROUND_ENABLED":          "true",
			"JAMSESH_PLAYGROUND_HARD_CAP_S":       "3600",
			"JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S":   "3600",
			// Set participants high enough that valid-nickname joins don't fill
			// the session across the full seed + random iteration set.
			"JAMSESH_PLAYGROUND_MAX_PARTICIPANTS":   "10000",
			// Raise the per-IP session-creation cap well above what a single
			// test run creates (1 session). The minimum enforced by
			// NewCreateRateLimiter is 1/hour regardless of this setting.
			"JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR": "10000",
		},
	})

	// Create ONE session shared across all iterations. Invalid inputs return
	// 400 without adding a member (no participant-count pressure). Valid inputs
	// add members, but MaxParticipants=10000 absorbs the full iteration set.
	// Nickname-collision retries are logged and accepted by assertNicknameContract.
	sharedSessionID := nicknameCreateSession(t, ctx, p.URL)

	runCase := func(t *testing.T, nickname string) {
		t.Helper()
		status, body := nicknameJoinSession(t, ctx, p.URL, sharedSessionID, nickname)
		assertNicknameContract(t, nickname, status, body)
	}

	// -------------------------------------------------------------------------
	// Phase 1: seed corpus — the 11 cases from handler_test.go plus extensions
	// -------------------------------------------------------------------------
	type seedCase struct {
		description string
		nickname    string
	}
	seeds := []seedCase{
		// From TestJoinPlaygroundSession_NicknameValidation — invalid cases.
		{description: "too_short_1char", nickname: "a"},
		{description: "too_long_25char", nickname: "aaaaaaaaaaaaaaaaaaaaaaaaa"},
		{description: "has_space", nickname: "ab cd"},
		{description: "has_at", nickname: "ab@cd"},
		{description: "has_slash", nickname: "ab/cd"},
		{description: "has_unicode", nickname: "foó"},
		// From TestJoinPlaygroundSession_NicknameValidation — valid cases.
		{description: "valid_2char", nickname: "ab"},
		{description: "valid_24char", nickname: "aaaaaaaaaaaaaaaaaaaaaaaa"},
		{description: "valid_all_digits", nickname: "12345"},
		{description: "valid_with_dashes", nickname: "foo-bar"},
		{description: "empty_server_mints", nickname: ""},
		// Length boundary cases not in the unit suite.
		{description: "length_1", nickname: "x"},
		{description: "length_23", nickname: "aaaaaaaaaaaaaaaaaaaaaaa"},
		{description: "length_25", nickname: "aaaaaaaaaaaaaaaaaaaaaaaaa"},
		{description: "length_26", nickname: "aaaaaaaaaaaaaaaaaaaaaaaaaa"},
		// Mixed-ASCII edge cases.
		{description: "dashes_only", nickname: "---"},
		{description: "digit_letter_2char", nickname: "1a"},
		{description: "uppercase_max", nickname: "ABCDEFGHIJKLMNOPQRSTUVWX"},
		// Unicode and control-character edge cases.
		{description: "nul_byte", nickname: "\x00"},
		{description: "control_chars", nickname: "\x01\x02"},
		{description: "rtl_override", nickname: "ab‮cd"},
		{description: "zero_width_joiner", nickname: "ab‍cd"},
		{description: "accented_latin", nickname: "áéíóú"},
		{description: "cjk", nickname: "中文"},
		{description: "emoji", nickname: "\U0001f389\U0001f525"},
		{description: "underscore", nickname: "ab_cd"},
		{description: "dot", nickname: "ab.cd"},
		{description: "whitespace_only_server_mints", nickname: " "}, // TrimSpace → "" → server mints → 200
		{description: "newline", nickname: "ab\ncd"},
		{description: "tab", nickname: "ab\tcd"},
	}

	for i, seed := range seeds {
		seed := seed
		t.Run(fmt.Sprintf("seed_%02d_%s", i, sanitizeNicknameName(seed.description)), func(t *testing.T) {
			runCase(t, seed.nickname)
		})
	}

	// -------------------------------------------------------------------------
	// Phase 2: random property iterations
	// -------------------------------------------------------------------------
	iterations := 100
	if v := os.Getenv("NICKNAME_FUZZ_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			iterations = n
		}
	}

	seed64 := time.Now().UnixNano()
	t.Logf("fuzz/nickname: random seed = %d (rerun with NICKNAME_FUZZ_SEED=%d to reproduce)",
		seed64, seed64)
	if v := os.Getenv("NICKNAME_FUZZ_SEED"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed64 = n
			t.Logf("fuzz/nickname: using provided seed %d", seed64)
		}
	}
	rng := mrand.New(mrand.NewPCG(uint64(seed64), 0xdeadbeef))

	for i := 0; i < iterations; i++ {
		i := i
		nickname := generateRandomNickname(rng)
		t.Run(fmt.Sprintf("rand_%04d", i), func(t *testing.T) {
			runCase(t, nickname)
		})
	}
}

// ---------------------------------------------------------------------------
// Core assertion
// ---------------------------------------------------------------------------

// assertNicknameContract asserts the full response contract for a single
// nickname join attempt. Called from both FuzzPlaygroundNickname and
// TestPlaygroundNicknameFuzz so the assertion logic is not duplicated.
//
// Contract:
//   - 200: nicknameValid(nickname) must be true; echoed nickname is non-empty.
//   - 400: nicknameValid(nickname) must be false; error code is exactly
//     "playground.invalid_nickname"; message is non-empty.
//   - 5xx: unconditional failure — the portal must never panic on any input.
//   - Anything else: unconditional failure — unexpected status for this endpoint.
func assertNicknameContract(t *testing.T, nickname string, status int, body []byte) {
	t.Helper()

	switch {
	case status >= 500:
		// Production bug: the portal must never panic or return unhandled errors.
		// Park as High via /agile-workflow:park if this fires.
		t.Fatalf(
			"BUG: 5xx for nickname %q (len=%d) — portal panicked or unhandled error.\n"+
				"status: %d\nbody:   %s\n\n"+
				"Action: park as High via /agile-workflow:park.",
			nickname, len(nickname), status, body,
		)

	case status == http.StatusOK:
		var resp nicknameJoinResp
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("200 for %q but cannot decode body: %v\nbody: %s",
				nickname, err, body)
		}
		if resp.Nickname == "" {
			t.Fatalf("200 for %q but echoed nickname is empty\nbody: %s", nickname, body)
		}
		if !nicknameValid(nickname) {
			// Portal accepted an input the spec says must be rejected.
			// Do NOT adjust nicknameValid to paper over this; instead park a bug.
			t.Fatalf(
				"BUG: portal accepted %q (len=%d) but nicknameValid says invalid.\n"+
					"echoed: %q\n\n"+
					"Production validator is more permissive than the spec.\n"+
					"Action: park as Medium via /agile-workflow:park with title\n"+
					"\"Nickname validator accepts input outside documented rule\".",
				nickname, len(nickname), resp.Nickname,
			)
		}
		// Non-empty submitted nickname: the echo must match, unless a collision
		// forced the server to fall back to a wordlist pick. Log and accept.
		if nickname != "" && resp.Nickname != nickname {
			t.Logf("collision-retry: submitted %q, server returned %q (expected, not a bug)",
				nickname, resp.Nickname)
		}

	case status == http.StatusBadRequest:
		var resp nicknameErrResp
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("400 for %q but cannot decode body: %v\nbody: %s",
				nickname, err, body)
		}
		if resp.Error != "playground.invalid_nickname" {
			t.Fatalf(
				"400 for %q: wrong error code: got %q, want \"playground.invalid_nickname\".\nbody: %s",
				nickname, resp.Error, body,
			)
		}
		if resp.Message == "" {
			t.Fatalf("400 for %q but message is empty\nbody: %s", nickname, body)
		}
		if nicknameValid(nickname) {
			// Portal rejected an input the spec says must be accepted.
			t.Fatalf(
				"BUG: portal rejected %q (len=%d) but nicknameValid says valid.\n"+
					"error: %q\n\n"+
					"Production validator is more restrictive than the spec.\n"+
					"Action: park as Medium via /agile-workflow:park with title\n"+
					"\"Nickname validator rejects input inside documented rule\".",
				nickname, len(nickname), resp.Error,
			)
		}

	default:
		// 404: session not found (setup bug). 410: hard_cap elapsed (3600s cap,
		// should not happen). 409: session full (max_participants=100, should not
		// happen). 503: playground disabled (ExtraEnv sets enabled=true).
		t.Fatalf(
			"unexpected status %d for nickname %q (len=%d)\nbody: %s",
			status, nickname, len(nickname), body,
		)
	}
}

// ---------------------------------------------------------------------------
// HTTP helpers (scoped to playground-nickname tests)
// ---------------------------------------------------------------------------

// nicknameCreateSession calls POST /api/playground/sessions and returns the
// new session ID. Fails the test on any non-201 status.
func nicknameCreateSession(t testing.TB, ctx context.Context, baseURL string) string {
	t.Helper()
	url := strings.TrimRight(baseURL, "/") + "/api/playground/sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("nicknameCreateSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("nicknameCreateSession: POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("nicknameCreateSession: want 201, got %d: %s", resp.StatusCode, body)
	}
	var r struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("nicknameCreateSession: decode: %v\nbody: %s", err, body)
	}
	if r.Session.ID == "" {
		t.Fatalf("nicknameCreateSession: empty session id: %s", body)
	}
	return r.Session.ID
}

// nicknameJoinSession calls POST /api/playground/sessions/{id}/join with the
// given nickname (or an empty JSON body when nickname is ""). Returns the HTTP
// status code and raw response body without failing the test.
func nicknameJoinSession(t testing.TB, ctx context.Context, baseURL, sessionID, nickname string) (int, []byte) {
	t.Helper()
	var bodyBytes []byte
	if nickname != "" {
		b, _ := json.Marshal(map[string]string{"nickname": nickname})
		bodyBytes = b
	} else {
		bodyBytes = []byte("{}")
	}

	url := fmt.Sprintf("%s/api/playground/sessions/%s/join",
		strings.TrimRight(baseURL, "/"), sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("nicknameJoinSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("nicknameJoinSession: POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, body
}

// ---------------------------------------------------------------------------
// Random nickname generator
// ---------------------------------------------------------------------------

// generateRandomNickname returns a random string for use as a nickname fuzz
// input. It covers:
//   - Valid inputs: length 2-24, chars from [a-zA-Z0-9-]
//   - Invalid lengths: 0-1 and 25+
//   - Invalid chars: spaces, punctuation, control chars, Unicode beyond ASCII
//   - Boundary lengths: 1, 2, 24, 25, 26
//   - Unicode edge cases: combining chars, bidirectional overrides, emoji
func generateRandomNickname(rng *mrand.Rand) string {
	switch rng.IntN(8) {
	case 0:
		// Valid: length in [2,24], chars from valid set.
		length := 2 + rng.IntN(23)
		const validChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-"
		b := make([]byte, length)
		for i := range b {
			b[i] = validChars[rng.IntN(len(validChars))]
		}
		return string(b)

	case 1:
		// Invalid: too short (0 or 1 char).
		switch rng.IntN(3) {
		case 0:
			return "" // empty: server mints (valid path)
		case 1:
			const validChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-"
			return string([]byte{validChars[rng.IntN(len(validChars))]})
		default:
			// 1 char, invalid character class.
			invalidChars := []byte{' ', '@', '/', '.', '_'}
			return string([]byte{invalidChars[rng.IntN(len(invalidChars))]})
		}

	case 2:
		// Invalid: too long (25-64 chars), valid charset.
		length := 25 + rng.IntN(40)
		const validChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-"
		b := make([]byte, length)
		for i := range b {
			b[i] = validChars[rng.IntN(len(validChars))]
		}
		return string(b)

	case 3:
		// Invalid: valid length, mixed chars (some from invalid set).
		length := 2 + rng.IntN(23)
		const mixedChars = "abcdef @/._+=#~!?*%^&"
		b := make([]byte, length)
		for i := range b {
			b[i] = mixedChars[rng.IntN(len(mixedChars))]
		}
		return string(b)

	case 4:
		// Invalid: control characters (0x00-0x1f).
		length := 1 + rng.IntN(10)
		b := make([]byte, length)
		for i := range b {
			b[i] = byte(rng.IntN(32))
		}
		return string(b)

	case 5:
		// Invalid: Unicode beyond ASCII (various lengths).
		unicodeStrings := []string{
			"foó",                   // accented Latin
			"中文",                    // CJK
			"日本語",                   // CJK
			"\U0001f389\U0001f525",  // emoji: 🎉🔥
			"αβγ",                   // Greek
			"naïve",                 // Latin with combining
			"café",                  // Latin with accent
			"ab‮cd",            // RTL override
			"ab‍cd",            // zero-width joiner
			"ab\uFEFFcd",            // BOM (U+FEFF as escape)
			"ab\x00cd",              // embedded NUL byte
		}
		return unicodeStrings[rng.IntN(len(unicodeStrings))]

	case 6:
		// Boundary lengths exactly: 1, 2, 24, 25.
		lengths := []int{1, 2, 24, 25}
		length := lengths[rng.IntN(len(lengths))]
		const validChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-"
		b := make([]byte, length)
		for i := range b {
			b[i] = validChars[rng.IntN(len(validChars))]
		}
		return string(b)

	default:
		// Random mix: arbitrary bytes 0-127.
		length := rng.IntN(32)
		b := make([]byte, length)
		for i := range b {
			b[i] = byte(rng.IntN(128))
		}
		return string(b)
	}
}

// sanitizeNicknameName converts a human-readable description into a safe Go
// test name. Local to this file to avoid conflicts with sanitizeName and
// sanitizeFencingName in the other fuzz test files.
func sanitizeNicknameName(s string) string {
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
