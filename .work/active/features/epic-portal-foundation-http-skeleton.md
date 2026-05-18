---
id: epic-portal-foundation-http-skeleton
kind: feature
stage: done
tags: [portal]
parent: epic-portal-foundation
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Foundation — HTTP Skeleton

## Brief

The portal's HTTP server skeleton: process entry, chi router with the
per-subroute middleware shape that each auth mechanism needs, structured
logging, the standardized JSON error contract, configuration loading
(env vars + optional YAML), and TLS termination (native HTTPS with cert
paths AND HTTP-behind-trusted-proxy mode, config-selected).

This is the chassis every subsequent route group plugs into. Subroutes are
declared by route-group with their own middleware stacks: `/api/*` (Bearer
auth), `/git/*` (HTTP Basic, owned by `epic-portal-git`), `/mcp/*` (Bearer
auth via MCP headersHelper, owned by `epic-portal-api`), `/ws` (WebSocket
upgrade, owned by `epic-portal-api`). This feature stands up the chassis
and the `/api/*` Bearer-auth subroute scaffold; other epics mount their
groups against it.

The error contract from `docs/PROTOCOL.md > HTTP error contract` is enforced
by middleware that converts panics and recognized error types to the JSON
envelope (`error`, `message`, optional `details`). Structured logging
includes request ID, route, auth subject (when authenticated), and outcome.

Does NOT cover the auth middleware logic itself — that's the tokens
feature. Does NOT cover any concrete endpoint implementations — those
belong to auth-flows, accounts, or sibling epics.

## Epic context

- Parent epic: `epic-portal-foundation`
- Position in epic: parallel with data-layer; tokens and every other
  endpoint-bearing feature mounts against this chassis.

## Foundation references

- `docs/SPEC.md` — Stack > Backend, Hard constraints, Deployment shape
- `docs/ARCHITECTURE.md` — Portal component overview
- `docs/PROTOCOL.md` — HTTP error contract
- `docs/SECURITY.md` — Self-host security posture (TLS posture)

## Inherited epic design decisions

- **HTTP routing**: `chi` — per-subroute middleware stacks make the
  multi-auth shape clean.
- **TLS posture**: support both native HTTPS (cert path config) and
  HTTP-behind-trusted-proxy mode. Operator selects via config.

## Generated-contracts scope

This feature also owns the initial wiring for the spec-first generated-
contracts pipeline locked in `docs/SPEC.md > Generated contracts`:

- Bootstraps `docs/openapi.yaml` with the OpenAPI 3.1 skeleton (info,
  servers, security schemes for Bearer, `components/schemas/`
  placeholder, empty `paths`). Each subsequent REST feature's design
  pass adds its endpoints + schemas to this same file.
- Wires `oapi-codegen` (chi backend) into the Go build: a Makefile
  target `make generate` reads `docs/openapi.yaml` and produces
  generated Go interfaces under an internal package (e.g.,
  `internal/api/openapi/server.gen.go`).
- Wires `openapi-typescript` into the Vite frontend build similarly:
  `make generate` also produces TS types under
  `frontend/src/lib/api/types.gen.ts`.
- CI verifies sync: `make generate && git diff --exit-code` fails the
  build if the working tree disagrees with the spec.

The actual endpoint definitions in the spec are added by the REST
features as they're designed; this feature just establishes the
authoring + codegen pipeline so subsequent features have a place to
hang their schemas.

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Logging**: stdlib `log/slog` with a JSON `Handler` in production
  (config-selected text handler in development). No third-party logger
  (zap/zerolog are unjustified given slog is now stdlib).
- **Config loading**: hand-written struct with `gopkg.in/yaml.v3`
  optional file load + env-var overrides. Lightweight; no env-binding
  library — the field set is small and obvious.
- **Error envelope**: dedicated `internal/portal/httperr` package
  owning the `Error` type, the JSON contract from PROTOCOL.md, and the
  middleware-level Recoverer/NotFound/MethodNotAllowed handlers. Every
  HTTP path that returns an error must route through this package —
  there is no second envelope shape anywhere in the portal.
- **OpenAPI version declaration**: `openapi: 3.0.3` in
  `docs/openapi.yaml` (per locked constraint in `docs/SPEC.md` and the
  research doc — oapi-codegen mainline does not yet support 3.1).
- **Frontend package layout**: minimal `frontend/` skeleton with
  `package.json` + `tsconfig.json` so `openapi-typescript` has a
  destination directory and the codegen target hangs together even
  before the Svelte app exists. The full SPA build wire is added by
  `epic-portal-ui-foundation`.
- **Server lifecycle**: stdlib `net/http.Server` with explicit
  `ReadHeaderTimeout`, `IdleTimeout`, and `ReadTimeout`. Graceful
  shutdown on SIGINT/SIGTERM with a 25-second drain budget.
- **Trust boundary in proxied mode**: when `tls.mode = behind_proxy`,
  install `chi/middleware.RealIP` so `X-Forwarded-For` /
  `X-Real-IP` are honored; when `tls.mode = native`, the middleware
  is omitted (clients hit us directly).
- **Generate-target ownership**: a single root `Makefile` exposes
  `make generate`, which depends on per-domain targets (`generate-db`,
  `generate-api`). Each story that adds codegen appends its sub-target
  idempotently; sibling features merge edits on the same `Makefile`.

## Architectural choice

**chi as the chassis, `httperr` as the JSON envelope, `slog` for
structured logging, stdlib `net/http.Server` for lifecycle.**

Alternatives considered:

- **stdlib `http.ServeMux` (Go 1.22 enhanced)** — handles routing fine
  but the per-subroute middleware shape (Bearer / Basic /
  subprotocol) is awkward at the verbosity level; chi's `r.Route(...,
  func(r chi.Router) { r.Use(BearerAuth); ... })` reads as one
  declarative block, ServeMux reduces to manual wrapper chains.
- **gin or echo** — heavier; replaces stdlib `http.Handler` with a
  bespoke type. We'd lose drop-in compatibility with `http.Handler`
  third-party libs (mcp-go SDK's handler, `git-http-backend` adapter,
  `gorilla/websocket`-style upgraders). chi keeps the stdlib type
  invariant.

chi is the locked decision in `docs/SPEC.md > Stack > chi`.

## Implementation Units

### Unit 1: JSON error envelope

**File**: `internal/portal/httperr/httperr.go`
**Story**: `epic-portal-foundation-http-skeleton-router-and-middleware`

```go
// Package httperr is the only place in the portal that emits an HTTP
// error response. The envelope matches docs/PROTOCOL.md > HTTP error
// contract verbatim.
package httperr

import (
    "encoding/json"
    "errors"
    "log/slog"
    "net/http"
)

// Error is the structured error type used by all handlers.
type Error struct {
    Code    string         `json:"error"`
    Message string         `json:"message"`
    Details map[string]any `json:"details,omitempty"`

    // HTTPStatus is the response code to write. Required.
    HTTPStatus int `json:"-"`

    // Wrapped is an inner error for log context (never serialized).
    Wrapped error `json:"-"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func (e *Error) Unwrap() error { return e.Wrapped }

// Write serializes err to w using the standard envelope. Any non-*Error
// value is wrapped as ErrInternal (500). Callers should never write
// error responses except via this helper.
func Write(w http.ResponseWriter, r *http.Request, err error) {
    var e *Error
    if !errors.As(err, &e) {
        e = ErrInternal(err)
    }
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(e.HTTPStatus)
    _ = json.NewEncoder(w).Encode(e)
    // Log at the level appropriate to the status.
    if e.HTTPStatus >= 500 {
        slog.ErrorContext(r.Context(), "http error",
            "code", e.Code, "status", e.HTTPStatus, "err", e.Wrapped)
    }
}

// Canonical constructors — extend per PROTOCOL.md as endpoints land.
func ErrInternal(cause error) *Error {
    return &Error{Code: "internal", Message: "internal server error",
        HTTPStatus: http.StatusInternalServerError, Wrapped: cause}
}
func ErrInvalidToken() *Error {
    return &Error{Code: "auth.invalid_token", Message: "invalid token",
        HTTPStatus: http.StatusUnauthorized}
}
func ErrExpiredToken() *Error {
    return &Error{Code: "auth.expired_token", Message: "token expired",
        HTTPStatus: http.StatusUnauthorized}
}
func ErrInsufficientPermission() *Error {
    return &Error{Code: "auth.insufficient_permission",
        Message: "insufficient permission",
        HTTPStatus: http.StatusForbidden}
}
func ErrSessionNotFound() *Error {
    return &Error{Code: "session.not_found", Message: "session not found",
        HTTPStatus: http.StatusNotFound}
}
// ... add per PROTOCOL.md code list as features need them
```

**Implementation Notes**:
- The `Details` map carries error-specific fields (`paths`, `missing`,
  etc.) per PROTOCOL.md. Constructors helper-attach for known codes
  (e.g., `ErrScopeViolation(paths ...)`).
- `Wrapped` enables `errors.Is`/`As` chains in callers without
  leaking internals into JSON.
- `slog.ErrorContext` carries the request ID from context (set by the
  RequestID middleware) so error logs correlate to access logs.

### Unit 2: Router middleware (recovery, NotFound, MethodNotAllowed)

**File**: `internal/portal/httperr/middleware.go`
**Story**: `epic-portal-foundation-http-skeleton-router-and-middleware`

```go
package httperr

import (
    "fmt"
    "net/http"
    "runtime/debug"

    "log/slog"
)

// Recoverer converts panics to the JSON envelope. Replaces chi's default
// text/plain recoverer.
func Recoverer(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                slog.ErrorContext(r.Context(), "panic",
                    "recover", fmt.Sprint(rec),
                    "stack", string(debug.Stack()))
                Write(w, r, &Error{
                    Code: "internal", Message: "internal server error",
                    HTTPStatus: http.StatusInternalServerError,
                })
            }
        }()
        next.ServeHTTP(w, r)
    })
}

// NotFoundHandler returns the JSON envelope for unknown routes.
func NotFoundHandler() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        Write(w, r, &Error{
            Code: "route.not_found", Message: "no route matches",
            HTTPStatus: http.StatusNotFound,
        })
    })
}

// MethodNotAllowedHandler returns the JSON envelope for method mismatch.
func MethodNotAllowedHandler() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        Write(w, r, &Error{
            Code: "route.method_not_allowed",
            Message: "method not allowed for route",
            HTTPStatus: http.StatusMethodNotAllowed,
        })
    })
}
```

### Unit 3: Access-log middleware

**File**: `internal/portal/logging/logging.go`
**Story**: `epic-portal-foundation-http-skeleton-router-and-middleware`

```go
package logging

import (
    "log/slog"
    "net/http"
    "os"
    "time"
)

// Setup configures the default slog logger from config. Returns the
// logger to allow callers to scope attributes.
func Setup(format string, level slog.Level) *slog.Logger {
    var handler slog.Handler
    opts := &slog.HandlerOptions{Level: level}
    switch format {
    case "text":
        handler = slog.NewTextHandler(os.Stdout, opts)
    default:
        handler = slog.NewJSONHandler(os.Stdout, opts)
    }
    l := slog.New(handler)
    slog.SetDefault(l)
    return l
}

// Access wraps every request in a structured access log line. Reads the
// request ID from context (set by chi's RequestID middleware upstream).
func Access(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        sr := &statusRecorder{ResponseWriter: w, status: 200}
        next.ServeHTTP(sr, r)
        slog.InfoContext(r.Context(), "http access",
            "method", r.Method,
            "path", r.URL.Path,
            "status", sr.status,
            "duration_ms", time.Since(start).Milliseconds(),
            "bytes", sr.bytes,
        )
    })
}

type statusRecorder struct {
    http.ResponseWriter
    status int
    bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
    s.status = code
    s.ResponseWriter.WriteHeader(code)
}
func (s *statusRecorder) Write(b []byte) (int, error) {
    n, err := s.ResponseWriter.Write(b)
    s.bytes += n
    return n, err
}
```

### Unit 4: Router builder

**File**: `internal/portal/router/router.go`
**Story**: `epic-portal-foundation-http-skeleton-router-and-middleware`

```go
package router

import (
    "net/http"

    "github.com/go-chi/chi/v5"
    chimw "github.com/go-chi/chi/v5/middleware"

    "jamsesh/internal/portal/httperr"
    "jamsesh/internal/portal/logging"
)

// Deps is the dependency surface every subroute mount may require.
// Concrete handler interfaces (TokenStore, MCP handler, etc.) land here
// as sibling features ship.
type Deps struct {
    TrustProxyHeaders bool
    // Mount hooks — left as nilable closures so this feature does not
    // hard-depend on sibling features that haven't shipped yet.
    MountAPI func(chi.Router) // owned by tokens + auth-flows + accounts
    MountGit func(chi.Router) // owned by epic-portal-git
    MountMCP http.Handler     // owned by epic-portal-api
    MountWS  http.HandlerFunc // owned by epic-portal-api
}

// New returns the root http.Handler. Middleware order is intentional:
// RequestID first (everything else logs it), RealIP gated on
// TrustProxyHeaders, Access logging, Recoverer wraps panics into the
// JSON envelope, then route groups mount with their own auth middleware.
func New(d Deps) http.Handler {
    r := chi.NewRouter()
    r.NotFound(httperr.NotFoundHandler().ServeHTTP)
    r.MethodNotAllowed(httperr.MethodNotAllowedHandler().ServeHTTP)

    r.Use(chimw.RequestID)
    if d.TrustProxyHeaders {
        r.Use(chimw.RealIP)
    }
    r.Use(logging.Access)
    r.Use(httperr.Recoverer)

    // Healthcheck — public, no auth.
    r.Get("/healthz", healthz)

    r.Route("/api", func(r chi.Router) {
        if d.MountAPI != nil {
            d.MountAPI(r) // sibling features attach Bearer auth + handlers
        }
    })
    if d.MountGit != nil {
        r.Route("/git", d.MountGit)
    }
    if d.MountMCP != nil {
        r.Mount("/mcp", d.MountMCP)
    }
    if d.MountWS != nil {
        r.Get("/ws/sessions/{sessionID}", d.MountWS)
    }

    return r
}

func healthz(w http.ResponseWriter, _ *http.Request) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    _, _ = w.Write([]byte(`{"status":"ok"}`))
}
```

**Implementation Notes**:
- Nilable mount hooks are the seam that lets http-skeleton ship
  before its dependents. The portal `main.go` populates whichever hooks
  are wired in the current build; missing hooks simply 404.
- This pattern also makes router tests independent — each test passes
  the Deps it cares about; no need for the full constellation.

**Acceptance Criteria** (Units 1-4):
- [ ] `httperr.Error` serializes to the exact envelope from
      PROTOCOL.md (field order, JSON tag spelling, `details` omitted
      when nil)
- [ ] Panicking handlers return a 500 with the standard envelope and
      the panic is logged
- [ ] Unknown paths return 404 with `error: "route.not_found"`
- [ ] Method mismatches return 405 with `error: "route.method_not_allowed"`
- [ ] `/healthz` returns 200 with `{"status":"ok"}`
- [ ] Request ID set by chi appears in `slog` access log line

---

### Unit 5: docs/openapi.yaml skeleton

**File**: `docs/openapi.yaml`
**Story**: `epic-portal-foundation-http-skeleton-openapi-bootstrap`

```yaml
openapi: 3.0.3
info:
  title: jamsesh portal
  version: 0.0.1
  description: |
    REST surface for the jamsesh portal. Single source of truth for
    server interfaces (oapi-codegen) and client types
    (openapi-typescript). Authoring rules in
    docs/SPEC.md > Generated contracts.
servers:
  - url: https://{host}
    variables:
      host:
        default: localhost:8443
security:
  - bearerAuth: []
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: opaque
  schemas:
    ErrorEnvelope:
      type: object
      required: [error, message]
      properties:
        error:
          type: string
          description: machine-readable error code
        message:
          type: string
          description: human-readable error message
        details:
          type: object
          additionalProperties: true
  responses:
    Unauthorized:
      description: missing or invalid token
      content:
        application/json:
          schema: {$ref: '#/components/schemas/ErrorEnvelope'}
    Forbidden:
      description: authenticated but not permitted
      content:
        application/json:
          schema: {$ref: '#/components/schemas/ErrorEnvelope'}
    NotFound:
      description: route or resource not found
      content:
        application/json:
          schema: {$ref: '#/components/schemas/ErrorEnvelope'}
paths: {}
```

### Unit 6: oapi-codegen config + generated stub

**Files**: `oapi-codegen.yaml`, `internal/api/openapi/server.gen.go`
**Story**: `epic-portal-foundation-http-skeleton-openapi-bootstrap`

`oapi-codegen.yaml`:

```yaml
package: openapi
output: internal/api/openapi/server.gen.go
generate:
  chi-server: true
  strict-server: true
  models: true
  embedded-spec: true
output-options:
  prefer-skip-optional-pointer: true
```

`go:generate` directive in `internal/api/openapi/doc.go`:

```go
// Package openapi holds the oapi-codegen generated server interface.
// Regenerate with `make generate-api`. Do not hand-edit *.gen.go.
package openapi

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=../../../oapi-codegen.yaml ../../../docs/openapi.yaml
```

Generated output is committed (so `git diff --exit-code` in CI catches
drift). With empty `paths`, oapi-codegen emits an empty
`StrictServerInterface` and the shared `ErrorEnvelope` model — enough
for downstream features to mount against.

### Unit 7: openapi-typescript wiring + frontend skeleton

**Files**: `frontend/package.json`, `frontend/tsconfig.json`,
`frontend/.gitignore`, `frontend/src/lib/api/types.gen.ts`
**Story**: `epic-portal-foundation-http-skeleton-openapi-bootstrap`

`frontend/package.json`:

```json
{
  "name": "@jamsesh/portal-frontend",
  "private": true,
  "version": "0.0.1",
  "type": "module",
  "scripts": {
    "generate": "openapi-typescript ../docs/openapi.yaml -o src/lib/api/types.gen.ts"
  },
  "devDependencies": {
    "openapi-typescript": "^7.5.0",
    "typescript": "^5.4.0"
  }
}
```

`frontend/tsconfig.json` (minimal):

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "skipLibCheck": true,
    "esModuleInterop": true
  },
  "include": ["src/**/*.ts"]
}
```

`frontend/.gitignore`:

```
node_modules/
dist/
```

`frontend/src/lib/api/types.gen.ts` is the committed generated output.
With empty `paths`, it's a short file declaring the `paths`,
`components`, and `operations` types from the empty spec — adequate
foundation for sibling features.

### Unit 8: Makefile

**File**: `Makefile`
**Story**: `epic-portal-foundation-http-skeleton-openapi-bootstrap`
(Coordinates with `data-layer-queries-and-codegen` which adds
`generate-db`.)

```makefile
.PHONY: generate generate-db generate-api generate-api-go generate-api-ts

generate: generate-db generate-api

generate-db:
	sqlc generate

generate-api: generate-api-go generate-api-ts

generate-api-go:
	go generate ./internal/api/openapi/...

generate-api-ts:
	cd frontend && npm install --silent && npm run generate
```

**Implementation Notes**:
- The Makefile is shared infrastructure. If `data-layer-queries-and-codegen`
  lands first, it creates the file with `generate` + `generate-db`; this
  story's commit adds the API targets via Edit. If this story lands
  first, the reverse. Implementer should `cat Makefile` defensively
  before deciding between Write and Edit.

**Acceptance Criteria** (Units 5-8):
- [ ] `make generate-api` runs clean from a fresh checkout
      (`npm install` populated `node_modules`, `oapi-codegen` produced
      `server.gen.go`)
- [ ] `make generate && git diff --exit-code` is green
- [ ] `docs/openapi.yaml` validates as openapi 3.0.3 (e.g., via
      `npx @redocly/cli lint docs/openapi.yaml`)
- [ ] `internal/api/openapi/server.gen.go` exports
      `StrictServerInterface` and the `ErrorEnvelope` model

---

### Unit 9: Config

**File**: `internal/portal/config/config.go`
**Story**: `epic-portal-foundation-http-skeleton-config-tls-and-entry`

```go
package config

import (
    "fmt"
    "log/slog"
    "os"
    "strconv"

    "gopkg.in/yaml.v3"
)

type Config struct {
    Bind     string    `yaml:"bind"`     // ":8443" or "0.0.0.0:8443"
    DBDriver string    `yaml:"db_driver"` // "sqlite" | "postgres"
    DBDSN    string    `yaml:"db_dsn"`
    TLS      TLSConfig `yaml:"tls"`
    Log      LogConfig `yaml:"log"`
    Storage  string    `yaml:"storage"`  // path for bare repos
}

type TLSConfig struct {
    Mode     string `yaml:"mode"`      // "native" | "behind_proxy"
    CertPath string `yaml:"cert_path"`  // only if mode == native
    KeyPath  string `yaml:"key_path"`   // only if mode == native
}

type LogConfig struct {
    Format string     `yaml:"format"` // "json" | "text"
    Level  slog.Level `yaml:"level"`
}

// Load reads YAML at path (optional), then overlays env vars
// (JAMSESH_BIND, JAMSESH_DB_DRIVER, JAMSESH_DB_DSN, JAMSESH_TLS_MODE,
// JAMSESH_TLS_CERT, JAMSESH_TLS_KEY, JAMSESH_LOG_FORMAT,
// JAMSESH_LOG_LEVEL, JAMSESH_STORAGE).
func Load(path string) (Config, error) {
    cfg := defaults()
    if path != "" {
        b, err := os.ReadFile(path)
        if err != nil {
            return cfg, fmt.Errorf("config: read %s: %w", path, err)
        }
        if err := yaml.Unmarshal(b, &cfg); err != nil {
            return cfg, fmt.Errorf("config: parse %s: %w", path, err)
        }
    }
    applyEnv(&cfg)
    if err := cfg.validate(); err != nil {
        return cfg, err
    }
    return cfg, nil
}

func defaults() Config {
    return Config{
        Bind:     ":8443",
        DBDriver: "sqlite",
        DBDSN:    "./jamsesh.db",
        TLS:      TLSConfig{Mode: "behind_proxy"},
        Log:      LogConfig{Format: "json", Level: slog.LevelInfo},
        Storage:  "./storage",
    }
}

func (c Config) validate() error {
    switch c.TLS.Mode {
    case "native":
        if c.TLS.CertPath == "" || c.TLS.KeyPath == "" {
            return fmt.Errorf("config: tls.mode=native requires cert_path and key_path")
        }
    case "behind_proxy":
        // ok
    default:
        return fmt.Errorf("config: tls.mode must be 'native' or 'behind_proxy'")
    }
    switch c.DBDriver {
    case "sqlite", "postgres":
    default:
        return fmt.Errorf("config: db_driver must be 'sqlite' or 'postgres'")
    }
    return nil
}

func applyEnv(c *Config) {
    if v := os.Getenv("JAMSESH_BIND"); v != "" {
        c.Bind = v
    }
    if v := os.Getenv("JAMSESH_DB_DRIVER"); v != "" {
        c.DBDriver = v
    }
    if v := os.Getenv("JAMSESH_DB_DSN"); v != "" {
        c.DBDSN = v
    }
    if v := os.Getenv("JAMSESH_TLS_MODE"); v != "" {
        c.TLS.Mode = v
    }
    if v := os.Getenv("JAMSESH_TLS_CERT"); v != "" {
        c.TLS.CertPath = v
    }
    if v := os.Getenv("JAMSESH_TLS_KEY"); v != "" {
        c.TLS.KeyPath = v
    }
    if v := os.Getenv("JAMSESH_LOG_FORMAT"); v != "" {
        c.Log.Format = v
    }
    if v := os.Getenv("JAMSESH_LOG_LEVEL"); v != "" {
        if n, err := strconv.Atoi(v); err == nil {
            c.Log.Level = slog.Level(n)
        }
    }
    if v := os.Getenv("JAMSESH_STORAGE"); v != "" {
        c.Storage = v
    }
}
```

### Unit 10: Server lifecycle

**File**: `internal/portal/server/server.go`
**Story**: `epic-portal-foundation-http-skeleton-config-tls-and-entry`

```go
package server

import (
    "context"
    "errors"
    "log/slog"
    "net/http"
    "time"

    "jamsesh/internal/portal/config"
)

// Run blocks serving handler on cfg.Bind in the requested TLS mode.
// Returns nil on graceful shutdown (ctx cancelled), error on listen
// failure.
func Run(ctx context.Context, cfg config.Config, handler http.Handler) error {
    srv := &http.Server{
        Addr:              cfg.Bind,
        Handler:           handler,
        ReadHeaderTimeout: 5 * time.Second,
        ReadTimeout:       30 * time.Second,
        IdleTimeout:       2 * time.Minute,
    }

    listenErr := make(chan error, 1)
    go func() {
        switch cfg.TLS.Mode {
        case "native":
            listenErr <- srv.ListenAndServeTLS(cfg.TLS.CertPath, cfg.TLS.KeyPath)
        default: // behind_proxy
            listenErr <- srv.ListenAndServe()
        }
    }()

    slog.InfoContext(ctx, "portal listening",
        "bind", cfg.Bind, "tls_mode", cfg.TLS.Mode)

    select {
    case err := <-listenErr:
        if errors.Is(err, http.ErrServerClosed) {
            return nil
        }
        return err
    case <-ctx.Done():
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
        defer cancel()
        return srv.Shutdown(shutdownCtx)
    }
}
```

### Unit 11: Portal binary entry

**File**: `cmd/portal/main.go`
**Story**: `epic-portal-foundation-http-skeleton-config-tls-and-entry`

```go
package main

import (
    "context"
    "flag"
    "log/slog"
    "os"
    "os/signal"
    "syscall"

    "jamsesh/internal/portal/config"
    "jamsesh/internal/portal/logging"
    "jamsesh/internal/portal/router"
    "jamsesh/internal/portal/server"
)

func main() {
    cfgPath := flag.String("config", "", "path to YAML config (env vars override)")
    flag.Parse()

    cfg, err := config.Load(*cfgPath)
    if err != nil {
        slog.Error("config load failed", "err", err)
        os.Exit(2)
    }
    logging.Setup(cfg.Log.Format, cfg.Log.Level)

    ctx, cancel := signal.NotifyContext(context.Background(),
        os.Interrupt, syscall.SIGTERM)
    defer cancel()

    handler := router.New(router.Deps{
        TrustProxyHeaders: cfg.TLS.Mode == "behind_proxy",
        // Sibling features wire MountAPI / MountGit / MountMCP /
        // MountWS through this file as they ship. For now, only
        // /healthz is live.
    })

    if err := server.Run(ctx, cfg, handler); err != nil {
        slog.Error("server exited with error", "err", err)
        os.Exit(1)
    }
}
```

**Acceptance Criteria** (Units 9-11):
- [ ] `config.Load("")` returns defaults
- [ ] `config.Load(path)` loads YAML, env vars override
- [ ] Invalid TLS mode / DB driver fails validation
- [ ] `cmd/portal` builds; `JAMSESH_BIND=:0 ./portal` starts and
      `GET /healthz` returns 200
- [ ] SIGINT triggers graceful shutdown within ≤ 25s

## Implementation Order

1. **openapi-bootstrap** story — spec skeleton, codegen configs,
   Makefile, frontend skeleton, generated stubs committed.
   (Parallel-safe with story 2.)
2. **router-and-middleware** story — httperr, logging, router
   builder, healthcheck. (Parallel-safe with story 1.)
3. **config-tls-and-entry** story — config, server lifecycle, main.go.
   Depends on `router-and-middleware`.

## Testing

### Unit Tests

- `internal/portal/httperr/httperr_test.go` — envelope shape, status
  code propagation, errors.As/Is round-trips
- `internal/portal/httperr/middleware_test.go` — panic recovery,
  NotFound, MethodNotAllowed paths
- `internal/portal/logging/logging_test.go` — JSON handler line
  shape, access middleware status capture
- `internal/portal/router/router_test.go` — nil mount hooks 404,
  mounted hooks reach handler, healthcheck
- `internal/portal/config/config_test.go` — defaults, YAML load, env
  overlay precedence, validation failures
- `internal/portal/server/server_test.go` — graceful shutdown, listen
  error propagation

### Integration

- `cmd/portal/main_test.go` (smoke) — build, start with ephemeral
  port + behind-proxy mode, `GET /healthz`, SIGTERM, exit cleanly

## Risks

- **Generate-pipeline drift between this feature and data-layer.** Both
  features modify the root `Makefile`. Story bodies call this out and
  prescribe defensive edits, but the first parallel landing wins the
  initial file shape. Mitigation: keep the Makefile small; each story
  appends `.PHONY` targets idempotently.
- **Empty OpenAPI spec generating no useful code.** With `paths: {}`,
  oapi-codegen produces an empty `StrictServerInterface`. That's fine
  for the bootstrap, but the first REST feature must populate at least
  one path before the strict-handler wiring proves real. Mitigation:
  document this dependency in the spec file's `description` and in
  the next REST feature's brief.
- **slog default logger global state.** `logging.Setup` calls
  `slog.SetDefault`. Tests that run in parallel against multiple
  format configs will race. Mitigation: tests construct `slog.Logger`
  values directly and pass them explicitly; only `cmd/portal/main`
  calls `Setup`.
- **frontend/ skeleton without a real Vite app.** The Makefile target
  runs `npm install` inside `frontend/`, which requires a working
  npm + Node toolchain in CI. Mitigation: pin Node version in CI
  config (added by `epic-distribution-build-pipeline`); the local
  developer story tolerates `npm install` running on demand.

## Implementation summary

All 3 child stories advanced to `stage: review`:

| Story | Status | Notes |
|---|---|---|
| `http-skeleton-router-and-middleware` | review | httperr, logging, router-with-Deps shape, /healthz. 27 unit tests green. Clean implementation of the multi-auth mount-hook seam |
| `http-skeleton-openapi-bootstrap` | review | OpenAPI 3.0.3 skeleton, oapi-codegen v2.7.0 + openapi-typescript 7.13.0 wiring, generated stubs committed. Added `output-options.skip-prune: true` to force `ErrorEnvelope` emission with empty paths. `tools/tools.go` anchors the codegen tool dep |
| `http-skeleton-config-tls-and-entry` | review | Config Load + env overlay, server lifecycle with graceful shutdown, cmd/portal entry. Added `LogConfig.UnmarshalYAML` to accept both int and string slog levels. In-process smoke test exercises the full startup → healthz → SIGTERM cycle |

### Mid-wave fix
- After Wave 2 the oapi-codegen `output:` path produced a duplicated `internal/api/openapi/internal/api/openapi/` directory because `output:` resolved relative to the package dir under `go generate`. Fixed by changing `output: internal/api/openapi/server.gen.go` → `output: server.gen.go` (commit `bfaa23a`).

### Cross-cutting deviations
- `go-version-file: go.mod` used in CI instead of pinning a version string — `go.mod` linter bumped Go directive to 1.25.7 mid-implementation, so `go-version-file` keeps CI in lockstep automatically
- `LogConfig.UnmarshalYAML` supports both integer values and slog name strings (improvement over int-only design)

### Verification
- `go build ./cmd/portal` and `go build ./...` clean
- `go test ./internal/portal/...` green (all packages)
- `actionlint` clean on workflow files
- `make generate && git diff --exit-code` green
- In-process smoke test (server_test.go:TestGracefulShutdown) green

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Capability complete. The HTTP chassis is up: chi router with the correct middleware stack, JSON error envelope matching PROTOCOL.md, slog access logging, config loader (env + YAML), TLS modes (native + behind-proxy), graceful shutdown, and the generated-contracts pipeline (oapi-codegen + openapi-typescript) wired with empty paths ready for downstream features to populate. Late-binding shape via router.Deps mount hooks is exactly right for incremental feature shipping. No cross-cutting concerns at the seam.
