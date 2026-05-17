---
name: oapi-codegen
description: Reference for oapi-codegen v2 (post-deepmap-fork) — OpenAPI to Go. Auto-loads when editing oapi-codegen.yaml, docs/openapi.yaml, files importing github.com/oapi-codegen/oapi-codegen/v2 or github.com/oapi-codegen/runtime, generated *.gen.go files, or files with //go:generate oapi-codegen directives. Triggers on terms — StrictServerInterface, ServerInterface, HandlerFromMux, HandlerWithOptions, NewStrictHandler, ChiServerOptions, chi-server, strict-server, embedded-spec, RequestObject, ResponseObject, RequestEditorFn, ClientWithResponses, openapi 3.0 vs 3.1.
user-invocable: false
---

# oapi-codegen reference (jamsesh)

**Canonical module**: `github.com/oapi-codegen/oapi-codegen/v2`.
**Pinned version**: v2.7.0 (2026-05-01). Runtime helpers:
`github.com/oapi-codegen/runtime`.

**Fork history**: project moved from `github.com/deepmap/oapi-codegen`
to its own org in May 2024 at v2.3.0. Versions ≤ v2.2.0 still live at
the deepmap path. Use the new path for everything.

## CRITICAL: OpenAPI 3.1 status (read this first)

`docs/SPEC.md` commits jamsesh to OpenAPI 3.1, but **oapi-codegen
v2.7.0 does NOT yet support 3.1** (kin-openapi parser blocker). The
experimental `oapi-codegen-exp` fork supports 3.1 but is explicitly
NOT production-ready.

**Workaround**: declare `openapi: 3.0.3` in `docs/openapi.yaml` and
constrain authoring to the 3.0-compatible subset. The TS generator
handles both natively, so this is a single-side compromise. See
**`references/3.1-workaround.md`** for the full authoring rules and
migration trigger.

## Why oapi-codegen here (locked decision)

Generated Go server interfaces from `docs/openapi.yaml`. Handlers
implement a typed `StrictServerInterface` — drift between spec and
code is a **compile error**. Component schemas become Go structs
reused by the WebSocket gateway inside event-envelope payloads
(single source of truth for wire shape).

## oapi-codegen.yaml (canonical config)

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/oapi-codegen/oapi-codegen/v2.7.0/configuration-schema.json
package: api
output: internal/api/api.gen.go
generate:
  chi-server: true        # chi-flavored HandlerFromMux
  strict-server: true     # typed Request/Response wrapper layer
  models: true            # struct types from components.schemas
  embedded-spec: true     # spec bytes embedded for /openapi.json
output-options:
  prefer-skip-optional-pointer: true
```

Invoke from `go:generate`:

```go
//go:generate oapi-codegen -config oapi-codegen.yaml ../../docs/openapi.yaml
```

Install CLI:
`go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.0`.

## Strict server (USE THIS — don't use raw ServerInterface)

Each operation gets a typed `XxxRequestObject` (params + body) and a
sum-type `XxxResponseObject` interface with one impl per declared
HTTP status. Codegen wires JSON encode/decode, status, content-type
automatically.

```go
type Server struct{ store store.Store }

var _ api.StrictServerInterface = (*Server)(nil)

func (s *Server) GetSession(ctx context.Context, req api.GetSessionRequestObject) (api.GetSessionResponseObject, error) {
    acct := authContext(ctx)
    sess, err := s.store.GetSession(ctx, acct.OrgID, req.SessionId)
    if errors.Is(err, sql.ErrNoRows) {
        return api.GetSession404JSONResponse{
            NotFoundJSONResponse: api.NotFoundJSONResponse{Code: "session.not_found"},
        }, nil
    }
    if err != nil {
        return nil, err // → 500 via default ErrorHandlerFunc
    }
    return api.GetSession200JSONResponse(toAPISession(sess)), nil
}
```

## Mounting on chi

```go
import (
    "github.com/go-chi/chi/v5"
    "your/internal/api"
)

func MountAPI(r chi.Router, srv api.StrictServerInterface) {
    strict := api.NewStrictHandler(srv, nil) // 2nd arg: []StrictMiddlewareFunc
    api.HandlerFromMux(strict, r)
}
```

jamsesh's auth middleware lives at the `r.Route("/api", ...)`
boundary in the chi skeleton — NOT inside the generated handlers.
For custom error handling, use `api.HandlerWithOptions(strict,
api.ChiServerOptions{ErrorHandlerFunc: jsonErrorHandler, ...})`.

## Generated structs reused by WebSocket gateway

```go
type Envelope struct {
    Version   int       `json:"version"`
    Seq       int64     `json:"seq"`
    Type      string    `json:"type"`        // commit.arrived, ...
    Payload   any       `json:"payload"`     // api.Commit, api.Comment, ...
    Timestamp time.Time `json:"timestamp"`
    SessionID string    `json:"session_id"`
}
```

Same `api.Session` / `api.Comment` types flow through REST AND WS.

## Client (for tests, plugin RPC)

```yaml
# tools/clients/oapi-codegen-client.yaml
package: portalapi
output: tools/clients/portalapi/client.gen.go
generate:
  client: true
  models: true
```

```go
client, _ := portalapi.NewClientWithResponses(baseURL,
    portalapi.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
        req.Header.Set("Authorization", "Bearer "+token)
        return nil
    }),
)
resp, _ := client.GetSessionWithResponse(ctx, sessionID)
if resp.JSON200 != nil { /* typed body */ }
```

Production TS client uses `openapi-typescript` + `openapi-fetch`
against the same spec — Go client is for in-process tests and CLI
binaries only.

## Build wire

```makefile
generate:
	go generate ./...
	npx openapi-typescript docs/openapi.yaml -o ui/src/api/schema.gen.ts

ci-generate:
	$(MAKE) generate
	git diff --exit-code
```

## Pitfalls

- **OpenAPI 3.1 declarations break the build**. Keep
  `openapi: 3.0.3`. See `references/3.1-workaround.md`.
- **`oneOf` / `anyOf` without discriminator → opaque `interface{}`**
  in generated Go. Always provide a discriminator.
- **`additionalProperties: true`** → `map[string]interface{}`. Lock
  additional properties to a schema or `false`.
- **`required:` vs `nullable:`** are independent. Required nullable
  → `*string`; non-required non-nullable → `string` (with
  `prefer-skip-optional-pointer`).
- **`example:` is 3.0; `examples:` array is 3.1.** Use `example:`.
- **Don't edit `*.gen.go`.** Regenerate. CI catches hand-edits.
- **`HandlerFromMux`** already registers routes — don't `r.Mount("/",
  h)` on the same router (double registration).
- **deepmap import path is dead-end**: stopped at v2.2.0. Migrate
  any legacy code to the new path.
- **`-package` CLI flag is deprecated** — use YAML config `package:`.

## References

- jamsesh foundation: `docs/SPEC.md` Generated contracts section
- jamsesh spec file: `docs/openapi.yaml` (single source of truth)
- Research doc: `docs/research/core-go-server-stack.md`
- 3.1 workaround details: `references/3.1-workaround.md`
- godoc: https://pkg.go.dev/github.com/oapi-codegen/oapi-codegen/v2
- Repo: https://github.com/oapi-codegen/oapi-codegen
