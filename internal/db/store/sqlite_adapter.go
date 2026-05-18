package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"jamsesh/internal/db/sqlitestore"
)

// compile-time assertion: sqliteAdapter satisfies Store.
var _ Store = (*sqliteAdapter)(nil)

// NewSQLiteAdapter wraps a *sql.DB opened with the modernc.org/sqlite driver
// and returns it as a Store.
func NewSQLiteAdapter(db *sql.DB) Store {
	return &sqliteAdapter{q: sqlitestore.New(db), db: db}
}

type sqliteAdapter struct {
	q  *sqlitestore.Queries
	db *sql.DB
}

func (a *sqliteAdapter) Dialect() string { return "sqlite" }
func (a *sqliteAdapter) Close() error    { return a.db.Close() }
func (a *sqliteAdapter) Ping(ctx context.Context) error { return a.db.PingContext(ctx) }

// RawDB returns the underlying *sql.DB. This is provided for tests that need
// to configure connection-pool settings (e.g. SetMaxOpenConns) on a freshly
// opened in-memory SQLite database. Production code must not use it.
func (a *sqliteAdapter) RawDB() *sql.DB { return a.db }

// ---------------------------------------------------------------------------
// mapSQLiteErr normalises dialect-specific errors to store sentinels.
// ---------------------------------------------------------------------------

func mapSQLiteErr(err error) error {
	if err == nil {
		return nil
	}
	// sql.ErrNoRows covers the standard database/sql path.
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	// modernc.org/sqlite surfaces constraint violations as *sqlite.Error.
	// Both SQLITE_CONSTRAINT_UNIQUE (standalone UNIQUE index) and
	// SQLITE_CONSTRAINT_PRIMARYKEY (composite PRIMARY KEY acting as the
	// uniqueness constraint, e.g. org_members and session_members) map to
	// ErrUniqueViolation — they are semantically the same to callers.
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()
		if code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
			return ErrUniqueViolation
		}
	}
	return err
}

// nullStringToPtr converts sql.NullString to *string for domain types.
func nullStringToPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

// ptrToNullString converts *string to sql.NullString for query params.
func ptrToNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// nullTimeToPtr converts sql.NullTime to *time.Time for domain types.
func nullTimeToPtr(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	t := nt.Time
	return &t
}

// ptrToNullTime converts *time.Time to sql.NullTime for query params.
func ptrToNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// ---------------------------------------------------------------------------
// Row mappers
// ---------------------------------------------------------------------------

func sqliteOrg(row sqlitestore.Org) Org {
	return Org{
		ID:                  row.ID,
		Name:                row.Name,
		Slug:                row.Slug,
		CreatedAt:           row.CreatedAt,
		SessionInvitePolicy: row.SessionInvitePolicy,
	}
}

func sqliteAccount(row sqlitestore.Account) Account {
	return Account{
		ID:           row.ID,
		Email:        row.Email,
		DisplayName:  row.DisplayName,
		GithubUserID: nullStringToPtr(row.GithubUserID),
		CreatedAt:    row.CreatedAt,
	}
}

func sqliteOrgMember(row sqlitestore.OrgMember) OrgMember {
	return OrgMember{
		OrgID:     row.OrgID,
		AccountID: row.AccountID,
		Role:      row.Role,
		CreatedAt: row.CreatedAt,
	}
}

func sqliteOrgMemberWithAccount(row sqlitestore.ListOrgMembersRow) OrgMemberWithAccount {
	return OrgMemberWithAccount{
		AccountID:        row.ID,
		Email:            row.Email,
		DisplayName:      row.DisplayName,
		GithubUserID:     nullStringToPtr(row.GithubUserID),
		AccountCreatedAt: row.CreatedAt,
		Role:             row.Role,
		CreatedAt:        row.JoinedAt,
	}
}

func sqliteSession(row sqlitestore.Session) Session {
	return Session{
		ID:                        row.ID,
		OrgID:                     row.OrgID,
		Name:                      row.Name,
		Goal:                      row.Goal,
		WritableScope:             row.WritableScope,
		DefaultMode:               row.DefaultMode,
		BaseSHA:                   row.BaseSha,
		Status:                    row.Status,
		CreatedAt:                 row.CreatedAt,
		EndedAt:                   row.EndedAt,
		EndReason:                 nullStringToPtr(row.EndReason),
		FinalizeLockedByAccountID: nullStringToPtr(row.FinalizeLockedByAccountID),
	}
}

func sqliteRefMode(row sqlitestore.RefMode) RefMode {
	return RefMode{
		SessionID: row.SessionID,
		Ref:       row.Ref,
		Mode:      row.Mode,
	}
}

func sqliteSessionMember(row sqlitestore.SessionMember) SessionMember {
	return SessionMember{
		OrgID:     row.OrgID,
		SessionID: row.SessionID,
		AccountID: row.AccountID,
		Role:      row.Role,
		JoinedAt:  row.JoinedAt,
	}
}

func sqliteSessionMembership(row sqlitestore.ListSessionMembershipsForAccountRow) SessionMembership {
	return SessionMembership{
		OrgID:         row.OrgID,
		SessionID:     row.SessionID,
		AccountID:     row.AccountID,
		Role:          row.Role,
		JoinedAt:      row.JoinedAt,
		SessionName:   row.SessionName,
		SessionStatus: row.SessionStatus,
		SessionGoal:   row.SessionGoal,
	}
}

func sqliteOAuthToken(row sqlitestore.OauthToken) OAuthToken {
	return OAuthToken{
		ID:         row.ID,
		AccountID:  row.AccountID,
		TokenHash:  row.TokenHash,
		Kind:       row.Kind,
		IssuedAt:   row.IssuedAt,
		ExpiresAt:  row.ExpiresAt,
		LastUsedAt: row.LastUsedAt,
		RevokedAt:  row.RevokedAt,
	}
}

func sqliteMagicLinkToken(row sqlitestore.MagicLinkToken) MagicLinkToken {
	return MagicLinkToken{
		ID:        row.ID,
		TokenHash: row.TokenHash,
		Email:     row.Email,
		IssuedAt:  row.IssuedAt,
		ExpiresAt: row.ExpiresAt,
		UsedAt:    row.UsedAt,
	}
}

func sqliteArchivedSession(row sqlitestore.ArchivedSession) ArchivedSession {
	var ids []string
	_ = json.Unmarshal([]byte(row.MemberAccountIds), &ids)
	if ids == nil {
		ids = []string{}
	}
	// row.EndedAt is *time.Time due to the global *.ended_at sqlc override, but
	// the schema marks it NOT NULL — it will never be nil in practice.
	var endedAt time.Time
	if row.EndedAt != nil {
		endedAt = *row.EndedAt
	}
	return ArchivedSession{
		SessionID:        row.SessionID,
		OrgID:            row.OrgID,
		Name:             row.Name,
		GoalText:         row.GoalText,
		MemberAccountIDs: ids,
		EndedAt:          endedAt,
		ArchivedAt:       row.ArchivedAt,
		EndReason:        row.EndReason,
		FinalBranchName:  nullStringToPtr(row.FinalBranchName),
	}
}

// ---------------------------------------------------------------------------
// OrgStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) CreateOrg(ctx context.Context, p CreateOrgParams) (Org, error) {
	row, err := a.q.CreateOrg(ctx, sqlitestore.CreateOrgParams{
		ID:        p.ID,
		Name:      p.Name,
		Slug:      p.Slug,
		CreatedAt: p.CreatedAt,
	})
	if err != nil {
		return Org{}, mapSQLiteErr(err)
	}
	return sqliteOrg(row), nil
}

func (a *sqliteAdapter) GetOrgByID(ctx context.Context, id string) (Org, error) {
	row, err := a.q.GetOrgByID(ctx, id)
	if err != nil {
		return Org{}, mapSQLiteErr(err)
	}
	return sqliteOrg(row), nil
}

func (a *sqliteAdapter) GetOrgBySlug(ctx context.Context, slug string) (Org, error) {
	row, err := a.q.GetOrgBySlug(ctx, slug)
	if err != nil {
		return Org{}, mapSQLiteErr(err)
	}
	return sqliteOrg(row), nil
}

func (a *sqliteAdapter) UpdateOrgSessionInvitePolicy(ctx context.Context, p UpdateOrgSessionInvitePolicyParams) error {
	return mapSQLiteErr(a.q.UpdateOrgSessionInvitePolicy(ctx, sqlitestore.UpdateOrgSessionInvitePolicyParams{
		SessionInvitePolicy: p.SessionInvitePolicy,
		ID:                  p.ID,
	}))
}

// ---------------------------------------------------------------------------
// AccountStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) CreateAccount(ctx context.Context, p CreateAccountParams) (Account, error) {
	row, err := a.q.CreateAccount(ctx, sqlitestore.CreateAccountParams{
		ID:           p.ID,
		Email:        p.Email,
		DisplayName:  p.DisplayName,
		GithubUserID: ptrToNullString(p.GithubUserID),
		CreatedAt:    p.CreatedAt,
	})
	if err != nil {
		return Account{}, mapSQLiteErr(err)
	}
	return sqliteAccount(row), nil
}

func (a *sqliteAdapter) GetAccountByID(ctx context.Context, id string) (Account, error) {
	row, err := a.q.GetAccountByID(ctx, id)
	if err != nil {
		return Account{}, mapSQLiteErr(err)
	}
	return sqliteAccount(row), nil
}

func (a *sqliteAdapter) GetAccountByEmail(ctx context.Context, email string) (Account, error) {
	row, err := a.q.GetAccountByEmail(ctx, email)
	if err != nil {
		return Account{}, mapSQLiteErr(err)
	}
	return sqliteAccount(row), nil
}

func (a *sqliteAdapter) GetAccountByGitHubUserID(ctx context.Context, githubUserID *string) (Account, error) {
	row, err := a.q.GetAccountByGitHubUserID(ctx, ptrToNullString(githubUserID))
	if err != nil {
		return Account{}, mapSQLiteErr(err)
	}
	return sqliteAccount(row), nil
}

func (a *sqliteAdapter) UpdateAccountDisplayName(ctx context.Context, p UpdateAccountDisplayNameParams) error {
	return mapSQLiteErr(a.q.UpdateAccountDisplayName(ctx, sqlitestore.UpdateAccountDisplayNameParams{
		ID:          p.ID,
		DisplayName: p.DisplayName,
	}))
}

// ---------------------------------------------------------------------------
// OrgMemberStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) AddOrgMember(ctx context.Context, p AddOrgMemberParams) error {
	return mapSQLiteErr(a.q.AddOrgMember(ctx, sqlitestore.AddOrgMemberParams{
		OrgID:     p.OrgID,
		AccountID: p.AccountID,
		Role:      p.Role,
		CreatedAt: p.CreatedAt,
	}))
}

func (a *sqliteAdapter) GetOrgMember(ctx context.Context, p GetOrgMemberParams) (OrgMember, error) {
	row, err := a.q.GetOrgMember(ctx, sqlitestore.GetOrgMemberParams{
		OrgID:     p.OrgID,
		AccountID: p.AccountID,
	})
	if err != nil {
		return OrgMember{}, mapSQLiteErr(err)
	}
	return sqliteOrgMember(row), nil
}

func (a *sqliteAdapter) ListOrgsForAccount(ctx context.Context, accountID string) ([]Org, error) {
	rows, err := a.q.ListOrgsForAccount(ctx, accountID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	orgs := make([]Org, len(rows))
	for i, r := range rows {
		orgs[i] = sqliteOrg(r)
	}
	return orgs, nil
}

func (a *sqliteAdapter) ListOrgMembers(ctx context.Context, orgID string) ([]OrgMemberWithAccount, error) {
	rows, err := a.q.ListOrgMembers(ctx, orgID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	members := make([]OrgMemberWithAccount, len(rows))
	for i, r := range rows {
		members[i] = sqliteOrgMemberWithAccount(r)
		members[i].OrgID = orgID
	}
	return members, nil
}

func (a *sqliteAdapter) RemoveOrgMember(ctx context.Context, p RemoveOrgMemberParams) error {
	return mapSQLiteErr(a.q.RemoveOrgMember(ctx, sqlitestore.RemoveOrgMemberParams{
		OrgID:     p.OrgID,
		AccountID: p.AccountID,
	}))
}

// ---------------------------------------------------------------------------
// SessionStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) CreateSession(ctx context.Context, p CreateSessionParams) (Session, error) {
	row, err := a.q.CreateSession(ctx, sqlitestore.CreateSessionParams{
		ID:            p.ID,
		OrgID:         p.OrgID,
		Name:          p.Name,
		Goal:          p.Goal,
		WritableScope: p.WritableScope,
		DefaultMode:   p.DefaultMode,
		BaseSha:       p.BaseSHA,
		Status:        p.Status,
		CreatedAt:     p.CreatedAt,
		EndedAt:       p.EndedAt,
	})
	if err != nil {
		return Session{}, mapSQLiteErr(err)
	}
	return sqliteSession(row), nil
}

func (a *sqliteAdapter) GetSession(ctx context.Context, orgID, id string) (Session, error) {
	row, err := a.q.GetSession(ctx, sqlitestore.GetSessionParams{
		OrgID: orgID,
		ID:    id,
	})
	if err != nil {
		return Session{}, mapSQLiteErr(err)
	}
	return sqliteSession(row), nil
}

// GetSessionByID looks up a session by its primary key without org scoping.
// Intentional cross-org exception: the org_id returned on the Session is used
// by the LifecycleManager to route subsequent org-scoped operations.
func (a *sqliteAdapter) GetSessionByID(ctx context.Context, id string) (Session, error) {
	row, err := a.q.GetSessionByID(ctx, id)
	if err != nil {
		return Session{}, mapSQLiteErr(err)
	}
	return sqliteSession(row), nil
}

func (a *sqliteAdapter) ListSessionsForOrg(ctx context.Context, orgID string) ([]Session, error) {
	rows, err := a.q.ListSessionsForOrg(ctx, orgID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	sessions := make([]Session, len(rows))
	for i, r := range rows {
		sessions[i] = sqliteSession(r)
	}
	return sessions, nil
}

func (a *sqliteAdapter) ListSessionsForOrgWithCursor(ctx context.Context, p ListSessionsForOrgWithCursorParams) ([]Session, error) {
	rows, err := a.q.ListSessionsForOrgWithCursor(ctx, sqlitestore.ListSessionsForOrgWithCursorParams{
		OrgID:     p.OrgID,
		CreatedAt: p.Before,
		Limit:     p.Limit,
	})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	sessions := make([]Session, len(rows))
	for i, r := range rows {
		sessions[i] = sqliteSession(r)
	}
	return sessions, nil
}

func (a *sqliteAdapter) UpdateSessionStatus(ctx context.Context, p UpdateSessionStatusParams) error {
	return mapSQLiteErr(a.q.UpdateSessionStatus(ctx, sqlitestore.UpdateSessionStatusParams{
		OrgID:  p.OrgID,
		ID:     p.ID,
		Status: p.Status,
	}))
}

func (a *sqliteAdapter) SetSessionBaseSHA(ctx context.Context, p SetSessionBaseSHAParams) error {
	return mapSQLiteErr(a.q.SetSessionBaseSHA(ctx, sqlitestore.SetSessionBaseSHAParams{
		OrgID:   p.OrgID,
		ID:      p.ID,
		BaseSha: p.BaseSHA,
	}))
}

func (a *sqliteAdapter) UpdateSessionGoalScopeMode(ctx context.Context, p UpdateSessionGoalScopeModeParams) error {
	return mapSQLiteErr(a.q.UpdateSessionGoalScopeMode(ctx, sqlitestore.UpdateSessionGoalScopeModeParams{
		OrgID:         p.OrgID,
		ID:            p.ID,
		Goal:          p.Goal,
		WritableScope: p.WritableScope,
		DefaultMode:   p.DefaultMode,
	}))
}

func (a *sqliteAdapter) SetSessionEndReason(ctx context.Context, p SetSessionEndReasonParams) error {
	return mapSQLiteErr(a.q.SetSessionEndReason(ctx, sqlitestore.SetSessionEndReasonParams{
		OrgID:     p.OrgID,
		ID:        p.ID,
		EndReason: ptrToNullString(p.EndReason),
		EndedAt:   p.EndedAt,
	}))
}

func (a *sqliteAdapter) SetFinalizeLock(ctx context.Context, p SetFinalizeLockParams) error {
	return mapSQLiteErr(a.q.SetFinalizeLock(ctx, sqlitestore.SetFinalizeLockParams{
		OrgID:                     p.OrgID,
		ID:                        p.ID,
		FinalizeLockedByAccountID: ptrToNullString(p.AccountID),
	}))
}

func (a *sqliteAdapter) ClearFinalizeLock(ctx context.Context, p ClearFinalizeLockParams) error {
	return mapSQLiteErr(a.q.ClearFinalizeLock(ctx, sqlitestore.ClearFinalizeLockParams{
		OrgID: p.OrgID,
		ID:    p.ID,
	}))
}

func (a *sqliteAdapter) DeleteSession(ctx context.Context, p DeleteSessionParams) error {
	return mapSQLiteErr(a.q.DeleteSession(ctx, sqlitestore.DeleteSessionParams{
		OrgID: p.OrgID,
		ID:    p.ID,
	}))
}

// ---------------------------------------------------------------------------
// SessionMemberStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) AddSessionMember(ctx context.Context, p AddSessionMemberParams) error {
	return mapSQLiteErr(a.q.AddSessionMember(ctx, sqlitestore.AddSessionMemberParams{
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
		AccountID: p.AccountID,
		Role:      p.Role,
		JoinedAt:  p.JoinedAt,
	}))
}

func (a *sqliteAdapter) GetSessionMember(ctx context.Context, p GetSessionMemberParams) (SessionMember, error) {
	row, err := a.q.GetSessionMember(ctx, sqlitestore.GetSessionMemberParams{
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
		AccountID: p.AccountID,
	})
	if err != nil {
		return SessionMember{}, mapSQLiteErr(err)
	}
	return sqliteSessionMember(row), nil
}

func (a *sqliteAdapter) ListSessionMembers(ctx context.Context, p ListSessionMembersParams) ([]SessionMember, error) {
	rows, err := a.q.ListSessionMembers(ctx, sqlitestore.ListSessionMembersParams{
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
	})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	members := make([]SessionMember, len(rows))
	for i, r := range rows {
		members[i] = sqliteSessionMember(r)
	}
	return members, nil
}

func (a *sqliteAdapter) RemoveSessionMember(ctx context.Context, p RemoveSessionMemberParams) error {
	return mapSQLiteErr(a.q.RemoveSessionMember(ctx, sqlitestore.RemoveSessionMemberParams{
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
		AccountID: p.AccountID,
	}))
}

func (a *sqliteAdapter) ListSessionMembershipsForAccount(ctx context.Context, accountID string) ([]SessionMembership, error) {
	rows, err := a.q.ListSessionMembershipsForAccount(ctx, accountID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	memberships := make([]SessionMembership, len(rows))
	for i, r := range rows {
		memberships[i] = sqliteSessionMembership(r)
	}
	return memberships, nil
}

// ---------------------------------------------------------------------------
// OAuthTokenStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) CreateOAuthToken(ctx context.Context, p CreateOAuthTokenParams) (OAuthToken, error) {
	row, err := a.q.CreateOAuthToken(ctx, sqlitestore.CreateOAuthTokenParams{
		ID:         p.ID,
		AccountID:  p.AccountID,
		TokenHash:  p.TokenHash,
		Kind:       p.Kind,
		IssuedAt:   p.IssuedAt,
		ExpiresAt:  p.ExpiresAt,
		LastUsedAt: p.LastUsedAt,
		RevokedAt:  p.RevokedAt,
	})
	if err != nil {
		return OAuthToken{}, mapSQLiteErr(err)
	}
	return sqliteOAuthToken(row), nil
}

func (a *sqliteAdapter) GetOAuthTokenByHash(ctx context.Context, tokenHash string) (OAuthToken, error) {
	row, err := a.q.GetOAuthTokenByHash(ctx, tokenHash)
	if err != nil {
		return OAuthToken{}, mapSQLiteErr(err)
	}
	return sqliteOAuthToken(row), nil
}

func (a *sqliteAdapter) TouchOAuthTokenLastUsed(ctx context.Context, p TouchOAuthTokenLastUsedParams) error {
	return mapSQLiteErr(a.q.TouchOAuthTokenLastUsed(ctx, sqlitestore.TouchOAuthTokenLastUsedParams{
		ID:         p.ID,
		LastUsedAt: p.LastUsedAt,
	}))
}

func (a *sqliteAdapter) RevokeOAuthToken(ctx context.Context, p RevokeOAuthTokenParams) error {
	return mapSQLiteErr(a.q.RevokeOAuthToken(ctx, sqlitestore.RevokeOAuthTokenParams{
		ID:        p.ID,
		RevokedAt: p.RevokedAt,
	}))
}

func (a *sqliteAdapter) RevokeAllOAuthTokensForAccount(ctx context.Context, p RevokeAllOAuthTokensForAccountParams) error {
	return mapSQLiteErr(a.q.RevokeAllOAuthTokensForAccount(ctx, sqlitestore.RevokeAllOAuthTokensForAccountParams{
		AccountID: p.AccountID,
		RevokedAt: p.RevokedAt,
	}))
}

func (a *sqliteAdapter) ListOAuthTokensForAccount(ctx context.Context, accountID string) ([]OAuthToken, error) {
	rows, err := a.q.ListOAuthTokensForAccount(ctx, accountID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	tokens := make([]OAuthToken, len(rows))
	for i, r := range rows {
		tokens[i] = sqliteOAuthToken(r)
	}
	return tokens, nil
}

// ---------------------------------------------------------------------------
// MagicLinkTokenStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) CreateMagicLinkToken(ctx context.Context, p CreateMagicLinkTokenParams) (MagicLinkToken, error) {
	row, err := a.q.CreateMagicLinkToken(ctx, sqlitestore.CreateMagicLinkTokenParams{
		ID:        p.ID,
		TokenHash: p.TokenHash,
		Email:     p.Email,
		IssuedAt:  p.IssuedAt,
		ExpiresAt: p.ExpiresAt,
		UsedAt:    p.UsedAt,
	})
	if err != nil {
		return MagicLinkToken{}, mapSQLiteErr(err)
	}
	return sqliteMagicLinkToken(row), nil
}

func (a *sqliteAdapter) GetMagicLinkTokenByHash(ctx context.Context, tokenHash string) (MagicLinkToken, error) {
	row, err := a.q.GetMagicLinkTokenByHash(ctx, tokenHash)
	if err != nil {
		return MagicLinkToken{}, mapSQLiteErr(err)
	}
	return sqliteMagicLinkToken(row), nil
}

func (a *sqliteAdapter) ConsumeMagicLinkToken(ctx context.Context, p ConsumeMagicLinkTokenParams) error {
	return mapSQLiteErr(a.q.ConsumeMagicLinkToken(ctx, sqlitestore.ConsumeMagicLinkTokenParams{
		ID:     p.ID,
		UsedAt: p.UsedAt,
	}))
}

// ---------------------------------------------------------------------------
// ArchivedSessionStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) InsertArchivedSession(ctx context.Context, p InsertArchivedSessionParams) error {
	endedAt := p.EndedAt // time.Time → *time.Time for sqlc-generated param
	return mapSQLiteErr(a.q.InsertArchivedSession(ctx, sqlitestore.InsertArchivedSessionParams{
		SessionID:        p.SessionID,
		OrgID:            p.OrgID,
		Name:             p.Name,
		GoalText:         p.GoalText,
		MemberAccountIds: p.MemberAccountIDs,
		EndedAt:          &endedAt,
		ArchivedAt:       p.ArchivedAt,
		EndReason:        p.EndReason,
		FinalBranchName:  ptrToNullString(p.FinalBranchName),
	}))
}

func (a *sqliteAdapter) GetArchivedSession(ctx context.Context, p GetArchivedSessionParams) (ArchivedSession, error) {
	row, err := a.q.GetArchivedSession(ctx, sqlitestore.GetArchivedSessionParams{
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
	})
	if err != nil {
		return ArchivedSession{}, mapSQLiteErr(err)
	}
	return sqliteArchivedSession(row), nil
}

// ---------------------------------------------------------------------------
// OAuthStateStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) InsertOAuthState(ctx context.Context, p InsertOAuthStateParams) error {
	return mapSQLiteErr(a.q.InsertOAuthState(ctx, sqlitestore.InsertOAuthStateParams{
		Nonce:       p.Nonce,
		Provider:    p.Provider,
		RedirectUri: p.RedirectURI,
		CreatedAt:   p.CreatedAt,
		ExpiresAt:   p.ExpiresAt,
	}))
}

func (a *sqliteAdapter) ConsumeOAuthState(ctx context.Context, nonce string) (OAuthState, error) {
	row, err := a.q.ConsumeOAuthState(ctx, nonce)
	if err != nil {
		return OAuthState{}, mapSQLiteErr(err)
	}
	return OAuthState{
		Nonce:       row.Nonce,
		Provider:    row.Provider,
		RedirectURI: row.RedirectUri,
		CreatedAt:   row.CreatedAt,
		ExpiresAt:   row.ExpiresAt,
	}, nil
}

func (a *sqliteAdapter) CleanupExpiredOAuthState(ctx context.Context, before time.Time) error {
	return mapSQLiteErr(a.q.CleanupExpiredOAuthState(ctx, before))
}

// ---------------------------------------------------------------------------
// EventLogStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) EnsureEventSeqRow(ctx context.Context, sessionID string) error {
	return mapSQLiteErr(a.q.EnsureEventSeqRow(ctx, sessionID))
}

func (a *sqliteAdapter) AllocateNextSeq(ctx context.Context, sessionID string) (int64, error) {
	seq, err := a.q.AllocateNextSeq(ctx, sessionID)
	return seq, mapSQLiteErr(err)
}

func (a *sqliteAdapter) AllocateNextSeqN(ctx context.Context, sessionID string, n int64) (int64, error) {
	seq, err := a.q.AllocateNextSeqN(ctx, sqlitestore.AllocateNextSeqNParams{
		Next:      n,
		SessionID: sessionID,
	})
	return seq, mapSQLiteErr(err)
}

func (a *sqliteAdapter) InsertEvent(ctx context.Context, p InsertEventParams) error {
	return mapSQLiteErr(a.q.InsertEvent(ctx, sqlitestore.InsertEventParams{
		ID:        p.ID,
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
		Seq:       p.Seq,
		Type:      p.Type,
		Payload:   p.Payload,
		CreatedAt: p.CreatedAt,
	}))
}

func (a *sqliteAdapter) ListEventsSince(ctx context.Context, p ListEventsSinceParams) ([]Event, error) {
	rows, err := a.q.ListEventsSince(ctx, sqlitestore.ListEventsSinceParams{
		SessionID: p.SessionID,
		Seq:       p.SinceSeq,
		Limit:     p.Limit,
	})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{
			ID:        r.ID,
			OrgID:     r.OrgID,
			SessionID: r.SessionID,
			Seq:       r.Seq,
			Type:      r.Type,
			Payload:   r.Payload,
			CreatedAt: r.CreatedAt,
		}
	}
	return events, nil
}

func (a *sqliteAdapter) ListEventsSinceForDigest(ctx context.Context, p ListEventsSinceForDigestParams) ([]Event, error) {
	rows, err := a.q.ListEventsSinceForDigest(ctx, sqlitestore.ListEventsSinceForDigestParams{
		SessionID: p.SessionID,
		Seq:       p.SinceSeq,
		Limit:     p.Limit,
	})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{
			ID:        r.ID,
			OrgID:     r.OrgID,
			SessionID: r.SessionID,
			Seq:       r.Seq,
			Type:      r.Type,
			Payload:   r.Payload,
			CreatedAt: r.CreatedAt,
		}
	}
	return events, nil
}

// ---------------------------------------------------------------------------
// PresenceStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) UpsertPresence(ctx context.Context, p UpsertPresenceParams) error {
	return mapSQLiteErr(a.q.UpsertPresence(ctx, sqlitestore.UpsertPresenceParams{
		OrgID:        p.OrgID,
		SessionID:    p.SessionID,
		AccountID:    p.AccountID,
		Ref:          p.Ref,
		CurrentSha:   p.CurrentSHA,
		LastActiveAt: p.LastActiveAt,
	}))
}

func (a *sqliteAdapter) ListPresenceForSession(ctx context.Context, sessionID string) ([]PresenceRow, error) {
	rows, err := a.q.ListPresenceForSession(ctx, sessionID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	out := make([]PresenceRow, len(rows))
	for i, r := range rows {
		out[i] = PresenceRow{
			OrgID:        r.OrgID,
			SessionID:    r.SessionID,
			AccountID:    r.AccountID,
			Ref:          r.Ref,
			CurrentSHA:   r.CurrentSha,
			LastActiveAt: r.LastActiveAt,
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// OrgInviteStore
// ---------------------------------------------------------------------------

func sqliteOrgInvite(row sqlitestore.OrgInvite) OrgInvite {
	return OrgInvite{
		ID:                  row.ID,
		OrgID:               row.OrgID,
		InviterAccountID:    row.InviterAccountID,
		RecipientEmail:      row.RecipientEmail,
		TokenHash:           row.TokenHash,
		CreatedAt:           row.CreatedAt,
		ExpiresAt:           row.ExpiresAt,
		AcceptedAt:          row.AcceptedAt,
		AcceptedByAccountID: nullStringToPtr(row.AcceptedByAccountID),
	}
}

func (a *sqliteAdapter) InsertOrgInvite(ctx context.Context, p InsertOrgInviteParams) (OrgInvite, error) {
	row, err := a.q.InsertOrgInvite(ctx, sqlitestore.InsertOrgInviteParams{
		ID:                  p.ID,
		OrgID:               p.OrgID,
		InviterAccountID:    p.InviterAccountID,
		RecipientEmail:      p.RecipientEmail,
		TokenHash:           p.TokenHash,
		CreatedAt:           p.CreatedAt,
		ExpiresAt:           p.ExpiresAt,
		AcceptedAt:          p.AcceptedAt,
		AcceptedByAccountID: ptrToNullString(p.AcceptedByAccountID),
	})
	if err != nil {
		return OrgInvite{}, mapSQLiteErr(err)
	}
	return sqliteOrgInvite(row), nil
}

func (a *sqliteAdapter) GetOrgInviteByID(ctx context.Context, id string) (OrgInvite, error) {
	row, err := a.q.GetOrgInviteByID(ctx, id)
	if err != nil {
		return OrgInvite{}, mapSQLiteErr(err)
	}
	return sqliteOrgInvite(row), nil
}

func (a *sqliteAdapter) GetOrgInviteByTokenHash(ctx context.Context, tokenHash string) (OrgInvite, error) {
	row, err := a.q.GetOrgInviteByTokenHash(ctx, tokenHash)
	if err != nil {
		return OrgInvite{}, mapSQLiteErr(err)
	}
	return sqliteOrgInvite(row), nil
}

func (a *sqliteAdapter) MarkOrgInviteAccepted(ctx context.Context, p MarkOrgInviteAcceptedParams) error {
	return mapSQLiteErr(a.q.MarkOrgInviteAccepted(ctx, sqlitestore.MarkOrgInviteAcceptedParams{
		ID:                  p.ID,
		AcceptedAt:          &p.AcceptedAt,
		AcceptedByAccountID: ptrToNullString(&p.AcceptedByAccountID),
	}))
}

func (a *sqliteAdapter) ListPendingOrgInvitesForOrg(ctx context.Context, p ListPendingOrgInvitesForOrgParams) ([]OrgInvite, error) {
	rows, err := a.q.ListPendingOrgInvitesForOrg(ctx, sqlitestore.ListPendingOrgInvitesForOrgParams{
		OrgID:     p.OrgID,
		ExpiresAt: p.Now,
	})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	invites := make([]OrgInvite, len(rows))
	for i, r := range rows {
		invites[i] = sqliteOrgInvite(r)
	}
	return invites, nil
}

func (a *sqliteAdapter) ListPendingOrgInvitesForEmail(ctx context.Context, p ListPendingOrgInvitesForEmailParams) ([]OrgInvite, error) {
	rows, err := a.q.ListPendingOrgInvitesForEmail(ctx, sqlitestore.ListPendingOrgInvitesForEmailParams{
		RecipientEmail: p.Email,
		ExpiresAt:      p.Now,
	})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	invites := make([]OrgInvite, len(rows))
	for i, r := range rows {
		invites[i] = sqliteOrgInvite(r)
	}
	return invites, nil
}

// ---------------------------------------------------------------------------
// SessionInviteStore
// ---------------------------------------------------------------------------

func sqliteSessionInvite(row sqlitestore.SessionInvite) SessionInvite {
	return SessionInvite{
		ID:                  row.ID,
		OrgID:               row.OrgID,
		SessionID:           row.SessionID,
		InviterAccountID:    row.InviterAccountID,
		InviteeEmail:        row.InviteeEmail,
		TokenHash:           row.TokenHash,
		CreatedAt:           row.CreatedAt,
		ExpiresAt:           row.ExpiresAt,
		AcceptedAt:          row.AcceptedAt,
		AcceptedByAccountID: nullStringToPtr(row.AcceptedByAccountID),
	}
}

func (a *sqliteAdapter) InsertSessionInvite(ctx context.Context, p InsertSessionInviteParams) (SessionInvite, error) {
	row, err := a.q.InsertSessionInvite(ctx, sqlitestore.InsertSessionInviteParams{
		ID:                  p.ID,
		OrgID:               p.OrgID,
		SessionID:           p.SessionID,
		InviterAccountID:    p.InviterAccountID,
		InviteeEmail:        p.InviteeEmail,
		TokenHash:           p.TokenHash,
		CreatedAt:           p.CreatedAt,
		ExpiresAt:           p.ExpiresAt,
		AcceptedAt:          p.AcceptedAt,
		AcceptedByAccountID: ptrToNullString(p.AcceptedByAccountID),
	})
	if err != nil {
		return SessionInvite{}, mapSQLiteErr(err)
	}
	return sqliteSessionInvite(row), nil
}

func (a *sqliteAdapter) GetSessionInviteByID(ctx context.Context, id string) (SessionInvite, error) {
	row, err := a.q.GetSessionInviteByID(ctx, id)
	if err != nil {
		return SessionInvite{}, mapSQLiteErr(err)
	}
	return sqliteSessionInvite(row), nil
}

func (a *sqliteAdapter) GetSessionInviteByTokenHash(ctx context.Context, tokenHash string) (SessionInvite, error) {
	row, err := a.q.GetSessionInviteByTokenHash(ctx, tokenHash)
	if err != nil {
		return SessionInvite{}, mapSQLiteErr(err)
	}
	return sqliteSessionInvite(row), nil
}

func (a *sqliteAdapter) MarkSessionInviteAccepted(ctx context.Context, p MarkSessionInviteAcceptedParams) error {
	return mapSQLiteErr(a.q.MarkSessionInviteAccepted(ctx, sqlitestore.MarkSessionInviteAcceptedParams{
		ID:                  p.ID,
		AcceptedAt:          &p.AcceptedAt,
		AcceptedByAccountID: ptrToNullString(&p.AcceptedByAccountID),
	}))
}

func (a *sqliteAdapter) ListPendingSessionInvitesForSession(ctx context.Context, p ListPendingSessionInvitesForSessionParams) ([]SessionInvite, error) {
	rows, err := a.q.ListPendingSessionInvitesForSession(ctx, sqlitestore.ListPendingSessionInvitesForSessionParams{
		SessionID: p.SessionID,
		ExpiresAt: p.Now,
	})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	invites := make([]SessionInvite, len(rows))
	for i, r := range rows {
		invites[i] = sqliteSessionInvite(r)
	}
	return invites, nil
}

// ---------------------------------------------------------------------------
// RefModeStore
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) UpsertRefMode(ctx context.Context, p UpsertRefModeParams) error {
	return mapSQLiteErr(a.q.UpsertRefMode(ctx, sqlitestore.UpsertRefModeParams{
		SessionID: p.SessionID,
		Ref:       p.Ref,
		Mode:      p.Mode,
	}))
}

func (a *sqliteAdapter) GetRefMode(ctx context.Context, p GetRefModeParams) (RefMode, error) {
	row, err := a.q.GetRefMode(ctx, sqlitestore.GetRefModeParams{
		SessionID: p.SessionID,
		Ref:       p.Ref,
	})
	if err != nil {
		return RefMode{}, mapSQLiteErr(err)
	}
	return sqliteRefMode(row), nil
}

func (a *sqliteAdapter) ListRefModesForSession(ctx context.Context, sessionID string) ([]RefMode, error) {
	rows, err := a.q.ListRefModesForSession(ctx, sessionID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	out := make([]RefMode, len(rows))
	for i, r := range rows {
		out[i] = sqliteRefMode(r)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// WithTx
// ---------------------------------------------------------------------------

// sqliteTxStore wraps a *sqlitestore.Queries scoped to a transaction and
// satisfies TxStore.
type sqliteTxStore struct {
	q *sqlitestore.Queries
}

var _ TxStore = (*sqliteTxStore)(nil)

func (a *sqliteAdapter) WithTx(ctx context.Context, fn func(TxStore) error) error {
	// BEGIN IMMEDIATE acquires a write-lock upfront — necessary for SQLite to
	// avoid SQLITE_BUSY when multiple goroutines try to write concurrently.
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}
	// sqlitestore.New accepts a DBTX; *sql.Tx satisfies that interface.
	txq := sqlitestore.New(tx)
	ts := &sqliteTxStore{q: txq}
	if err := fn(ts); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit tx: %w", mapSQLiteErr(err))
	}
	return nil
}

// Delegate all TxStore methods to the underlying *sqlitestore.Queries.
// OrgStore
func (s *sqliteTxStore) CreateOrg(ctx context.Context, p CreateOrgParams) (Org, error) {
	row, err := s.q.CreateOrg(ctx, sqlitestore.CreateOrgParams{ID: p.ID, Name: p.Name, Slug: p.Slug, CreatedAt: p.CreatedAt})
	if err != nil {
		return Org{}, mapSQLiteErr(err)
	}
	return sqliteOrg(row), nil
}
func (s *sqliteTxStore) GetOrgByID(ctx context.Context, id string) (Org, error) {
	row, err := s.q.GetOrgByID(ctx, id)
	if err != nil {
		return Org{}, mapSQLiteErr(err)
	}
	return sqliteOrg(row), nil
}
func (s *sqliteTxStore) GetOrgBySlug(ctx context.Context, slug string) (Org, error) {
	row, err := s.q.GetOrgBySlug(ctx, slug)
	if err != nil {
		return Org{}, mapSQLiteErr(err)
	}
	return sqliteOrg(row), nil
}
func (s *sqliteTxStore) UpdateOrgSessionInvitePolicy(ctx context.Context, p UpdateOrgSessionInvitePolicyParams) error {
	return mapSQLiteErr(s.q.UpdateOrgSessionInvitePolicy(ctx, sqlitestore.UpdateOrgSessionInvitePolicyParams{
		SessionInvitePolicy: p.SessionInvitePolicy,
		ID:                  p.ID,
	}))
}

// AccountStore
func (s *sqliteTxStore) CreateAccount(ctx context.Context, p CreateAccountParams) (Account, error) {
	row, err := s.q.CreateAccount(ctx, sqlitestore.CreateAccountParams{ID: p.ID, Email: p.Email, DisplayName: p.DisplayName, GithubUserID: ptrToNullString(p.GithubUserID), CreatedAt: p.CreatedAt})
	if err != nil {
		return Account{}, mapSQLiteErr(err)
	}
	return sqliteAccount(row), nil
}
func (s *sqliteTxStore) GetAccountByID(ctx context.Context, id string) (Account, error) {
	row, err := s.q.GetAccountByID(ctx, id)
	if err != nil {
		return Account{}, mapSQLiteErr(err)
	}
	return sqliteAccount(row), nil
}
func (s *sqliteTxStore) GetAccountByEmail(ctx context.Context, email string) (Account, error) {
	row, err := s.q.GetAccountByEmail(ctx, email)
	if err != nil {
		return Account{}, mapSQLiteErr(err)
	}
	return sqliteAccount(row), nil
}
func (s *sqliteTxStore) GetAccountByGitHubUserID(ctx context.Context, githubUserID *string) (Account, error) {
	row, err := s.q.GetAccountByGitHubUserID(ctx, ptrToNullString(githubUserID))
	if err != nil {
		return Account{}, mapSQLiteErr(err)
	}
	return sqliteAccount(row), nil
}
func (s *sqliteTxStore) UpdateAccountDisplayName(ctx context.Context, p UpdateAccountDisplayNameParams) error {
	return mapSQLiteErr(s.q.UpdateAccountDisplayName(ctx, sqlitestore.UpdateAccountDisplayNameParams{ID: p.ID, DisplayName: p.DisplayName}))
}

// OrgMemberStore
func (s *sqliteTxStore) AddOrgMember(ctx context.Context, p AddOrgMemberParams) error {
	return mapSQLiteErr(s.q.AddOrgMember(ctx, sqlitestore.AddOrgMemberParams{OrgID: p.OrgID, AccountID: p.AccountID, Role: p.Role, CreatedAt: p.CreatedAt}))
}
func (s *sqliteTxStore) GetOrgMember(ctx context.Context, p GetOrgMemberParams) (OrgMember, error) {
	row, err := s.q.GetOrgMember(ctx, sqlitestore.GetOrgMemberParams{OrgID: p.OrgID, AccountID: p.AccountID})
	if err != nil {
		return OrgMember{}, mapSQLiteErr(err)
	}
	return sqliteOrgMember(row), nil
}
func (s *sqliteTxStore) ListOrgsForAccount(ctx context.Context, accountID string) ([]Org, error) {
	rows, err := s.q.ListOrgsForAccount(ctx, accountID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	orgs := make([]Org, len(rows))
	for i, r := range rows {
		orgs[i] = sqliteOrg(r)
	}
	return orgs, nil
}
func (s *sqliteTxStore) ListOrgMembers(ctx context.Context, orgID string) ([]OrgMemberWithAccount, error) {
	rows, err := s.q.ListOrgMembers(ctx, orgID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	members := make([]OrgMemberWithAccount, len(rows))
	for i, r := range rows {
		members[i] = sqliteOrgMemberWithAccount(r)
		members[i].OrgID = orgID
	}
	return members, nil
}
func (s *sqliteTxStore) RemoveOrgMember(ctx context.Context, p RemoveOrgMemberParams) error {
	return mapSQLiteErr(s.q.RemoveOrgMember(ctx, sqlitestore.RemoveOrgMemberParams{OrgID: p.OrgID, AccountID: p.AccountID}))
}

// SessionStore
func (s *sqliteTxStore) CreateSession(ctx context.Context, p CreateSessionParams) (Session, error) {
	row, err := s.q.CreateSession(ctx, sqlitestore.CreateSessionParams{ID: p.ID, OrgID: p.OrgID, Name: p.Name, Goal: p.Goal, WritableScope: p.WritableScope, DefaultMode: p.DefaultMode, BaseSha: p.BaseSHA, Status: p.Status, CreatedAt: p.CreatedAt, EndedAt: p.EndedAt})
	if err != nil {
		return Session{}, mapSQLiteErr(err)
	}
	return sqliteSession(row), nil
}
func (s *sqliteTxStore) GetSession(ctx context.Context, orgID, id string) (Session, error) {
	row, err := s.q.GetSession(ctx, sqlitestore.GetSessionParams{OrgID: orgID, ID: id})
	if err != nil {
		return Session{}, mapSQLiteErr(err)
	}
	return sqliteSession(row), nil
}
func (s *sqliteTxStore) GetSessionByID(ctx context.Context, id string) (Session, error) {
	row, err := s.q.GetSessionByID(ctx, id)
	if err != nil {
		return Session{}, mapSQLiteErr(err)
	}
	return sqliteSession(row), nil
}
func (s *sqliteTxStore) ListSessionsForOrg(ctx context.Context, orgID string) ([]Session, error) {
	rows, err := s.q.ListSessionsForOrg(ctx, orgID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	sessions := make([]Session, len(rows))
	for i, r := range rows {
		sessions[i] = sqliteSession(r)
	}
	return sessions, nil
}
func (s *sqliteTxStore) ListSessionsForOrgWithCursor(ctx context.Context, p ListSessionsForOrgWithCursorParams) ([]Session, error) {
	rows, err := s.q.ListSessionsForOrgWithCursor(ctx, sqlitestore.ListSessionsForOrgWithCursorParams{OrgID: p.OrgID, CreatedAt: p.Before, Limit: p.Limit})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	sessions := make([]Session, len(rows))
	for i, r := range rows {
		sessions[i] = sqliteSession(r)
	}
	return sessions, nil
}
func (s *sqliteTxStore) UpdateSessionStatus(ctx context.Context, p UpdateSessionStatusParams) error {
	return mapSQLiteErr(s.q.UpdateSessionStatus(ctx, sqlitestore.UpdateSessionStatusParams{OrgID: p.OrgID, ID: p.ID, Status: p.Status}))
}
func (s *sqliteTxStore) SetSessionBaseSHA(ctx context.Context, p SetSessionBaseSHAParams) error {
	return mapSQLiteErr(s.q.SetSessionBaseSHA(ctx, sqlitestore.SetSessionBaseSHAParams{OrgID: p.OrgID, ID: p.ID, BaseSha: p.BaseSHA}))
}
func (s *sqliteTxStore) UpdateSessionGoalScopeMode(ctx context.Context, p UpdateSessionGoalScopeModeParams) error {
	return mapSQLiteErr(s.q.UpdateSessionGoalScopeMode(ctx, sqlitestore.UpdateSessionGoalScopeModeParams{OrgID: p.OrgID, ID: p.ID, Goal: p.Goal, WritableScope: p.WritableScope, DefaultMode: p.DefaultMode}))
}
func (s *sqliteTxStore) SetSessionEndReason(ctx context.Context, p SetSessionEndReasonParams) error {
	return mapSQLiteErr(s.q.SetSessionEndReason(ctx, sqlitestore.SetSessionEndReasonParams{OrgID: p.OrgID, ID: p.ID, EndReason: ptrToNullString(p.EndReason), EndedAt: p.EndedAt}))
}
func (s *sqliteTxStore) SetFinalizeLock(ctx context.Context, p SetFinalizeLockParams) error {
	return mapSQLiteErr(s.q.SetFinalizeLock(ctx, sqlitestore.SetFinalizeLockParams{OrgID: p.OrgID, ID: p.ID, FinalizeLockedByAccountID: ptrToNullString(p.AccountID)}))
}
func (s *sqliteTxStore) ClearFinalizeLock(ctx context.Context, p ClearFinalizeLockParams) error {
	return mapSQLiteErr(s.q.ClearFinalizeLock(ctx, sqlitestore.ClearFinalizeLockParams{OrgID: p.OrgID, ID: p.ID}))
}
func (s *sqliteTxStore) DeleteSession(ctx context.Context, p DeleteSessionParams) error {
	return mapSQLiteErr(s.q.DeleteSession(ctx, sqlitestore.DeleteSessionParams{OrgID: p.OrgID, ID: p.ID}))
}

// SessionMemberStore
func (s *sqliteTxStore) AddSessionMember(ctx context.Context, p AddSessionMemberParams) error {
	return mapSQLiteErr(s.q.AddSessionMember(ctx, sqlitestore.AddSessionMemberParams{OrgID: p.OrgID, SessionID: p.SessionID, AccountID: p.AccountID, Role: p.Role, JoinedAt: p.JoinedAt}))
}
func (s *sqliteTxStore) GetSessionMember(ctx context.Context, p GetSessionMemberParams) (SessionMember, error) {
	row, err := s.q.GetSessionMember(ctx, sqlitestore.GetSessionMemberParams{OrgID: p.OrgID, SessionID: p.SessionID, AccountID: p.AccountID})
	if err != nil {
		return SessionMember{}, mapSQLiteErr(err)
	}
	return sqliteSessionMember(row), nil
}
func (s *sqliteTxStore) ListSessionMembers(ctx context.Context, p ListSessionMembersParams) ([]SessionMember, error) {
	rows, err := s.q.ListSessionMembers(ctx, sqlitestore.ListSessionMembersParams{OrgID: p.OrgID, SessionID: p.SessionID})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	members := make([]SessionMember, len(rows))
	for i, r := range rows {
		members[i] = sqliteSessionMember(r)
	}
	return members, nil
}
func (s *sqliteTxStore) RemoveSessionMember(ctx context.Context, p RemoveSessionMemberParams) error {
	return mapSQLiteErr(s.q.RemoveSessionMember(ctx, sqlitestore.RemoveSessionMemberParams{OrgID: p.OrgID, SessionID: p.SessionID, AccountID: p.AccountID}))
}
func (s *sqliteTxStore) ListSessionMembershipsForAccount(ctx context.Context, accountID string) ([]SessionMembership, error) {
	rows, err := s.q.ListSessionMembershipsForAccount(ctx, accountID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	memberships := make([]SessionMembership, len(rows))
	for i, r := range rows {
		memberships[i] = sqliteSessionMembership(r)
	}
	return memberships, nil
}

// OAuthTokenStore
func (s *sqliteTxStore) CreateOAuthToken(ctx context.Context, p CreateOAuthTokenParams) (OAuthToken, error) {
	row, err := s.q.CreateOAuthToken(ctx, sqlitestore.CreateOAuthTokenParams{ID: p.ID, AccountID: p.AccountID, TokenHash: p.TokenHash, Kind: p.Kind, IssuedAt: p.IssuedAt, ExpiresAt: p.ExpiresAt, LastUsedAt: p.LastUsedAt, RevokedAt: p.RevokedAt})
	if err != nil {
		return OAuthToken{}, mapSQLiteErr(err)
	}
	return sqliteOAuthToken(row), nil
}
func (s *sqliteTxStore) GetOAuthTokenByHash(ctx context.Context, tokenHash string) (OAuthToken, error) {
	row, err := s.q.GetOAuthTokenByHash(ctx, tokenHash)
	if err != nil {
		return OAuthToken{}, mapSQLiteErr(err)
	}
	return sqliteOAuthToken(row), nil
}
func (s *sqliteTxStore) TouchOAuthTokenLastUsed(ctx context.Context, p TouchOAuthTokenLastUsedParams) error {
	return mapSQLiteErr(s.q.TouchOAuthTokenLastUsed(ctx, sqlitestore.TouchOAuthTokenLastUsedParams{ID: p.ID, LastUsedAt: p.LastUsedAt}))
}
func (s *sqliteTxStore) RevokeOAuthToken(ctx context.Context, p RevokeOAuthTokenParams) error {
	return mapSQLiteErr(s.q.RevokeOAuthToken(ctx, sqlitestore.RevokeOAuthTokenParams{ID: p.ID, RevokedAt: p.RevokedAt}))
}
func (s *sqliteTxStore) RevokeAllOAuthTokensForAccount(ctx context.Context, p RevokeAllOAuthTokensForAccountParams) error {
	return mapSQLiteErr(s.q.RevokeAllOAuthTokensForAccount(ctx, sqlitestore.RevokeAllOAuthTokensForAccountParams{AccountID: p.AccountID, RevokedAt: p.RevokedAt}))
}
func (s *sqliteTxStore) ListOAuthTokensForAccount(ctx context.Context, accountID string) ([]OAuthToken, error) {
	rows, err := s.q.ListOAuthTokensForAccount(ctx, accountID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	tokens := make([]OAuthToken, len(rows))
	for i, r := range rows {
		tokens[i] = sqliteOAuthToken(r)
	}
	return tokens, nil
}

// MagicLinkTokenStore
func (s *sqliteTxStore) CreateMagicLinkToken(ctx context.Context, p CreateMagicLinkTokenParams) (MagicLinkToken, error) {
	row, err := s.q.CreateMagicLinkToken(ctx, sqlitestore.CreateMagicLinkTokenParams{ID: p.ID, TokenHash: p.TokenHash, Email: p.Email, IssuedAt: p.IssuedAt, ExpiresAt: p.ExpiresAt, UsedAt: p.UsedAt})
	if err != nil {
		return MagicLinkToken{}, mapSQLiteErr(err)
	}
	return sqliteMagicLinkToken(row), nil
}
func (s *sqliteTxStore) GetMagicLinkTokenByHash(ctx context.Context, tokenHash string) (MagicLinkToken, error) {
	row, err := s.q.GetMagicLinkTokenByHash(ctx, tokenHash)
	if err != nil {
		return MagicLinkToken{}, mapSQLiteErr(err)
	}
	return sqliteMagicLinkToken(row), nil
}
func (s *sqliteTxStore) ConsumeMagicLinkToken(ctx context.Context, p ConsumeMagicLinkTokenParams) error {
	return mapSQLiteErr(s.q.ConsumeMagicLinkToken(ctx, sqlitestore.ConsumeMagicLinkTokenParams{ID: p.ID, UsedAt: p.UsedAt}))
}

// ArchivedSessionStore
func (s *sqliteTxStore) InsertArchivedSession(ctx context.Context, p InsertArchivedSessionParams) error {
	endedAt := p.EndedAt
	return mapSQLiteErr(s.q.InsertArchivedSession(ctx, sqlitestore.InsertArchivedSessionParams{SessionID: p.SessionID, OrgID: p.OrgID, Name: p.Name, GoalText: p.GoalText, MemberAccountIds: p.MemberAccountIDs, EndedAt: &endedAt, ArchivedAt: p.ArchivedAt, EndReason: p.EndReason, FinalBranchName: ptrToNullString(p.FinalBranchName)}))
}
func (s *sqliteTxStore) GetArchivedSession(ctx context.Context, p GetArchivedSessionParams) (ArchivedSession, error) {
	row, err := s.q.GetArchivedSession(ctx, sqlitestore.GetArchivedSessionParams{OrgID: p.OrgID, SessionID: p.SessionID})
	if err != nil {
		return ArchivedSession{}, mapSQLiteErr(err)
	}
	return sqliteArchivedSession(row), nil
}

// OAuthStateStore
func (s *sqliteTxStore) InsertOAuthState(ctx context.Context, p InsertOAuthStateParams) error {
	return mapSQLiteErr(s.q.InsertOAuthState(ctx, sqlitestore.InsertOAuthStateParams{Nonce: p.Nonce, Provider: p.Provider, RedirectUri: p.RedirectURI, CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt}))
}
func (s *sqliteTxStore) ConsumeOAuthState(ctx context.Context, nonce string) (OAuthState, error) {
	row, err := s.q.ConsumeOAuthState(ctx, nonce)
	if err != nil {
		return OAuthState{}, mapSQLiteErr(err)
	}
	return OAuthState{Nonce: row.Nonce, Provider: row.Provider, RedirectURI: row.RedirectUri, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}, nil
}
func (s *sqliteTxStore) CleanupExpiredOAuthState(ctx context.Context, before time.Time) error {
	return mapSQLiteErr(s.q.CleanupExpiredOAuthState(ctx, before))
}

// EventLogStore
func (s *sqliteTxStore) EnsureEventSeqRow(ctx context.Context, sessionID string) error {
	return mapSQLiteErr(s.q.EnsureEventSeqRow(ctx, sessionID))
}
func (s *sqliteTxStore) AllocateNextSeq(ctx context.Context, sessionID string) (int64, error) {
	seq, err := s.q.AllocateNextSeq(ctx, sessionID)
	return seq, mapSQLiteErr(err)
}
func (s *sqliteTxStore) AllocateNextSeqN(ctx context.Context, sessionID string, n int64) (int64, error) {
	seq, err := s.q.AllocateNextSeqN(ctx, sqlitestore.AllocateNextSeqNParams{Next: n, SessionID: sessionID})
	return seq, mapSQLiteErr(err)
}
func (s *sqliteTxStore) InsertEvent(ctx context.Context, p InsertEventParams) error {
	return mapSQLiteErr(s.q.InsertEvent(ctx, sqlitestore.InsertEventParams{ID: p.ID, OrgID: p.OrgID, SessionID: p.SessionID, Seq: p.Seq, Type: p.Type, Payload: p.Payload, CreatedAt: p.CreatedAt}))
}
func (s *sqliteTxStore) ListEventsSince(ctx context.Context, p ListEventsSinceParams) ([]Event, error) {
	rows, err := s.q.ListEventsSince(ctx, sqlitestore.ListEventsSinceParams{SessionID: p.SessionID, Seq: p.SinceSeq, Limit: p.Limit})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{ID: r.ID, OrgID: r.OrgID, SessionID: r.SessionID, Seq: r.Seq, Type: r.Type, Payload: r.Payload, CreatedAt: r.CreatedAt}
	}
	return events, nil
}
func (s *sqliteTxStore) ListEventsSinceForDigest(ctx context.Context, p ListEventsSinceForDigestParams) ([]Event, error) {
	rows, err := s.q.ListEventsSinceForDigest(ctx, sqlitestore.ListEventsSinceForDigestParams{SessionID: p.SessionID, Seq: p.SinceSeq, Limit: p.Limit})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{ID: r.ID, OrgID: r.OrgID, SessionID: r.SessionID, Seq: r.Seq, Type: r.Type, Payload: r.Payload, CreatedAt: r.CreatedAt}
	}
	return events, nil
}

// PresenceStore
func (s *sqliteTxStore) UpsertPresence(ctx context.Context, p UpsertPresenceParams) error {
	return mapSQLiteErr(s.q.UpsertPresence(ctx, sqlitestore.UpsertPresenceParams{OrgID: p.OrgID, SessionID: p.SessionID, AccountID: p.AccountID, Ref: p.Ref, CurrentSha: p.CurrentSHA, LastActiveAt: p.LastActiveAt}))
}
func (s *sqliteTxStore) ListPresenceForSession(ctx context.Context, sessionID string) ([]PresenceRow, error) {
	rows, err := s.q.ListPresenceForSession(ctx, sessionID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	out := make([]PresenceRow, len(rows))
	for i, r := range rows {
		out[i] = PresenceRow{OrgID: r.OrgID, SessionID: r.SessionID, AccountID: r.AccountID, Ref: r.Ref, CurrentSHA: r.CurrentSha, LastActiveAt: r.LastActiveAt}
	}
	return out, nil
}

// OrgInviteStore
func (s *sqliteTxStore) InsertOrgInvite(ctx context.Context, p InsertOrgInviteParams) (OrgInvite, error) {
	row, err := s.q.InsertOrgInvite(ctx, sqlitestore.InsertOrgInviteParams{ID: p.ID, OrgID: p.OrgID, InviterAccountID: p.InviterAccountID, RecipientEmail: p.RecipientEmail, TokenHash: p.TokenHash, CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt, AcceptedAt: p.AcceptedAt, AcceptedByAccountID: ptrToNullString(p.AcceptedByAccountID)})
	if err != nil {
		return OrgInvite{}, mapSQLiteErr(err)
	}
	return sqliteOrgInvite(row), nil
}
func (s *sqliteTxStore) GetOrgInviteByID(ctx context.Context, id string) (OrgInvite, error) {
	row, err := s.q.GetOrgInviteByID(ctx, id)
	if err != nil {
		return OrgInvite{}, mapSQLiteErr(err)
	}
	return sqliteOrgInvite(row), nil
}
func (s *sqliteTxStore) GetOrgInviteByTokenHash(ctx context.Context, tokenHash string) (OrgInvite, error) {
	row, err := s.q.GetOrgInviteByTokenHash(ctx, tokenHash)
	if err != nil {
		return OrgInvite{}, mapSQLiteErr(err)
	}
	return sqliteOrgInvite(row), nil
}
func (s *sqliteTxStore) MarkOrgInviteAccepted(ctx context.Context, p MarkOrgInviteAcceptedParams) error {
	return mapSQLiteErr(s.q.MarkOrgInviteAccepted(ctx, sqlitestore.MarkOrgInviteAcceptedParams{ID: p.ID, AcceptedAt: &p.AcceptedAt, AcceptedByAccountID: ptrToNullString(&p.AcceptedByAccountID)}))
}
func (s *sqliteTxStore) ListPendingOrgInvitesForOrg(ctx context.Context, p ListPendingOrgInvitesForOrgParams) ([]OrgInvite, error) {
	rows, err := s.q.ListPendingOrgInvitesForOrg(ctx, sqlitestore.ListPendingOrgInvitesForOrgParams{OrgID: p.OrgID, ExpiresAt: p.Now})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	invites := make([]OrgInvite, len(rows))
	for i, r := range rows {
		invites[i] = sqliteOrgInvite(r)
	}
	return invites, nil
}
func (s *sqliteTxStore) ListPendingOrgInvitesForEmail(ctx context.Context, p ListPendingOrgInvitesForEmailParams) ([]OrgInvite, error) {
	rows, err := s.q.ListPendingOrgInvitesForEmail(ctx, sqlitestore.ListPendingOrgInvitesForEmailParams{RecipientEmail: p.Email, ExpiresAt: p.Now})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	invites := make([]OrgInvite, len(rows))
	for i, r := range rows {
		invites[i] = sqliteOrgInvite(r)
	}
	return invites, nil
}

// RefModeStore
func (s *sqliteTxStore) UpsertRefMode(ctx context.Context, p UpsertRefModeParams) error {
	return mapSQLiteErr(s.q.UpsertRefMode(ctx, sqlitestore.UpsertRefModeParams{SessionID: p.SessionID, Ref: p.Ref, Mode: p.Mode}))
}
func (s *sqliteTxStore) GetRefMode(ctx context.Context, p GetRefModeParams) (RefMode, error) {
	row, err := s.q.GetRefMode(ctx, sqlitestore.GetRefModeParams{SessionID: p.SessionID, Ref: p.Ref})
	if err != nil {
		return RefMode{}, mapSQLiteErr(err)
	}
	return sqliteRefMode(row), nil
}
func (s *sqliteTxStore) ListRefModesForSession(ctx context.Context, sessionID string) ([]RefMode, error) {
	rows, err := s.q.ListRefModesForSession(ctx, sessionID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	out := make([]RefMode, len(rows))
	for i, r := range rows {
		out[i] = sqliteRefMode(r)
	}
	return out, nil
}

// SessionInviteStore (TxStore)
func (s *sqliteTxStore) InsertSessionInvite(ctx context.Context, p InsertSessionInviteParams) (SessionInvite, error) {
	row, err := s.q.InsertSessionInvite(ctx, sqlitestore.InsertSessionInviteParams{ID: p.ID, OrgID: p.OrgID, SessionID: p.SessionID, InviterAccountID: p.InviterAccountID, InviteeEmail: p.InviteeEmail, TokenHash: p.TokenHash, CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt, AcceptedAt: p.AcceptedAt, AcceptedByAccountID: ptrToNullString(p.AcceptedByAccountID)})
	if err != nil {
		return SessionInvite{}, mapSQLiteErr(err)
	}
	return sqliteSessionInvite(row), nil
}
func (s *sqliteTxStore) GetSessionInviteByID(ctx context.Context, id string) (SessionInvite, error) {
	row, err := s.q.GetSessionInviteByID(ctx, id)
	if err != nil {
		return SessionInvite{}, mapSQLiteErr(err)
	}
	return sqliteSessionInvite(row), nil
}
func (s *sqliteTxStore) GetSessionInviteByTokenHash(ctx context.Context, tokenHash string) (SessionInvite, error) {
	row, err := s.q.GetSessionInviteByTokenHash(ctx, tokenHash)
	if err != nil {
		return SessionInvite{}, mapSQLiteErr(err)
	}
	return sqliteSessionInvite(row), nil
}
func (s *sqliteTxStore) MarkSessionInviteAccepted(ctx context.Context, p MarkSessionInviteAcceptedParams) error {
	return mapSQLiteErr(s.q.MarkSessionInviteAccepted(ctx, sqlitestore.MarkSessionInviteAcceptedParams{ID: p.ID, AcceptedAt: &p.AcceptedAt, AcceptedByAccountID: ptrToNullString(&p.AcceptedByAccountID)}))
}
func (s *sqliteTxStore) ListPendingSessionInvitesForSession(ctx context.Context, p ListPendingSessionInvitesForSessionParams) ([]SessionInvite, error) {
	rows, err := s.q.ListPendingSessionInvitesForSession(ctx, sqlitestore.ListPendingSessionInvitesForSessionParams{SessionID: p.SessionID, ExpiresAt: p.Now})
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	invites := make([]SessionInvite, len(rows))
	for i, r := range rows {
		invites[i] = sqliteSessionInvite(r)
	}
	return invites, nil
}

// ---------------------------------------------------------------------------
// ConflictEventStore (outer adapter)
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) InsertConflictEvent(ctx context.Context, p InsertConflictEventParams) error {
	return mapSQLiteErr(a.q.InsertConflictEvent(ctx, sqlitestore.InsertConflictEventParams{
		ID:                 p.ID,
		OrgID:              p.OrgID,
		SessionID:          p.SessionID,
		SourceCommit:       p.SourceCommit,
		DraftTip:           p.DraftTip,
		Ancestor:           p.Ancestor,
		Conflicts:          p.Conflicts,
		AddressedTo:        p.AddressedTo,
		Status:             p.Status,
		ResolvingCommitSha: ptrToNullString(p.ResolvingCommitSHA),
		CreatedAt:          p.CreatedAt,
		ResolvedAt:         ptrToNullTime(p.ResolvedAt),
	}))
}

func (a *sqliteAdapter) GetConflictEventByID(ctx context.Context, id string) (ConflictEvent, error) {
	row, err := a.q.GetConflictEventByID(ctx, id)
	if err != nil {
		return ConflictEvent{}, mapSQLiteErr(err)
	}
	return sqliteConflictEvent(row), nil
}

func (a *sqliteAdapter) MarkConflictEventResolved(ctx context.Context, p MarkConflictEventResolvedParams) error {
	return mapSQLiteErr(a.q.MarkConflictEventResolved(ctx, sqlitestore.MarkConflictEventResolvedParams{
		ID:                 p.ID,
		SessionID:          p.SessionID,
		ResolvingCommitSha: ptrToNullString(&p.ResolvingCommitSHA),
		ResolvedAt:         ptrToNullTime(&p.ResolvedAt),
	}))
}

func (a *sqliteAdapter) ListOpenConflictEventsForSession(ctx context.Context, sessionID string) ([]ConflictEvent, error) {
	rows, err := a.q.ListOpenConflictEventsForSession(ctx, sessionID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	out := make([]ConflictEvent, len(rows))
	for i, r := range rows {
		out[i] = sqliteConflictEvent(r)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// ConflictEventStore (TxStore)
// ---------------------------------------------------------------------------

func (s *sqliteTxStore) InsertConflictEvent(ctx context.Context, p InsertConflictEventParams) error {
	return mapSQLiteErr(s.q.InsertConflictEvent(ctx, sqlitestore.InsertConflictEventParams{
		ID:                 p.ID,
		OrgID:              p.OrgID,
		SessionID:          p.SessionID,
		SourceCommit:       p.SourceCommit,
		DraftTip:           p.DraftTip,
		Ancestor:           p.Ancestor,
		Conflicts:          p.Conflicts,
		AddressedTo:        p.AddressedTo,
		Status:             p.Status,
		ResolvingCommitSha: ptrToNullString(p.ResolvingCommitSHA),
		CreatedAt:          p.CreatedAt,
		ResolvedAt:         ptrToNullTime(p.ResolvedAt),
	}))
}

func (s *sqliteTxStore) GetConflictEventByID(ctx context.Context, id string) (ConflictEvent, error) {
	row, err := s.q.GetConflictEventByID(ctx, id)
	if err != nil {
		return ConflictEvent{}, mapSQLiteErr(err)
	}
	return sqliteConflictEvent(row), nil
}

func (s *sqliteTxStore) MarkConflictEventResolved(ctx context.Context, p MarkConflictEventResolvedParams) error {
	return mapSQLiteErr(s.q.MarkConflictEventResolved(ctx, sqlitestore.MarkConflictEventResolvedParams{
		ID:                 p.ID,
		SessionID:          p.SessionID,
		ResolvingCommitSha: ptrToNullString(&p.ResolvingCommitSHA),
		ResolvedAt:         ptrToNullTime(&p.ResolvedAt),
	}))
}

func (s *sqliteTxStore) ListOpenConflictEventsForSession(ctx context.Context, sessionID string) ([]ConflictEvent, error) {
	rows, err := s.q.ListOpenConflictEventsForSession(ctx, sessionID)
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	out := make([]ConflictEvent, len(rows))
	for i, r := range rows {
		out[i] = sqliteConflictEvent(r)
	}
	return out, nil
}

// sqliteConflictEvent converts a sqlitestore.ConflictEvent to domain ConflictEvent.
func sqliteConflictEvent(r sqlitestore.ConflictEvent) ConflictEvent {
	return ConflictEvent{
		ID:                 r.ID,
		OrgID:              r.OrgID,
		SessionID:          r.SessionID,
		SourceCommit:       r.SourceCommit,
		DraftTip:           r.DraftTip,
		Ancestor:           r.Ancestor,
		Conflicts:          r.Conflicts,
		AddressedTo:        r.AddressedTo,
		Status:             r.Status,
		ResolvingCommitSHA: nullStringToPtr(r.ResolvingCommitSha),
		CreatedAt:          r.CreatedAt,
		ResolvedAt:         nullTimeToPtr(r.ResolvedAt),
	}
}

// ---------------------------------------------------------------------------
// CommentStore (outer adapter)
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) InsertComment(ctx context.Context, p InsertCommentParams) error {
	return mapSQLiteErr(a.q.InsertComment(ctx, sqliteInsertCommentParams(p)))
}

func (a *sqliteAdapter) GetCommentByID(ctx context.Context, id string) (Comment, error) {
	row, err := a.q.GetCommentByID(ctx, id)
	if err != nil {
		return Comment{}, mapSQLiteErr(err)
	}
	return sqliteComment(row), nil
}

func (a *sqliteAdapter) ResolveComment(ctx context.Context, p ResolveCommentParams) error {
	return mapSQLiteErr(a.q.ResolveComment(ctx, sqlitestore.ResolveCommentParams{
		ID:                  p.ID,
		SessionID:           p.SessionID,
		ResolvedAt:          sql.NullTime{Time: p.ResolvedAt, Valid: true},
		ResolvedByAccountID: sql.NullString{String: p.ResolvedByAccountID, Valid: true},
		ResolutionNote:      ptrToNullString(p.ResolutionNote),
	}))
}

func (a *sqliteAdapter) ListCommentsForSession(ctx context.Context, p ListCommentsForSessionParams) ([]Comment, error) {
	rows, err := a.q.ListCommentsForSession(ctx, sqliteListCommentsParams(p))
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	out := make([]Comment, len(rows))
	for i, r := range rows {
		out[i] = sqliteComment(r)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// CommentStore (TxStore)
// ---------------------------------------------------------------------------

func (s *sqliteTxStore) InsertComment(ctx context.Context, p InsertCommentParams) error {
	return mapSQLiteErr(s.q.InsertComment(ctx, sqliteInsertCommentParams(p)))
}

func (s *sqliteTxStore) GetCommentByID(ctx context.Context, id string) (Comment, error) {
	row, err := s.q.GetCommentByID(ctx, id)
	if err != nil {
		return Comment{}, mapSQLiteErr(err)
	}
	return sqliteComment(row), nil
}

func (s *sqliteTxStore) ResolveComment(ctx context.Context, p ResolveCommentParams) error {
	return mapSQLiteErr(s.q.ResolveComment(ctx, sqlitestore.ResolveCommentParams{
		ID:                  p.ID,
		SessionID:           p.SessionID,
		ResolvedAt:          sql.NullTime{Time: p.ResolvedAt, Valid: true},
		ResolvedByAccountID: sql.NullString{String: p.ResolvedByAccountID, Valid: true},
		ResolutionNote:      ptrToNullString(p.ResolutionNote),
	}))
}

func (s *sqliteTxStore) ListCommentsForSession(ctx context.Context, p ListCommentsForSessionParams) ([]Comment, error) {
	rows, err := s.q.ListCommentsForSession(ctx, sqliteListCommentsParams(p))
	if err != nil {
		return nil, mapSQLiteErr(err)
	}
	out := make([]Comment, len(rows))
	for i, r := range rows {
		out[i] = sqliteComment(r)
	}
	return out, nil
}

// sqliteInsertCommentParams converts domain InsertCommentParams to sqlitestore.InsertCommentParams.
func sqliteInsertCommentParams(p InsertCommentParams) sqlitestore.InsertCommentParams {
	var lineStart, lineEnd sql.NullInt64
	if p.AnchorLineStart != nil {
		lineStart = sql.NullInt64{Int64: int64(*p.AnchorLineStart), Valid: true}
	}
	if p.AnchorLineEnd != nil {
		lineEnd = sql.NullInt64{Int64: int64(*p.AnchorLineEnd), Valid: true}
	}
	return sqlitestore.InsertCommentParams{
		ID:                  p.ID,
		OrgID:               p.OrgID,
		SessionID:           p.SessionID,
		AuthorAccountID:     p.AuthorAccountID,
		AuthorKind:          p.AuthorKind,
		AnchorCommitSha:     p.AnchorCommitSHA,
		AnchorFilePath:      ptrToNullString(p.AnchorFilePath),
		AnchorLineStart:     lineStart,
		AnchorLineEnd:       lineEnd,
		Body:                p.Body,
		AddressedTo:         ptrToNullString(p.AddressedTo),
		Kind:                p.Kind,
		CreatedAt:           p.CreatedAt,
		ResolvedAt:          ptrToNullTime(p.ResolvedAt),
		ResolvedByAccountID: ptrToNullString(p.ResolvedByAccountID),
		ResolutionNote:      ptrToNullString(p.ResolutionNote),
	}
}

// sqliteListCommentsParams converts domain ListCommentsForSessionParams to sqlitestore.ListCommentsForSessionParams.
func sqliteListCommentsParams(p ListCommentsForSessionParams) sqlitestore.ListCommentsForSessionParams {
	addrFilter := ""
	if p.AddressedTo != "" {
		addrFilter = p.AddressedTo
	}
	return sqlitestore.ListCommentsForSessionParams{
		SessionID:       p.SessionID,
		Column2:         addrFilter,
		Column3:         sql.NullString{String: addrFilter, Valid: addrFilter != ""},
		Column4:         p.Kind,
		Kind:            p.Kind,
		Column6:         p.ResolvedFilter,
		Column7:         p.ResolvedFilter,
		Column8:         p.ResolvedFilter,
		Column9:         p.AnchorCommitSHA,
		AnchorCommitSha: p.AnchorCommitSHA,
		Column11:        p.AnchorFilePath,
		AnchorFilePath:  sql.NullString{String: p.AnchorFilePath, Valid: p.AnchorFilePath != ""},
		CreatedAt:       p.Before,
		Limit:           p.Limit,
	}
}

// sqliteComment converts a sqlitestore.Comment to domain Comment.
func sqliteComment(r sqlitestore.Comment) Comment {
	var lineStart, lineEnd *int32
	if r.AnchorLineStart.Valid {
		v := int32(r.AnchorLineStart.Int64)
		lineStart = &v
	}
	if r.AnchorLineEnd.Valid {
		v := int32(r.AnchorLineEnd.Int64)
		lineEnd = &v
	}
	return Comment{
		ID:                  r.ID,
		OrgID:               r.OrgID,
		SessionID:           r.SessionID,
		AuthorAccountID:     r.AuthorAccountID,
		AuthorKind:          r.AuthorKind,
		AnchorCommitSHA:     r.AnchorCommitSha,
		AnchorFilePath:      nullStringToPtr(r.AnchorFilePath),
		AnchorLineStart:     lineStart,
		AnchorLineEnd:       lineEnd,
		Body:                r.Body,
		AddressedTo:         nullStringToPtr(r.AddressedTo),
		Kind:                r.Kind,
		CreatedAt:           r.CreatedAt,
		ResolvedAt:          nullTimeToPtr(r.ResolvedAt),
		ResolvedByAccountID: nullStringToPtr(r.ResolvedByAccountID),
		ResolutionNote:      nullStringToPtr(r.ResolutionNote),
	}
}

// ---------------------------------------------------------------------------
// FinalizeLockStore (outer adapter)
// ---------------------------------------------------------------------------

func (a *sqliteAdapter) InsertFinalizeLock(ctx context.Context, p InsertFinalizeLockParams) error {
	return mapSQLiteErr(a.q.InsertFinalizeLock(ctx, sqliteInsertFinalizeLockParams(p)))
}

func (a *sqliteAdapter) GetFinalizeLockByID(ctx context.Context, id string) (FinalizeLock, error) {
	row, err := a.q.GetFinalizeLockByID(ctx, id)
	if err != nil {
		return FinalizeLock{}, mapSQLiteErr(err)
	}
	return sqliteFinalizeLock(row), nil
}

func (a *sqliteAdapter) GetActiveFinalizeLockForSession(ctx context.Context, sessionID string) (FinalizeLock, error) {
	row, err := a.q.GetActiveFinalizeLockForSession(ctx, sessionID)
	if err != nil {
		return FinalizeLock{}, mapSQLiteErr(err)
	}
	return sqliteFinalizeLock(row), nil
}

func (a *sqliteAdapter) UpdateFinalizeLockCuration(ctx context.Context, p UpdateFinalizeLockCurationParams) error {
	baseSHA := p.BaseSHA
	return mapSQLiteErr(a.q.UpdateFinalizeLockCuration(ctx, sqlitestore.UpdateFinalizeLockCurationParams{
		ID:                 p.ID,
		SelectedCommitShas: p.SelectedCommitSHAs,
		TargetBranch:       p.TargetBranch,
		BaseSha:            &baseSHA,
		Mode:               p.Mode,
		CommitMessage:      ptrToNullString(p.CommitMessage),
		LastActivityAt:     p.LastActivityAt,
	}))
}

func (a *sqliteAdapter) TouchFinalizeLock(ctx context.Context, p TouchFinalizeLockParams) error {
	return mapSQLiteErr(a.q.TouchFinalizeLock(ctx, sqlitestore.TouchFinalizeLockParams{
		ID:             p.ID,
		LastActivityAt: p.LastActivityAt,
	}))
}

func (a *sqliteAdapter) ReleaseFinalizeLock(ctx context.Context, p ReleaseFinalizeLockParams) error {
	return mapSQLiteErr(a.q.ReleaseFinalizeLock(ctx, sqlitestore.ReleaseFinalizeLockParams{
		ID:         p.ID,
		ReleasedAt: &p.ReleasedAt,
	}))
}

func (a *sqliteAdapter) SupersedeFinalizeLock(ctx context.Context, p SupersedeFinalizeLockParams) error {
	return mapSQLiteErr(a.q.SupersedeFinalizeLock(ctx, sqlitestore.SupersedeFinalizeLockParams{
		ID:                 p.ID,
		SupersededByLockID: sql.NullString{String: p.SupersededByLockID, Valid: true},
	}))
}

// ---------------------------------------------------------------------------
// FinalizeLockStore (TxStore)
// ---------------------------------------------------------------------------

func (s *sqliteTxStore) InsertFinalizeLock(ctx context.Context, p InsertFinalizeLockParams) error {
	return mapSQLiteErr(s.q.InsertFinalizeLock(ctx, sqliteInsertFinalizeLockParams(p)))
}

func (s *sqliteTxStore) GetFinalizeLockByID(ctx context.Context, id string) (FinalizeLock, error) {
	row, err := s.q.GetFinalizeLockByID(ctx, id)
	if err != nil {
		return FinalizeLock{}, mapSQLiteErr(err)
	}
	return sqliteFinalizeLock(row), nil
}

func (s *sqliteTxStore) GetActiveFinalizeLockForSession(ctx context.Context, sessionID string) (FinalizeLock, error) {
	row, err := s.q.GetActiveFinalizeLockForSession(ctx, sessionID)
	if err != nil {
		return FinalizeLock{}, mapSQLiteErr(err)
	}
	return sqliteFinalizeLock(row), nil
}

func (s *sqliteTxStore) UpdateFinalizeLockCuration(ctx context.Context, p UpdateFinalizeLockCurationParams) error {
	baseSHA := p.BaseSHA
	return mapSQLiteErr(s.q.UpdateFinalizeLockCuration(ctx, sqlitestore.UpdateFinalizeLockCurationParams{
		ID:                 p.ID,
		SelectedCommitShas: p.SelectedCommitSHAs,
		TargetBranch:       p.TargetBranch,
		BaseSha:            &baseSHA,
		Mode:               p.Mode,
		CommitMessage:      ptrToNullString(p.CommitMessage),
		LastActivityAt:     p.LastActivityAt,
	}))
}

func (s *sqliteTxStore) TouchFinalizeLock(ctx context.Context, p TouchFinalizeLockParams) error {
	return mapSQLiteErr(s.q.TouchFinalizeLock(ctx, sqlitestore.TouchFinalizeLockParams{
		ID:             p.ID,
		LastActivityAt: p.LastActivityAt,
	}))
}

func (s *sqliteTxStore) ReleaseFinalizeLock(ctx context.Context, p ReleaseFinalizeLockParams) error {
	return mapSQLiteErr(s.q.ReleaseFinalizeLock(ctx, sqlitestore.ReleaseFinalizeLockParams{
		ID:         p.ID,
		ReleasedAt: &p.ReleasedAt,
	}))
}

func (s *sqliteTxStore) SupersedeFinalizeLock(ctx context.Context, p SupersedeFinalizeLockParams) error {
	return mapSQLiteErr(s.q.SupersedeFinalizeLock(ctx, sqlitestore.SupersedeFinalizeLockParams{
		ID:                 p.ID,
		SupersededByLockID: sql.NullString{String: p.SupersededByLockID, Valid: true},
	}))
}

// sqliteInsertFinalizeLockParams converts domain InsertFinalizeLockParams to sqlitestore.InsertFinalizeLockParams.
func sqliteInsertFinalizeLockParams(p InsertFinalizeLockParams) sqlitestore.InsertFinalizeLockParams {
	baseSHA := p.BaseSHA
	return sqlitestore.InsertFinalizeLockParams{
		ID:                  p.ID,
		OrgID:               p.OrgID,
		SessionID:           p.SessionID,
		AcquiredByAccountID: p.AcquiredByAccountID,
		AcquiredAt:          p.AcquiredAt,
		LastActivityAt:      p.LastActivityAt,
		SelectedCommitShas:  p.SelectedCommitSHAs,
		TargetBranch:        p.TargetBranch,
		BaseSha:             &baseSHA,
		Mode:                p.Mode,
		CommitMessage:       ptrToNullString(p.CommitMessage),
		SupersededByLockID:  ptrToNullString(p.SupersededByLockID),
		ReleasedAt:          p.ReleasedAt,
	}
}

// sqliteFinalizeLock converts a sqlitestore.FinalizeLock to domain FinalizeLock.
func sqliteFinalizeLock(r sqlitestore.FinalizeLock) FinalizeLock {
	baseSHA := ""
	if r.BaseSha != nil {
		baseSHA = *r.BaseSha
	}
	return FinalizeLock{
		ID:                  r.ID,
		OrgID:               r.OrgID,
		SessionID:           r.SessionID,
		AcquiredByAccountID: r.AcquiredByAccountID,
		AcquiredAt:          r.AcquiredAt,
		LastActivityAt:      r.LastActivityAt,
		SelectedCommitSHAs:  r.SelectedCommitShas,
		TargetBranch:        r.TargetBranch,
		BaseSHA:             baseSHA,
		Mode:                r.Mode,
		CommitMessage:       nullStringToPtr(r.CommitMessage),
		SupersededByLockID:  nullStringToPtr(r.SupersededByLockID),
		ReleasedAt:          r.ReleasedAt,
	}
}

// ---------------------------------------------------------------------------
// LeaseStore (outer adapter)
// ---------------------------------------------------------------------------

// IssueLeaseFencingToken is Postgres-only; the SQLite adapter (single-instance,
// NoopManager) never calls this and returns an explicit error if it does.
func (a *sqliteAdapter) IssueLeaseFencingToken(_ context.Context) (int64, error) {
	return 0, fmt.Errorf("store: IssueLeaseFencingToken is not supported on SQLite")
}

func (a *sqliteAdapter) InsertLease(ctx context.Context, p InsertLeaseParams) (Lease, error) {
	row, err := a.q.InsertLease(ctx, sqlitestore.InsertLeaseParams{
		SessionID:    p.SessionID,
		PodID:        p.PodID,
		FencingToken: p.FencingToken,
	})
	if err != nil {
		return Lease{}, mapSQLiteErr(err)
	}
	return sqliteLease(row), nil
}

func (a *sqliteAdapter) MarkLeaseReleased(ctx context.Context, sessionID string) error {
	return mapSQLiteErr(a.q.MarkLeaseReleased(ctx, sessionID))
}

func (a *sqliteAdapter) UpdateLeaseHeartbeat(ctx context.Context, sessionID string) error {
	return mapSQLiteErr(a.q.UpdateLeaseHeartbeat(ctx, sessionID))
}

// DeleteReleasedLeasesOlderThan is Postgres-only; the SQLite adapter returns
// an explicit error if called.
func (a *sqliteAdapter) DeleteReleasedLeasesOlderThan(_ context.Context, _ time.Time) error {
	return fmt.Errorf("store: DeleteReleasedLeasesOlderThan is not supported on SQLite")
}

// ---------------------------------------------------------------------------
// LeaseStore (TxStore)
// ---------------------------------------------------------------------------

func (s *sqliteTxStore) IssueLeaseFencingToken(_ context.Context) (int64, error) {
	return 0, fmt.Errorf("store: IssueLeaseFencingToken is not supported on SQLite")
}

func (s *sqliteTxStore) InsertLease(ctx context.Context, p InsertLeaseParams) (Lease, error) {
	row, err := s.q.InsertLease(ctx, sqlitestore.InsertLeaseParams{
		SessionID:    p.SessionID,
		PodID:        p.PodID,
		FencingToken: p.FencingToken,
	})
	if err != nil {
		return Lease{}, mapSQLiteErr(err)
	}
	return sqliteLease(row), nil
}

func (s *sqliteTxStore) MarkLeaseReleased(ctx context.Context, sessionID string) error {
	return mapSQLiteErr(s.q.MarkLeaseReleased(ctx, sessionID))
}

func (s *sqliteTxStore) UpdateLeaseHeartbeat(ctx context.Context, sessionID string) error {
	return mapSQLiteErr(s.q.UpdateLeaseHeartbeat(ctx, sessionID))
}

func (s *sqliteTxStore) DeleteReleasedLeasesOlderThan(_ context.Context, _ time.Time) error {
	return fmt.Errorf("store: DeleteReleasedLeasesOlderThan is not supported on SQLite")
}

// sqliteLease converts a sqlitestore.Lease to a domain Lease.
func sqliteLease(r sqlitestore.Lease) Lease {
	return Lease{
		SessionID:    r.SessionID,
		PodID:        r.PodID,
		FencingToken: r.FencingToken,
		AcquiredAt:   r.AcquiredAt,
		ReleasedAt:   r.ReleasedAt,
		HeartbeatAt:  r.HeartbeatAt,
	}
}
