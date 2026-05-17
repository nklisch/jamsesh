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
	ID            string
	OrgID         string
	Name          string
	Goal          string
	WritableScope string     // JSON array, stored verbatim
	DefaultMode   string     // "sync" | "isolated"
	BaseSHA       *string    // nullable until first push
	Status        string     // "active" | "ended" | "archived"
	CreatedAt     time.Time
	EndedAt       *time.Time
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
	UpdateSessionStatus(ctx context.Context, arg UpdateSessionStatusParams) error
	SetSessionBaseSHA(ctx context.Context, arg SetSessionBaseSHAParams) error
	// DeleteSession removes a session row and cascades to session_members via FK.
	DeleteSession(ctx context.Context, arg DeleteSessionParams) error
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
