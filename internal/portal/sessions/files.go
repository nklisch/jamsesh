// Package sessions — file content endpoint.
// GET /api/orgs/{orgID}/sessions/{sessionID}/files?commit=<sha>&path=<filepath>
package sessions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

const (
	maxInlineBytes  = 1 << 20 // 1 MiB
	binaryScanBytes = 8000
)

// GetSessionFile returns the text content of a file at the specified commit.
// Binary files and files > 1 MiB are handled gracefully.
func (h *Handler) GetSessionFile(ctx context.Context, req openapi.GetSessionFileRequestObject) (openapi.GetSessionFileResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.GetSessionFile401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID
	commitSHA := req.Params.Commit
	filePath := req.Params.Path

	// Require org membership.
	if _, err := h.store.GetOrgMember(ctx, store.GetOrgMemberParams{
		OrgID:     orgID,
		AccountID: acc.ID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.GetSessionFile403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this org",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: files: get org member: %w", err)
	}

	// Require session membership.
	if _, err := h.store.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: acc.ID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.GetSessionFile403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this session",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: files: get session member: %w", err)
	}

	// Verify session exists.
	if _, err := h.store.GetSession(ctx, orgID, sessionID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.GetSessionFile404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: files: get session: %w", err)
	}

	// Open the bare repo.
	repoPath := h.storage.RepoPath(orgID, sessionID)
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("sessions: files: open repo: %w", err)
	}

	// Resolve the commit.
	hash := plumbing.NewHash(commitSHA)
	commitObj, err := repo.CommitObject(hash)
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return openapi.GetSessionFile404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "commit.not_found",
					Message: "commit not found",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: files: get commit: %w", err)
	}

	// Find the file in the commit tree.
	f, err := commitObj.File(filePath)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return openapi.GetSessionFile404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "file.not_found",
					Message: "file not found in commit",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: files: get file: %w", err)
	}

	// Check size before reading.
	if f.Size > maxInlineBytes {
		return openapi.GetSessionFile413JSONResponse(openapi.ErrorEnvelope{
			Error:   "file.too_large",
			Message: "file exceeds 1 MiB inline limit",
		}), nil
	}

	// Read the blob.
	reader, err := f.Reader()
	if err != nil {
		return nil, fmt.Errorf("sessions: files: open blob: %w", err)
	}
	defer func() { _ = reader.Close() }()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("sessions: files: read blob: %w", err)
	}

	// Detect binary: scan the first binaryScanBytes for null bytes.
	scanLen := len(raw)
	if scanLen > binaryScanBytes {
		scanLen = binaryScanBytes
	}
	isBinary := bytes.ContainsRune(raw[:scanLen], 0)

	mime := detectMIME(filePath, isBinary)

	if isBinary {
		return openapi.GetSessionFile200JSONResponse(openapi.SessionFileResponse{
			Content:  "",
			Mime:     mime,
			IsBinary: true,
		}), nil
	}

	return openapi.GetSessionFile200JSONResponse(openapi.SessionFileResponse{
		Content:  string(raw),
		Mime:     mime,
		IsBinary: false,
	}), nil
}

// detectMIME returns a simple MIME type based on file extension and binary flag.
func detectMIME(path string, isBinary bool) string {
	if isBinary {
		return "application/octet-stream"
	}
	// Basic extension-based MIME for common text types.
	dot := lastDot(path)
	switch dot {
	case ".go":
		return "text/x-go"
	case ".ts", ".tsx":
		return "text/typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "text/javascript"
	case ".svelte":
		return "text/x-svelte"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".md":
		return "text/markdown"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".sh":
		return "text/x-sh"
	case ".py":
		return "text/x-python"
	case ".rs":
		return "text/x-rust"
	case ".sql":
		return "text/x-sql"
	case ".toml":
		return "text/x-toml"
	}
	return "text/plain"
}

// lastDot returns the last dot-prefixed extension in path, or empty string.
func lastDot(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i:]
		}
		if path[i] == '/' {
			break
		}
	}
	return ""
}
