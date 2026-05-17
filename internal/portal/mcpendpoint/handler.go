// Package mcpendpoint implements the streamable-HTTP MCP endpoint for the
// jamsesh portal. It mounts at /mcp and exposes four tools: post_comment,
// resolve_comment, fork, and query_session_state.
//
// Auth is handled by the auth.RequireBearerToken middleware wrapping the
// handler before it is passed to router.Deps.MountMCP. Tool handlers
// retrieve the authenticated account via auth.TokenInfoFromContext.
package mcpendpoint

import (
	"context"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/comments"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// Clock is an injectable time source. Mirrors auth.Clock and tokens.Clock so a
// single *testclock.AdvanceableClock satisfies all of them. Per-package types
// avoid cross-package import coupling — structural typing carries the
// "advance once, move everywhere" property.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// Endpoint is the MCP endpoint for the jamsesh portal. Construct it once and
// call Handler to get the http.Handler ready for mounting.
//
// Endpoint is struct-literal-initialized in cmd/portal/main.go. The Clock field
// is optional: a nil Clock falls back to the real wall clock via the now()
// helper. This preserves backwards compatibility with tests that construct
// Endpoint directly without setting Clock.
type Endpoint struct {
	Store    store.Store
	Tokens   tokens.Service
	Storage  storage.Service
	Log      *events.Log
	Comments *comments.Service
	Clock    Clock
}

// now returns the Endpoint's current time. Falls back to realClock when
// Clock is nil so test code can construct Endpoint literals without
// initializing the field.
func (e *Endpoint) now() time.Time {
	if e.Clock == nil {
		return time.Now().UTC()
	}
	return e.Clock.Now()
}

// Handler constructs a streamable-HTTP MCP handler with Bearer auth middleware.
// The returned handler is suitable for mounting at router.Deps.MountMCP.
//
// Auth flow:
//  1. auth.RequireBearerToken validates the Bearer token in the Authorization
//     header and injects *auth.TokenInfo (with UserID = account.ID) into ctx.
//  2. getServer builds a per-request *mcp.Server (shared instance wrapped to
//     satisfy the callback signature).
//  3. Each tool handler calls auth.TokenInfoFromContext to get the UserID.
func (e *Endpoint) Handler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "jamsesh",
		Version: "0.1.0",
	}, &mcp.ServerOptions{
		Instructions: "jamsesh collaborative coding session tools",
	})

	e.registerTools(server)

	raw := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			SessionTimeout: 30 * time.Minute,
		},
	)

	authMW := auth.RequireBearerToken(e.verifyToken, &auth.RequireBearerTokenOptions{
		Scopes: []string{"mcp"},
	})

	return authMW(raw)
}

// verifyToken validates a raw Bearer token and returns the token info.
// It satisfies the auth.TokenVerifier signature expected by RequireBearerToken.
func (e *Endpoint) verifyToken(ctx context.Context, rawToken string, _ *http.Request) (*auth.TokenInfo, error) {
	account, err := e.Tokens.Validate(ctx, rawToken)
	if err != nil {
		return nil, auth.ErrInvalidToken
	}
	// Expiration must be non-zero (pitfall #3). Use a sentinel far-future time
	// since jamsesh tokens are opaque and the TTL is enforced in the DB.
	return &auth.TokenInfo{
		UserID:     account.ID,
		Scopes:     []string{"mcp"},
		Expiration: e.now().Add(24 * time.Hour),
	}, nil
}

// registerTools adds all four jamsesh tools to the server.
func (e *Endpoint) registerTools(s *mcp.Server) {
	mcp.AddTool(s,
		&mcp.Tool{
			Name:        "post_comment",
			Description: "Post a comment on a commit or file in a jamsesh session.",
		},
		e.postComment,
	)
	mcp.AddTool(s,
		&mcp.Tool{
			Name:        "resolve_comment",
			Description: "Mark a comment as resolved in a jamsesh session.",
		},
		e.resolveComment,
	)
	mcp.AddTool(s,
		&mcp.Tool{
			Name:        "fork",
			Description: "Create an agent branch (jam/<session>/<account>/<name>) pointing at a target commit.",
		},
		e.fork,
	)
	mcp.AddTool(s,
		&mcp.Tool{
			Name:        "query_session_state",
			Description: "Query the current state of a jamsesh session: goal, scope, draft tip, unresolved comments addressed to caller, open conflicts, and recent events.",
		},
		e.querySessionState,
	)
}

// findOrg walks the caller's session memberships to locate the orgID for the
// given sessionID. Returns an error if the caller is not a member.
func (e *Endpoint) findOrg(ctx context.Context, accountID, sessionID string) (string, error) {
	memberships, err := e.Store.ListSessionMembershipsForAccount(ctx, accountID)
	if err != nil {
		return "", err
	}
	for _, m := range memberships {
		if m.SessionID == sessionID {
			return m.OrgID, nil
		}
	}
	return "", permissionErrorf("not a member of session %s", sessionID)
}
