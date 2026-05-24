---
id: e2e-audit-playground-nickname-fuzz
kind: story
stage: done
tags: [testing, e2e-test, audit, playground]
parent: feature-e2e-playground-coverage-fuzz
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Nickname input boundary is fuzzed only at unit scope — no fuzz harness against the real validator + DB

## Severity
Low

## Finding type
missing-taxonomy-layer

## Evidence

`tests/e2e/fuzz/` has four harnesses: `fencing_token_test.go`,
`mcp_tool_input_test.go`, `object_storage_dsn_test.go`,
`pack_manifest_test.go`. None target playground inputs:

```
$ grep -rIn -E "nickname|playground|anon" tests/e2e/fuzz/
(no output)
```

Unit coverage: `TestJoinPlaygroundSession_NicknameValidation` exercises a
fixed enumeration of inputs (per the handler_test.go file name list). The
2-24 chars / letters / digits / dashes spec is verified once, in process.

## Why this matters

The nickname is the lowest-impact untrusted input in the playground
surface — it appears in WebSocket events, commit trailers
(`Jam-Session`-style trailers per the go-git skill), and the join-summary
response. A bug in the unicode-class detection (e.g. accepting
right-to-left override characters, zero-width joiners, control bytes,
NUL) would not crash anything but would let an anonymous user inject
display-mangling text into other participants' UIs. The fuzz layer in
`mcp_tool_input_test.go` already proves the harness pattern; this
extends it to a second untrusted-string entry point.

## Suggested remedy

Add `tests/e2e/fuzz/playground_nickname_test.go` modeled on
`mcp_tool_input_test.go`. Property: any byte sequence sent as the
`nickname` field of `POST /api/playground/sessions/{id}/join` either:
- Yields 2xx with the nickname round-tripped exactly in the response
  (and the round-tripped value satisfies the spec).
- Yields 4xx with a typed error (`playground.nickname_invalid` or
  similar).

But never 5xx, never silent normalization that produces a different
visible string than what was returned.

Iteration count via `NICKNAME_FUZZ_COUNT` env var, reproducibility via
`NICKNAME_FUZZ_SEED`, same as the other fuzz harnesses.

## Test sketch

```go
// tests/e2e/fuzz/playground_nickname_test.go
func TestPlaygroundNicknameFuzz(t *testing.T) {
    if testing.Short() { t.Skip("fuzz: -short") }

    count := envIntDefault("NICKNAME_FUZZ_COUNT", 100)
    seed  := envIntDefault("NICKNAME_FUZZ_SEED", time.Now().UnixNano())
    rng   := rand.New(rand.NewSource(seed))
    t.Logf("seed=%d count=%d", seed, count)

    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    p  := portal.Start(ctx, t, portal.Options{
        DBDriver: "postgres", DBDSN: pg.ContainerDSN,
        PlaygroundEnabled: true,
    })
    sess := createPlayground(t, p.URL)

    for i := 0; i < count; i++ {
        nick := randomBytes(rng, 0, 64) // boundary + invalid
        body := map[string]string{"nickname": string(nick)}
        resp := postJSON(t, p.URL+"/api/playground/sessions/"+sess.ID+"/join", "", body)

        switch {
        case resp.StatusCode >= 500:
            t.Fatalf("seed=%d iter=%d nick=%q produced 5xx %d",
                seed, i, nick, resp.StatusCode)
        case resp.StatusCode == 200:
            got := decodeJoinResp(t, resp).Nickname
            if got != string(nick) {
                t.Fatalf("seed=%d iter=%d silent normalization: sent %q got %q",
                    seed, i, nick, got)
            }
        case resp.StatusCode >= 400:
            // ok: typed reject.
        }
    }
}
```

## Implementation notes

**File:** `tests/e2e/fuzz/playground_nickname_test.go`

**Two test entry points implemented:**

1. `FuzzPlaygroundNickname` — Go-native fuzz harness (`func FuzzXxx(*testing.F)`).
   Seed corpus: 31 entries (19 invalid + 12 valid/edge), covering all 11 cases
   from `TestJoinPlaygroundSession_NicknameValidation` plus Unicode, control
   chars, RTL override, zero-width joiner, emoji, and length-boundary cases.
   Each fuzz iteration starts a fresh postgres + portal container bound to the
   iteration's `*testing.T` (matching the `TestFencingTokenFuzz` pattern — the
   fixtures require `*testing.T`, not `*testing.F`). All 31 seed cases pass
   under `go test -run FuzzPlaygroundNickname -count=1`.

2. `TestPlaygroundNicknameFuzz` — property-based companion. Starts the stack
   once (amortises container startup), creates one shared session, then runs
   30 seed cases + 100 random iterations. All 130 cases pass.

**Contract predicate (`nicknameValid`):**

The predicate mirrors the handler's actual validation pipeline, including the
`strings.TrimSpace` call:

```go
func nicknameValid(s string) bool {
    trimmed := strings.TrimSpace(s)
    if trimmed == "" {
        return true // empty or whitespace-only: server mints, 200 path
    }
    return len(trimmed) >= 2 && len(trimmed) <= 24 && nicknameValidRE.MatchString(trimmed)
}
```

**Contract discovery during testing:**

During seed corpus execution, `seed#18` (`" "` — single space) triggered the
BUG path in the initial predicate (which treated whitespace-only as invalid).
Investigation confirmed this is intentional production behavior: the handler's
`strings.TrimSpace(req.Body.Nickname) != ""` check collapses whitespace-only
strings to `""`, which falls through to the server-mints path (200). This is a
contract clarification, not a production bug. The predicate was updated to match.

**Rate limiter finding:**

`POST /api/playground/sessions` has a per-IP rate limiter
(`JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR`, default 3/hour). The
`TestPlaygroundNicknameFuzz` design was revised to share one session across all
iterations (invalid inputs never add a participant; valid inputs do, but
`MaxParticipants=10000` absorbs the full iteration set). Rate limit env var
set to 10000 in both test functions.

**Verification run:**

```
$ cd tests/e2e && go test ./fuzz/ -run FuzzPlaygroundNickname -count=1 -v
# 31 seeds: PASS (34s)

$ cd tests/e2e && go test ./fuzz/ -run TestPlaygroundNicknameFuzz -count=1 -v
# 30 seeds + 100 random: PASS (7s)
```

Active fuzzing (`go test -fuzz=FuzzPlaygroundNickname -fuzztime=30s`) was NOT
run in this session (expensive — requires dedicated fuzz time). The seed corpus
pass is sufficient for the regression layer; active fuzzing is left to CI or
manual invocation.

## Review (2026-05-24)

**Verdict**: Approve

**Notes**: Both test entry points pass. `FuzzPlaygroundNickname`
(native Go fuzz) exercises 31 seeds in ~34s.
`TestPlaygroundNicknameFuzz` (property-based companion) runs 130
cases in ~7s sharing one stack + one session (avoids the rate
limiter). One contract discovery documented in the story:
whitespace-only `" "` collapses to server-mints (200) because the
handler trims before the non-empty check — predicate updated to
match. Real-stack assertions throughout; no mocks.

Advanced `stage: review → done`.
