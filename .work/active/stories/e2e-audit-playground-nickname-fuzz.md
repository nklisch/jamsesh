---
id: e2e-audit-playground-nickname-fuzz
kind: story
stage: implementing
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
