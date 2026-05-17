// Package storage provides the on-disk and DB-side storage layer for portal
// git sessions. It exposes a Service interface consumed by smart-HTTP handlers,
// pre/post-receive hooks, and REST API handlers.
package storage

import (
	"context"
	"fmt"

	"jamsesh/internal/db/store"
)

// ArchiveInfo carries the metadata needed to archive a session.
// Defined here so that callers do not import archive.go (next story).
type ArchiveInfo struct {
	Name             string
	GoalText         string
	MemberAccountIDs []string
	EndedAt          interface{} // time.Time — filled in by archive-and-stub story
	EndReason        string      // "finalize" | "abandon" | "timeout"
	FinalBranchName  *string
}

// ArchivedRecord is the read-side view of an archived session row.
// Defined here so sibling packages can depend on the type without
// importing the archive implementation (added in epic-portal-git-storage-archive-and-stub).
type ArchivedRecord struct {
	SessionID        string
	OrgID            string
	Name             string
	GoalText         string
	MemberAccountIDs []string
	EndReason        string
	FinalBranchName  *string
}

// ArchivedStub is the 410 Gone response body returned for archived sessions.
type ArchivedStub struct {
	Error      string `json:"error"`   // "session.archived"
	Message    string `json:"message"` // user-readable explanation
	HTTPStatus int    `json:"-"`       // 410
}

// Service is the storage-layer interface consumed by all portal components.
// Methods implemented by this story: RepoPath, CreateRepo, RemoveRepo, RepoExists.
// Methods marked TODO are implemented by epic-portal-git-storage-archive-and-stub.
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
	// TODO: epic-portal-git-storage-archive-and-stub
	ArchiveSession(ctx context.Context, orgID, sessionID string, info ArchiveInfo) error

	// LookupArchived returns the archived record for a session, or ErrNotFound.
	// TODO: epic-portal-git-storage-archive-and-stub
	LookupArchived(ctx context.Context, orgID, sessionID string) (*ArchivedRecord, error)

	// StubResponse builds the 410 Gone response body for an archived session.
	// TODO: epic-portal-git-storage-archive-and-stub
	StubResponse(rec *ArchivedRecord) ArchivedStub
}

// service is the concrete implementation of Service.
type service struct {
	root  string      // absolute path to the storage root directory
	store store.Store // data layer; used by archive-and-stub methods
}

// New returns a Service backed by rootDir on disk and the given Store.
// rootDir is the storage root (e.g. /var/jamsesh/storage); it need not exist
// yet — CreateRepo creates subdirectories on demand.
func New(rootDir string, s store.Store) Service {
	return &service{root: rootDir, store: s}
}

// ---------------------------------------------------------------------------
// Unimplemented stubs — filled in by epic-portal-git-storage-archive-and-stub
// ---------------------------------------------------------------------------

// ArchiveSession is not yet implemented.
// TODO: epic-portal-git-storage-archive-and-stub
func (s *service) ArchiveSession(_ context.Context, _, _ string, _ ArchiveInfo) error {
	return fmt.Errorf("storage: ArchiveSession not implemented yet")
}

// LookupArchived is not yet implemented.
// TODO: epic-portal-git-storage-archive-and-stub
func (s *service) LookupArchived(_ context.Context, _, _ string) (*ArchivedRecord, error) {
	return nil, fmt.Errorf("storage: LookupArchived not implemented yet")
}

// StubResponse is not yet implemented.
// TODO: epic-portal-git-storage-archive-and-stub
func (s *service) StubResponse(_ *ArchivedRecord) ArchivedStub {
	return ArchivedStub{}
}
