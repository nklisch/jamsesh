# Pattern: testEnv harness struct

Each portal package's test file declares an unexported `testEnv struct`
that bundles the per-test wired dependencies (httptest.Server, store,
tokens service, stub storage/sender, etc.) and a `newTestEnv(t *testing.T)
*testEnv` constructor (often with `WithStore` / `WithClock` /
`WithTokens` variants) that wires the strict handler shim into a chi
router, registers `t.Cleanup`, and returns the struct. Tests then pull
pre-wired dependencies via `env.srv.URL`, `env.s`, `env.token`, etc.

## Rationale

Portal handler tests need a real httptest.Server (to exercise the chi
router + strict handler wrapping + middleware), a real store (to round-
trip data through the dialect adapter), a tokens service (to issue
bearer tokens for auth), and stubs for non-test-relevant deps (storage,
sender). Wiring all of that inline in every test would be 30+ lines of
boilerplate per `TestXxx`.

`testEnv` centralizes the wiring; `newTestEnv(t)` plus
`t.Cleanup(srv.Close)` makes setup a one-liner. Tests stay focused on
the behaviour under assertion. Per-test variation is opted in via
`WithStore` / `WithClock` / `WithTokens` overloads rather than
parameterising the base constructor.

The struct name is always `testEnv` (lowercase, unexported); the
constructor is always `newTestEnv`. Consistency across 8 packages means
an agent reading any portal test file knows where to find the
dependency wiring without grep.

## Examples

### Example 1: sessions package — full strict-handler wiring

**File**: `internal/portal/sessions/handler_test.go:227`
```go
type testEnv struct {
	srv      *httptest.Server
	svc      tokens.Service
	s        store.Store
	stor     *stubStorage
	sender   *stubSender
	eventLog *events.Log
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	s := openStore(t)
	return newTestEnvWithStore(t, s, s)
}

func newTestEnvWithStore(t *testing.T, handlerStore testSessionsStore, baseStore store.Store) *testEnv {
	t.Helper()
	svc := tokens.New(baseStore)
	stor := newStubStorage()
	log := events.New(baseStore)
	sender := &stubSender{}
	h := sessions.New(handlerStore, stor, log, sender, "http://localhost:8443")
	strictAPI := openapi.NewStrictHandlerWithOptions(&sessionsOnlyStrict{h}, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})
	// ... chi.NewRouter + bearer middleware + route registrations ...
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, svc: svc, s: baseStore, stor: stor, sender: sender, eventLog: log}
}
```

### Example 2: playground package — multiple constructor variants

**File**: `internal/portal/playground/handler_test.go:234`
```go
type testEnv struct {
	srv   *httptest.Server
	s     store.Store
	svc   tokens.Service
	stor  *stubStorage
	clock playground.Clock
}

func newTestEnv(t *testing.T, s store.Store, cfg playground.Config) *testEnv {
	t.Helper()
	return newTestEnvWithClock(t, s, cfg, fixedClock{t: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)})
}

func newTestEnvSQLite(t *testing.T, cfg playground.Config) *testEnv { /* ... */ }
func newTestEnvWithTokens(t *testing.T, s store.Store, cfg playground.Config, svc tokens.Service) *testEnv { /* ... */ }
func newTestEnvWithClock(t *testing.T, s store.Store, cfg playground.Config, clk playground.Clock) *testEnv { /* ... */ }
func newTestEnvWithClockAndTokens(...) *testEnv { /* ... */ }
```

### Example 3: comments package — service + handler bundled

**File**: `internal/portal/comments/service_test.go:32`
```go
type testEnv struct {
	s       store.Store
	svc     *comments.Service
	handler *comments.Handler
	srv     *httptest.Server
	token   string
	orgID   string
	sessID  string
	accID   string
}

func newTestEnv(t *testing.T) *testEnv { /* ... */ }
func newTestEnvWithStore(t *testing.T, handlerStore testCommentsStore, baseStore store.Store) *testEnv { /* ... */ }
```

(8 packages: tokens, githttp, sessions, wsgateway, comments, mcpendpoint,
playground; `internal/portal/testclock/handler_test.go` also follows.)

## When to Use

- Any new portal package's `*_test.go` file that needs an httptest.Server
  + real store + token-issuance + stub deps. Declare `testEnv` and
  `newTestEnv(t)` from the first test.
- When tests in a package start repeating the same 20-line wiring block
  inline, refactor to a `testEnv` harness.

## When NOT to Use

- **Pure-function unit tests** — `prereceive.ValidateWritableScope`,
  `wordlist.Pick`, etc. need no harness; call them directly with table
  inputs.
- **Tests that genuinely need a different wiring** (e.g. real WebSocket
  upgrade vs strict-handler HTTP). Spawn a sibling `testEnv2` or
  package-local struct rather than overloading `testEnv` with optional
  fields that most tests don't use.

## Common Violations

- **Exporting the struct or constructor** (`TestEnv`, `NewTestEnv`).
  The harness is intra-package scaffolding; sharing across packages
  invites coupling. If two packages need the same setup, the shared
  fixture belongs in `tests/e2e/fixtures/` (see
  `testcontainers-fixture-shape`).
- **Inline wiring in a single `TestXxx`** when a sibling test already
  uses `newTestEnv`. Even one-off tests should reuse the harness so
  changes to dependency wiring (new middleware, new sub-service) ripple
  through one constructor instead of N inline blocks.
- **Storing per-test mutable state on `testEnv`** that bleeds across
  subtests. Each `TestXxx` should call `newTestEnv(t)` to get a fresh
  harness; `t.Cleanup` handles teardown.

#### Index entry
- **testenv-harness-struct**: Each portal package's tests bundle wired deps (`*httptest.Server`, real `store.Store`, tokens service, stubs) in an unexported `testEnv struct` constructed via `newTestEnv(t *testing.T) *testEnv` with optional `WithStore`/`WithClock`/`WithTokens` overloads.
