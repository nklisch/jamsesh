package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

// ---------------------------------------------------------------------------
// Row mappers
// ---------------------------------------------------------------------------

func sqliteOrg(row sqlitestore.Org) Org {
	return Org{
		ID:        row.ID,
		Name:      row.Name,
		Slug:      row.Slug,
		CreatedAt: row.CreatedAt,
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
		ID:            row.ID,
		OrgID:         row.OrgID,
		Name:          row.Name,
		Goal:          row.Goal,
		WritableScope: row.WritableScope,
		DefaultMode:   row.DefaultMode,
		BaseSHA:       row.BaseSha,
		Status:        row.Status,
		CreatedAt:     row.CreatedAt,
		EndedAt:       row.EndedAt,
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
