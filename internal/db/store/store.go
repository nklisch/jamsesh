// Package store defines the data-access seam used by every component of the
// portal. Implementations are dialect-specific (sqlite, postgres) and selected
// at startup by db.Open(driver, dsn).
package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by Get* methods when no matching row exists.
// Implementations translate sql.ErrNoRows and pgx.ErrNoRows into this error.
var ErrNotFound = errors.New("store: not found")

// ErrUniqueViolation is returned when an INSERT or UPDATE would violate a
// UNIQUE constraint. Implementations translate SQLite SQLITE_CONSTRAINT_UNIQUE
// and Postgres SQLSTATE 23505 into this error.
var ErrUniqueViolation = errors.New("store: unique violation")

// TxStore is the interface available inside a WithTx callback. It mirrors all
// domain sub-interfaces but omits Close and Dialect (which operate on the
// outer connection, not the Tx).
type TxStore interface {
	OrgStore
	AccountStore
	OrgMemberStore
	SessionStore
	SessionMemberStore
	OAuthTokenStore
	MagicLinkTokenStore
	ArchivedSessionStore
	OAuthStateStore
	EventLogStore
	PresenceStore
	OrgInviteStore
	RefModeStore
	SessionInviteStore
	ConflictEventStore
	CommentStore
	FinalizeLockStore
}

// Store is the unified data-access interface for the portal. All handler and
// service code depends on this interface; dialect selection is once-at-startup
// via db.Open.
type Store interface {
	OrgStore
	AccountStore
	OrgMemberStore
	SessionStore
	SessionMemberStore
	OAuthTokenStore
	MagicLinkTokenStore
	ArchivedSessionStore
	OAuthStateStore
	EventLogStore
	PresenceStore
	OrgInviteStore
	RefModeStore
	SessionInviteStore
	ConflictEventStore
	CommentStore
	FinalizeLockStore

	// WithTx opens a dialect-appropriate transaction, calls fn with a TxStore
	// backed by that transaction, and commits on success or rolls back on any
	// error (including panics recovered as errors). The caller must not retain
	// the TxStore outside fn.
	WithTx(ctx context.Context, fn func(TxStore) error) error

	// Ping verifies the database connection is alive. Used by the /readyz probe.
	Ping(ctx context.Context) error

	// Close releases pool resources.
	Close() error
	// Dialect reports the underlying engine ("sqlite" or "postgres").
	// Useful in logs and metrics.
	Dialect() string
}

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// Org represents an organisation row.
type Org struct {
	ID        string
	Name      string
	Slug      string
	CreatedAt time.Time
}

// Account represents a user account.
type Account struct {
	ID           string
	Email        string
	DisplayName  string
	GithubUserID *string // nullable
	CreatedAt    time.Time
}

// OrgMember is a membership record joining an account to an org.
type OrgMember struct {
	OrgID     string
	AccountID string
	Role      string
	CreatedAt time.Time
}

// OrgMemberWithAccount is returned by ListOrgMembers — it combines the
// membership row with the joined account fields.
type OrgMemberWithAccount struct {
	OrgID        string
	AccountID    string
	Role         string
	CreatedAt    time.Time // member since
	Email        string
	DisplayName  string
	GithubUserID *string
	AccountCreatedAt time.Time
}

// Session is a collaborative coding session.
type Session struct {
	ID                       string
	OrgID                    string
	Name                     string
	Goal                     string
	WritableScope            string     // JSON array, stored verbatim
	DefaultMode              string     // "sync" | "isolated"
	BaseSHA                  *string    // nullable until first push
	Status                   string     // "active" | "finalizing" | "ended" | "archived"
	CreatedAt                time.Time
	EndedAt                  *time.Time
	EndReason                *string // nullable; "abandoned" | "finalized" | "timeout"
	FinalizeLockedByAccountID *string // nullable; set while finalize-flow is in progress
}

// RefMode is a per-ref mode override for a session.
type RefMode struct {
	SessionID string
	Ref       string
	Mode      string // "sync" | "isolated"
}

// SessionMember is a membership record for a session.
type SessionMember struct {
	OrgID     string
	SessionID string
	AccountID string
	Role      string
	JoinedAt  time.Time
}

// SessionMembership combines a session_members row with summary fields from
// the joined session. Returned by ListSessionMembershipsForAccount.
type SessionMembership struct {
	OrgID         string
	SessionID     string
	AccountID     string
	Role          string
	JoinedAt      time.Time
	SessionName   string
	SessionStatus string
	SessionGoal   string
}

// OAuthToken is a stored OAuth access or refresh token (hashed).
type OAuthToken struct {
	ID         string
	AccountID  string
	TokenHash  string
	Kind       string     // "access" | "refresh"
	IssuedAt   time.Time
	ExpiresAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

// OAuthState is a server-side nonce row used to validate OAuth callbacks.
type OAuthState struct {
	Nonce       string
	Provider    string
	RedirectURI string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// MagicLinkToken is a single-use sign-in token.
type MagicLinkToken struct {
	ID        string
	TokenHash string
	Email     string
	IssuedAt  time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// ArchivedSession is a record of a session that has been archived. The bare
// repo has been deleted; this row provides the stub response for 410 Gone.
type ArchivedSession struct {
	SessionID        string
	OrgID            string
	Name             string
	GoalText         string
	MemberAccountIDs []string // decoded from JSON TEXT column
	EndedAt          time.Time
	ArchivedAt       time.Time
	EndReason        string  // "finalize" | "abandon" | "timeout"
	FinalBranchName  *string // nil when absent
}

// ---------------------------------------------------------------------------
// Parameter types
// ---------------------------------------------------------------------------

type CreateOrgParams struct {
	ID        string
	Name      string
	Slug      string
	CreatedAt time.Time
}

type CreateAccountParams struct {
	ID           string
	Email        string
	DisplayName  string
	GithubUserID *string // nil → NULL
	CreatedAt    time.Time
}

type UpdateAccountDisplayNameParams struct {
	ID          string
	DisplayName string
}

type AddOrgMemberParams struct {
	OrgID     string
	AccountID string
	Role      string
	CreatedAt time.Time
}

type GetOrgMemberParams struct {
	OrgID     string
	AccountID string
}

type RemoveOrgMemberParams struct {
	OrgID     string
	AccountID string
}

type CreateSessionParams struct {
	ID            string
	OrgID         string
	Name          string
	Goal          string
	WritableScope string
	DefaultMode   string
	BaseSHA       *string
	Status        string
	CreatedAt     time.Time
	EndedAt       *time.Time
}

type GetSessionParams struct {
	OrgID string
	ID    string
}

type UpdateSessionStatusParams struct {
	OrgID  string
	ID     string
	Status string
}

type SetSessionBaseSHAParams struct {
	OrgID   string
	ID      string
	BaseSHA *string
}

type AddSessionMemberParams struct {
	OrgID     string
	SessionID string
	AccountID string
	Role      string
	JoinedAt  time.Time
}

type GetSessionMemberParams struct {
	OrgID     string
	SessionID string
	AccountID string
}

type ListSessionMembersParams struct {
	OrgID     string
	SessionID string
}

type RemoveSessionMemberParams struct {
	OrgID     string
	SessionID string
	AccountID string
}

type CreateOAuthTokenParams struct {
	ID         string
	AccountID  string
	TokenHash  string
	Kind       string
	IssuedAt   time.Time
	ExpiresAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

type TouchOAuthTokenLastUsedParams struct {
	ID         string
	LastUsedAt *time.Time
}

type RevokeOAuthTokenParams struct {
	ID        string
	RevokedAt *time.Time
}

type RevokeAllOAuthTokensForAccountParams struct {
	AccountID string
	RevokedAt *time.Time
}

type CreateMagicLinkTokenParams struct {
	ID        string
	TokenHash string
	Email     string
	IssuedAt  time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type ConsumeMagicLinkTokenParams struct {
	ID     string
	UsedAt *time.Time
}

type InsertOAuthStateParams struct {
	Nonce       string
	Provider    string
	RedirectURI string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

type InsertArchivedSessionParams struct {
	SessionID        string
	OrgID            string
	Name             string
	GoalText         string
	MemberAccountIDs string // JSON-encoded []string
	EndedAt          time.Time
	ArchivedAt       time.Time
	EndReason        string  // "finalize" | "abandon" | "timeout"
	FinalBranchName  *string // nil → NULL
}

type GetArchivedSessionParams struct {
	OrgID     string
	SessionID string
}

type DeleteSessionParams struct {
	OrgID string
	ID    string
}

type UpdateSessionGoalScopeModeParams struct {
	OrgID         string
	ID            string
	Goal          string
	WritableScope string
	DefaultMode   string
}

type SetSessionEndReasonParams struct {
	OrgID     string
	ID        string
	EndReason *string
	EndedAt   *time.Time
}

type SetFinalizeLockParams struct {
	OrgID     string
	ID        string
	AccountID *string // nil means no lock / clear
}

type ClearFinalizeLockParams struct {
	OrgID string
	ID    string
}

type UpsertRefModeParams struct {
	SessionID string
	Ref       string
	Mode      string
}

type GetRefModeParams struct {
	SessionID string
	Ref       string
}

// ---------------------------------------------------------------------------
// Sub-interfaces
// ---------------------------------------------------------------------------

// OrgStore covers org CRUD.
type OrgStore interface {
	CreateOrg(ctx context.Context, arg CreateOrgParams) (Org, error)
	GetOrgByID(ctx context.Context, id string) (Org, error)
	GetOrgBySlug(ctx context.Context, slug string) (Org, error)
}

// AccountStore covers account CRUD.
type AccountStore interface {
	CreateAccount(ctx context.Context, arg CreateAccountParams) (Account, error)
	GetAccountByID(ctx context.Context, id string) (Account, error)
	GetAccountByEmail(ctx context.Context, email string) (Account, error)
	// GetAccountByGitHubUserID looks up an account by its GitHub user ID.
	// Pass nil to search for accounts with NULL github_user_id.
	GetAccountByGitHubUserID(ctx context.Context, githubUserID *string) (Account, error)
	UpdateAccountDisplayName(ctx context.Context, arg UpdateAccountDisplayNameParams) error
}

// OrgMemberStore covers org membership.
type OrgMemberStore interface {
	AddOrgMember(ctx context.Context, arg AddOrgMemberParams) error
	GetOrgMember(ctx context.Context, arg GetOrgMemberParams) (OrgMember, error)
	ListOrgsForAccount(ctx context.Context, accountID string) ([]Org, error)
	ListOrgMembers(ctx context.Context, orgID string) ([]OrgMemberWithAccount, error)
	RemoveOrgMember(ctx context.Context, arg RemoveOrgMemberParams) error
}

// SessionStore covers session CRUD.
type SessionStore interface {
	CreateSession(ctx context.Context, arg CreateSessionParams) (Session, error)
	GetSession(ctx context.Context, orgID, id string) (Session, error)
	ListSessionsForOrg(ctx context.Context, orgID string) ([]Session, error)
	// ListSessionsForOrgWithCursor returns sessions ordered by created_at DESC
	// with created_at < before (cursor-based pagination).
	ListSessionsForOrgWithCursor(ctx context.Context, arg ListSessionsForOrgWithCursorParams) ([]Session, error)
	UpdateSessionStatus(ctx context.Context, arg UpdateSessionStatusParams) error
	UpdateSessionGoalScopeMode(ctx context.Context, arg UpdateSessionGoalScopeModeParams) error
	SetSessionBaseSHA(ctx context.Context, arg SetSessionBaseSHAParams) error
	SetSessionEndReason(ctx context.Context, arg SetSessionEndReasonParams) error
	SetFinalizeLock(ctx context.Context, arg SetFinalizeLockParams) error
	ClearFinalizeLock(ctx context.Context, arg ClearFinalizeLockParams) error
	// DeleteSession removes a session row and cascades to session_members via FK.
	DeleteSession(ctx context.Context, arg DeleteSessionParams) error
}

// RefModeStore covers per-ref mode overrides for sessions.
type RefModeStore interface {
	UpsertRefMode(ctx context.Context, arg UpsertRefModeParams) error
	GetRefMode(ctx context.Context, arg GetRefModeParams) (RefMode, error)
	ListRefModesForSession(ctx context.Context, sessionID string) ([]RefMode, error)
}

// SessionMemberStore covers session membership.
type SessionMemberStore interface {
	AddSessionMember(ctx context.Context, arg AddSessionMemberParams) error
	GetSessionMember(ctx context.Context, arg GetSessionMemberParams) (SessionMember, error)
	ListSessionMembers(ctx context.Context, arg ListSessionMembersParams) ([]SessionMember, error)
	RemoveSessionMember(ctx context.Context, arg RemoveSessionMemberParams) error
	// ListSessionMembershipsForAccount returns all session memberships for an
	// account across all orgs. This is the intentional cross-org exception —
	// the caller receives org_id on each row for further scoped queries.
	ListSessionMembershipsForAccount(ctx context.Context, accountID string) ([]SessionMembership, error)
}

// ArchivedSessionStore covers archived-session reads and writes.
type ArchivedSessionStore interface {
	InsertArchivedSession(ctx context.Context, arg InsertArchivedSessionParams) error
	GetArchivedSession(ctx context.Context, arg GetArchivedSessionParams) (ArchivedSession, error)
}

// OAuthTokenStore covers OAuth token lifecycle.
type OAuthTokenStore interface {
	CreateOAuthToken(ctx context.Context, arg CreateOAuthTokenParams) (OAuthToken, error)
	GetOAuthTokenByHash(ctx context.Context, tokenHash string) (OAuthToken, error)
	TouchOAuthTokenLastUsed(ctx context.Context, arg TouchOAuthTokenLastUsedParams) error
	RevokeOAuthToken(ctx context.Context, arg RevokeOAuthTokenParams) error
	RevokeAllOAuthTokensForAccount(ctx context.Context, arg RevokeAllOAuthTokensForAccountParams) error
	ListOAuthTokensForAccount(ctx context.Context, accountID string) ([]OAuthToken, error)
}

// MagicLinkTokenStore covers magic-link token lifecycle.
type MagicLinkTokenStore interface {
	CreateMagicLinkToken(ctx context.Context, arg CreateMagicLinkTokenParams) (MagicLinkToken, error)
	GetMagicLinkTokenByHash(ctx context.Context, tokenHash string) (MagicLinkToken, error)
	// ConsumeMagicLinkToken marks the token used (single-use enforcement at
	// the SQL level: UPDATE WHERE used_at IS NULL).
	ConsumeMagicLinkToken(ctx context.Context, arg ConsumeMagicLinkTokenParams) error
}

// Event is a persisted event-log row.
type Event struct {
	ID        string
	OrgID     string
	SessionID string
	Seq       int64
	Type      string
	Payload   string // JSON text
	CreatedAt time.Time
}

// PresenceRow is a presence row for a session participant.
type PresenceRow struct {
	OrgID        string
	SessionID    string
	AccountID    string
	Ref          string
	CurrentSHA   string
	LastActiveAt time.Time
}

// InsertEventParams are the parameters for inserting a single event row.
type InsertEventParams struct {
	ID        string
	OrgID     string
	SessionID string
	Seq       int64
	Type      string
	Payload   string
	CreatedAt time.Time
}

// ListEventsSinceParams are the parameters for the ListEventsSince query.
type ListEventsSinceParams struct {
	SessionID string
	SinceSeq  int64
	Limit     int64
}

// ListEventsSinceForDigestParams are the parameters for the ListEventsSinceForDigest query.
type ListEventsSinceForDigestParams struct {
	SessionID string
	SinceSeq  int64
	Limit     int64
}

// ListSessionsForOrgWithCursorParams are the parameters for cursor-paginated session listing.
type ListSessionsForOrgWithCursorParams struct {
	OrgID     string
	Before    time.Time // exclusive upper bound on created_at
	Limit     int64
}

// UpsertPresenceParams are the parameters for UpsertPresence.
type UpsertPresenceParams struct {
	OrgID        string
	SessionID    string
	AccountID    string
	Ref          string
	CurrentSHA   string
	LastActiveAt time.Time
}

// EventLogStore covers event-log writes and reads.
type EventLogStore interface {
	// EnsureEventSeqRow creates the event_seq row for the session if it
	// does not yet exist (idempotent).
	EnsureEventSeqRow(ctx context.Context, sessionID string) error
	// AllocateNextSeq atomically increments the per-session counter and
	// returns the newly allocated seq number.
	AllocateNextSeq(ctx context.Context, sessionID string) (int64, error)
	// AllocateNextSeqN atomically increments the counter by n and returns
	// the last allocated seq (the range is [last-n+1, last]).
	AllocateNextSeqN(ctx context.Context, sessionID string, n int64) (int64, error)
	// InsertEvent inserts a single event row.
	InsertEvent(ctx context.Context, p InsertEventParams) error
	// ListEventsSince returns events with seq > sinceSeq in ascending order.
	ListEventsSince(ctx context.Context, p ListEventsSinceParams) ([]Event, error)
	// ListEventsSinceForDigest returns only digest-relevant event types
	// (commit.arrived, comment.*, conflict.*, mode.changed) with seq > sinceSeq.
	ListEventsSinceForDigest(ctx context.Context, p ListEventsSinceForDigestParams) ([]Event, error)
}

// PresenceStore covers presence upserts and reads.
type PresenceStore interface {
	// UpsertPresence inserts or updates a presence row.
	UpsertPresence(ctx context.Context, p UpsertPresenceParams) error
	// ListPresenceForSession returns all presence rows for a session.
	ListPresenceForSession(ctx context.Context, sessionID string) ([]PresenceRow, error)
}

// OrgInvite is a pending or accepted org membership invite.
type OrgInvite struct {
	ID                  string
	OrgID               string
	InviterAccountID    string
	RecipientEmail      string
	TokenHash           string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	AcceptedAt          *time.Time
	AcceptedByAccountID *string
}

// SessionInvite is a pending or accepted session membership invite.
type SessionInvite struct {
	ID                  string
	OrgID               string
	SessionID           string
	InviterAccountID    string
	InviteeEmail        string
	TokenHash           string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	AcceptedAt          *time.Time
	AcceptedByAccountID *string
}

// ---------------------------------------------------------------------------
// OrgInvite parameter types
// ---------------------------------------------------------------------------

type InsertOrgInviteParams struct {
	ID                  string
	OrgID               string
	InviterAccountID    string
	RecipientEmail      string
	TokenHash           string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	AcceptedAt          *time.Time
	AcceptedByAccountID *string
}

type MarkOrgInviteAcceptedParams struct {
	ID                  string
	AcceptedAt          time.Time
	AcceptedByAccountID string
}

type ListPendingOrgInvitesForOrgParams struct {
	OrgID string
	Now   time.Time
}

type ListPendingOrgInvitesForEmailParams struct {
	Email string
	Now   time.Time
}

// ---------------------------------------------------------------------------
// SessionInvite parameter types
// ---------------------------------------------------------------------------

type InsertSessionInviteParams struct {
	ID                  string
	OrgID               string
	SessionID           string
	InviterAccountID    string
	InviteeEmail        string
	TokenHash           string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	AcceptedAt          *time.Time
	AcceptedByAccountID *string
}

type MarkSessionInviteAcceptedParams struct {
	ID                  string
	AcceptedAt          time.Time
	AcceptedByAccountID string
}

type ListPendingSessionInvitesForSessionParams struct {
	SessionID string
	Now       time.Time
}

// OrgInviteStore covers org invite lifecycle.
type OrgInviteStore interface {
	InsertOrgInvite(ctx context.Context, arg InsertOrgInviteParams) (OrgInvite, error)
	GetOrgInviteByID(ctx context.Context, id string) (OrgInvite, error)
	GetOrgInviteByTokenHash(ctx context.Context, tokenHash string) (OrgInvite, error)
	MarkOrgInviteAccepted(ctx context.Context, arg MarkOrgInviteAcceptedParams) error
	ListPendingOrgInvitesForOrg(ctx context.Context, arg ListPendingOrgInvitesForOrgParams) ([]OrgInvite, error)
	ListPendingOrgInvitesForEmail(ctx context.Context, arg ListPendingOrgInvitesForEmailParams) ([]OrgInvite, error)
}

// SessionInviteStore covers session invite lifecycle.
type SessionInviteStore interface {
	InsertSessionInvite(ctx context.Context, arg InsertSessionInviteParams) (SessionInvite, error)
	GetSessionInviteByID(ctx context.Context, id string) (SessionInvite, error)
	GetSessionInviteByTokenHash(ctx context.Context, tokenHash string) (SessionInvite, error)
	MarkSessionInviteAccepted(ctx context.Context, arg MarkSessionInviteAcceptedParams) error
	ListPendingSessionInvitesForSession(ctx context.Context, arg ListPendingSessionInvitesForSessionParams) ([]SessionInvite, error)
}

// ConflictEvent is a persisted conflict event row.
type ConflictEvent struct {
	ID                 string
	OrgID              string
	SessionID          string
	SourceCommit       string
	DraftTip           string
	Ancestor           string
	Conflicts          string     // JSON text
	AddressedTo        string     // JSON text
	Status             string     // "open" | "resolved"
	ResolvingCommitSHA *string    // nullable
	CreatedAt          time.Time
	ResolvedAt         *time.Time // nullable
}

// InsertConflictEventParams are the parameters for inserting a conflict event row.
type InsertConflictEventParams struct {
	ID                 string
	OrgID              string
	SessionID          string
	SourceCommit       string
	DraftTip           string
	Ancestor           string
	Conflicts          string // JSON text
	AddressedTo        string // JSON text
	Status             string // "open"
	ResolvingCommitSHA *string
	CreatedAt          time.Time
	ResolvedAt         *time.Time
}

// MarkConflictEventResolvedParams are the parameters for resolving a conflict event.
type MarkConflictEventResolvedParams struct {
	ID                 string
	SessionID          string
	ResolvingCommitSHA string
	ResolvedAt         time.Time
}

// ConflictEventStore covers conflict event writes and reads.
type ConflictEventStore interface {
	// InsertConflictEvent inserts a new conflict event row with status "open".
	InsertConflictEvent(ctx context.Context, p InsertConflictEventParams) error
	// GetConflictEventByID returns a conflict event by its ID.
	GetConflictEventByID(ctx context.Context, id string) (ConflictEvent, error)
	// MarkConflictEventResolved marks a conflict event as resolved (no-op if
	// already resolved or not found — the WHERE status='open' guard is safe
	// for replay).
	MarkConflictEventResolved(ctx context.Context, p MarkConflictEventResolvedParams) error
	// ListOpenConflictEventsForSession returns all open conflict events for a
	// session, ordered by created_at ascending.
	ListOpenConflictEventsForSession(ctx context.Context, sessionID string) ([]ConflictEvent, error)
}

// OAuthStateStore covers the transient OAuth state-nonce table.
type OAuthStateStore interface {
	// InsertOAuthState stores a new state nonce with a TTL.
	InsertOAuthState(ctx context.Context, arg InsertOAuthStateParams) error
	// ConsumeOAuthState atomically deletes and returns the nonce row.
	// Returns ErrNotFound when the nonce does not exist (already consumed or
	// never inserted).
	ConsumeOAuthState(ctx context.Context, nonce string) (OAuthState, error)
	// CleanupExpiredOAuthState deletes nonces whose expires_at is before the
	// given time. Intended for operator-cron cleanup.
	CleanupExpiredOAuthState(ctx context.Context, before time.Time) error
}

// Comment is a persisted comment row.
type Comment struct {
	ID                  string
	OrgID               string
	SessionID           string
	AuthorAccountID     string
	AuthorKind          string     // "human" | "agent"
	AnchorCommitSHA     string
	AnchorFilePath      *string    // nullable
	AnchorLineStart     *int32     // nullable
	AnchorLineEnd       *int32     // nullable
	Body                string
	AddressedTo         *string    // nullable
	Kind                string     // "question" | "suggestion" | "action-request" | "fyi"
	CreatedAt           time.Time
	ResolvedAt          *time.Time // nullable
	ResolvedByAccountID *string    // nullable
	ResolutionNote      *string    // nullable
}

// InsertCommentParams are the parameters for inserting a comment row.
type InsertCommentParams struct {
	ID                  string
	OrgID               string
	SessionID           string
	AuthorAccountID     string
	AuthorKind          string
	AnchorCommitSHA     string
	AnchorFilePath      *string
	AnchorLineStart     *int32
	AnchorLineEnd       *int32
	Body                string
	AddressedTo         *string
	Kind                string
	CreatedAt           time.Time
	ResolvedAt          *time.Time
	ResolvedByAccountID *string
	ResolutionNote      *string
}

// ResolveCommentParams are the parameters for resolving a comment.
type ResolveCommentParams struct {
	ID                  string
	SessionID           string
	ResolvedAt          time.Time
	ResolvedByAccountID string
	ResolutionNote      *string
}

// ListCommentsForSessionParams are the parameters for listing comments.
// ResolvedFilter: 0 = all, 1 = resolved only, 2 = unresolved only.
type ListCommentsForSessionParams struct {
	SessionID       string
	AddressedTo     string // substring match; empty = no filter
	Kind            string // exact match; empty = no filter
	ResolvedFilter  int    // 0 = all, 1 = resolved only, 2 = unresolved only
	AnchorCommitSHA string // empty = no filter
	AnchorFilePath  string // empty = no filter
	Before          time.Time
	Limit           int64
}

// CommentStore covers comment writes and reads.
type CommentStore interface {
	// InsertComment inserts a new comment row.
	InsertComment(ctx context.Context, p InsertCommentParams) error
	// GetCommentByID returns a comment by its ID.
	GetCommentByID(ctx context.Context, id string) (Comment, error)
	// ResolveComment marks a comment resolved (no-op if already resolved —
	// the WHERE resolved_at IS NULL guard is safe for replay).
	ResolveComment(ctx context.Context, p ResolveCommentParams) error
	// ListCommentsForSession returns cursor-paginated comments for a session.
	ListCommentsForSession(ctx context.Context, p ListCommentsForSessionParams) ([]Comment, error)
}

// FinalizeLock is a persisted finalize-flow lock row. The authoritative state
// for one in-flight finalize coordination per session. The
// sessions.finalize_locked_by_account_id pointer is a denormalised cache kept
// in sync by lock acquire/release for cheap session-detail responses.
type FinalizeLock struct {
	ID                   string
	OrgID                string
	SessionID            string
	AcquiredByAccountID  string
	AcquiredAt           time.Time
	LastActivityAt       time.Time
	SelectedCommitSHAs   string // JSON array of commit SHAs, stored verbatim
	TargetBranch         string
	BaseSHA              string
	Mode                 string  // "squash" | "preserve"
	CommitMessage        *string // nullable
	SupersededByLockID   *string // nullable; set when overridden
	ReleasedAt           *time.Time
}

// InsertFinalizeLockParams are the parameters for inserting a finalize lock row.
type InsertFinalizeLockParams struct {
	ID                  string
	OrgID               string
	SessionID           string
	AcquiredByAccountID string
	AcquiredAt          time.Time
	LastActivityAt      time.Time
	SelectedCommitSHAs  string
	TargetBranch        string
	BaseSHA             string
	Mode                string
	CommitMessage       *string
	SupersededByLockID  *string
	ReleasedAt          *time.Time
}

// UpdateFinalizeLockCurationParams are the parameters for updating a lock's
// curation state. Bumps last_activity_at atomically with the curation columns.
type UpdateFinalizeLockCurationParams struct {
	ID                 string
	SelectedCommitSHAs string
	TargetBranch       string
	BaseSHA            string
	Mode               string
	CommitMessage      *string
	LastActivityAt     time.Time
}

// TouchFinalizeLockParams refreshes only last_activity_at on a lock.
type TouchFinalizeLockParams struct {
	ID             string
	LastActivityAt time.Time
}

// ReleaseFinalizeLockParams sets released_at on a held lock. Idempotent: the
// underlying query is a no-op if released_at is already set.
type ReleaseFinalizeLockParams struct {
	ID         string
	ReleasedAt time.Time
}

// SupersedeFinalizeLockParams sets superseded_by_lock_id on an older lock
// when a fresh override creates a replacement.
type SupersedeFinalizeLockParams struct {
	ID                 string
	SupersededByLockID string
}

// FinalizeLockStore covers writes and reads on the finalize_locks table.
type FinalizeLockStore interface {
	// InsertFinalizeLock inserts a new lock row.
	InsertFinalizeLock(ctx context.Context, p InsertFinalizeLockParams) error
	// GetFinalizeLockByID returns the lock by its ID, or ErrNotFound.
	GetFinalizeLockByID(ctx context.Context, id string) (FinalizeLock, error)
	// GetActiveFinalizeLockForSession returns the single active lock row
	// (released_at IS NULL AND superseded_by_lock_id IS NULL) for the
	// session, or ErrNotFound when none exists.
	GetActiveFinalizeLockForSession(ctx context.Context, sessionID string) (FinalizeLock, error)
	// UpdateFinalizeLockCuration updates curation columns + last_activity_at.
	UpdateFinalizeLockCuration(ctx context.Context, p UpdateFinalizeLockCurationParams) error
	// TouchFinalizeLock bumps last_activity_at only.
	TouchFinalizeLock(ctx context.Context, p TouchFinalizeLockParams) error
	// ReleaseFinalizeLock sets released_at on the row. Idempotent against
	// already-released rows.
	ReleaseFinalizeLock(ctx context.Context, p ReleaseFinalizeLockParams) error
	// SupersedeFinalizeLock points superseded_by_lock_id at a replacement lock.
	SupersedeFinalizeLock(ctx context.Context, p SupersedeFinalizeLockParams) error
}
