// Package storage provides the on-disk and DB-side storage layer for portal
// git sessions. It exposes a Service interface consumed by smart-HTTP handlers,
// pre/post-receive hooks, and REST API handlers.
package storage

import (
	"context"
	"time"

	"jamsesh/internal/db/store"
)

// ArchiveInfo carries the metadata needed to archive a session.
type ArchiveInfo struct {
	Name             string
	GoalText         string
	MemberAccountIDs []string
	EndedAt          time.Time
	EndReason        string  // "finalize" | "abandon" | "timeout"
	FinalBranchName  *string
}

// ArchivedRecord is the read-side view of an archived session row.
type ArchivedRecord struct {
	SessionID        string
	OrgID            string
	Name             string
	GoalText         string
	MemberAccountIDs []string
	EndedAt          time.Time
	ArchivedAt       time.Time
	EndReason        string
	FinalBranchName  *string
}

// ArchivedStub is the 410 Gone response body returned for archived sessions.
type ArchivedStub struct {
	Error      string `json:"error"`   // "session.archived"
	Message    string `json:"message"` // user-readable explanation
	Details    struct {
		ArchivedAt      string  `json:"archived_at"`
		FinalBranchName *string `json:"final_branch_name,omitempty"`
		EndReason       string  `json:"end_reason"`
	} `json:"details"`
	HTTPStatus int `json:"-"` // 410
}

// Service is the storage-layer interface consumed by all portal components.
type Service interface {
	// RepoPath returns the absolute filesystem path for the bare repo of the
	// given org+session pair. It does not access the filesystem.
	RepoPath(orgID, sessionID string) string

	// CreateRepo initialises a new bare git repository on disk for the given
	// org+session pair. Returns an error if the repo already exists.
	CreateRepo(ctx context.Context, orgID, sessionID string) error

	// RemoveRepo hard-deletes the bare repo directory tree. It is idempotent:
	// if the path does not exist it returns nil.
	RemoveRepo(ctx context.Context, orgID, sessionID string) error

	// RepoExists reports whether a bare repo directory exists on disk for the
	// given org+session pair.
	RepoExists(orgID, sessionID string) (bool, error)

	// ArchiveSession archives a session: inserts the archived_sessions row,
	// hard-deletes the bare repo, and deletes the live sessions row.
	ArchiveSession(ctx context.Context, orgID, sessionID string, info ArchiveInfo) error

	// LookupArchived returns the archived record for a session, or
	// store.ErrNotFound if the session has not been archived.
	LookupArchived(ctx context.Context, orgID, sessionID string) (*ArchivedRecord, error)

	// StubResponse builds the 410 Gone response body for an archived session.
	StubResponse(rec *ArchivedRecord) ArchivedStub
}

// service is the concrete implementation of Service.
type service struct {
	root  string      // absolute path to the storage root directory
	store store.Store // data layer; used by archive methods
}

// New returns a Service backed by rootDir on disk and the given Store.
// rootDir is the storage root (e.g. /var/jamsesh/storage); it need not exist
// yet — CreateRepo creates subdirectories on demand.
func New(rootDir string, s store.Store) Service {
	return &service{root: rootDir, store: s}
}
