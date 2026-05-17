# Verified API signatures (v1.6.0)

Read this when you need exact types for tool registration, handler
signatures, or middleware wiring beyond what SKILL.md covers.

```go
// mcp/server.go
func NewServer(impl *Implementation, opts *ServerOptions) *Server
func AddTool[In, Out any](s *Server, t *Tool, h ToolHandlerFor[In, Out])

// mcp/streamable.go
// getServer returning nil -> HTTP 400 "no server available"
func NewStreamableHTTPHandler(
    getServer func(*http.Request) *Server,
    opts      *StreamableHTTPOptions,
) *StreamableHTTPHandler

// mcp/tool.go
type ToolHandlerFor[In, Out any] func(
    ctx context.Context, request *CallToolRequest, input In,
) (result *CallToolResult, output Out, _ error)

// mcp/shared.go
type RequestExtra struct {
    TokenInfo      *auth.TokenInfo
    Header         http.Header
    CloseSSEStream func(CloseSSEStreamArgs)
}

type ServerRequest[P Params] struct {
    Session *ServerSession
    Params  P
    Extra   *RequestExtra
}
type CallToolRequest = ServerRequest[*CallToolParamsRaw]

// mcp/protocol.go
type Tool struct {
    Name         string
    Description  string
    Title        string
    InputSchema  any   // required; auto-inferred from In via AddTool
    OutputSchema any   // optional; inferred from Out if Out != any
    Annotations  *ToolAnnotations
    Icons        []Icon
    Meta         Meta
}

type Implementation struct {
    Name       string  // required
    Version    string  // required
    Title      string
    WebsiteURL string
    Icons      []Icon
}

// auth/auth.go
type TokenInfo struct {
    Scopes     []string
    Expiration time.Time   // MUST be non-zero or middleware returns 401
    UserID     string      // non-empty enables session-hijacking protection
    Extra      map[string]any
}

type TokenVerifier func(
    ctx context.Context, token string, req *http.Request,
) (*TokenInfo, error)

type RequireBearerTokenOptions struct {
    ResourceMetadataURL string // optional, surfaced in WWW-Authenticate
    Scopes              []string
}

func RequireBearerToken(
    verifier TokenVerifier, opts *RequireBearerTokenOptions,
) func(http.Handler) http.Handler

func TokenInfoFromContext(ctx context.Context) *TokenInfo

var ErrInvalidToken = errors.New("invalid token") // unwrap target
var ErrOAuth        = errors.New("oauth error")
```

## `StreamableHTTPOptions`

```go
type StreamableHTTPOptions struct {
    Stateless                  bool          // default false
    JSONResponse               bool          // default false (text/event-stream)
    Logger                     *slog.Logger
    EventStore                 EventStore    // optional stream-resumption store
    SessionTimeout             time.Duration // zero = no timeout
    DisableLocalhostProtection bool          // DNS-rebind protection escape
    CrossOriginProtection      *http.CrossOriginProtection // deprecated; wrap externally
}
```

## `ServerOptions` (selected fields)

```go
type ServerOptions struct {
    Instructions       string
    Logger             *slog.Logger
    InitializedHandler func(context.Context, *InitializedRequest)
    PageSize           int           // default 1000
    KeepAlive          time.Duration // ping interval
    GetSessionID       func() string // override per-session ID generation
    SchemaCache        *SchemaCache  // share across servers if you want
    // SubscribeHandler / UnsubscribeHandler for resource subscriptions
}
```

## Tool error handling

Inside a `ToolHandlerFor` handler:

- Returning a non-nil `error` -> SDK packs it into
  `CallToolResult.Content` with `IsError: true`. This is a **tool
  error**, not a protocol error — the client sees it as a tool failure.
- Returning `nil, nil, nil` is invalid (missing result). Either return a
  populated `Out` (handler auto-fills Content) or an explicit
  `*CallToolResult`.
- Panicking is the SDK's "protocol error" path — don't.

## Streamable-HTTP wire shape

| Method   | Purpose                                            |
|----------|----------------------------------------------------|
| `POST`   | JSON-RPC messages; may create or use a session     |
| `GET`    | Open or resume standalone SSE stream               |
| `DELETE` | Terminate session                                  |

| Header                 | When                                       |
|------------------------|--------------------------------------------|
| `Accept`               | Always: `application/json, text/event-stream` |
| `Content-Type`         | POST: `application/json`                   |
| `Mcp-Session-Id`       | After first response on POST; required on GET/DELETE |
| `Mcp-Protocol-Version` | After `initialize` negotiation (`2025-11-25` is current) |
| `Authorization`        | `Bearer <token>` (validated by `auth.RequireBearerToken`) |
