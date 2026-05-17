package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"jamsesh/internal/db/pgstore"
)

// compile-time assertion: postgresAdapter satisfies Store.
var _ Store = (*postgresAdapter)(nil)

// NewPostgresAdapter wraps a *pgxpool.Pool and returns it as a Store.
func NewPostgresAdapter(pool *pgxpool.Pool) Store {
	return &postgresAdapter{q: pgstore.New(pool), pool: pool}
}

type postgresAdapter struct {
	q    *pgstore.Queries
	pool *pgxpool.Pool
}

func (a *postgresAdapter) Dialect() string { return "postgres" }

// Close releases all connections in the pool. pgxpool.Pool.Close is void.
func (a *postgresAdapter) Close() error {
	a.pool.Close()
	return nil
}

// ---------------------------------------------------------------------------
// mapPostgresErr normalises dialect-specific errors to store sentinels.
// ---------------------------------------------------------------------------

func mapPostgresErr(err error) error {
	if err == nil {
		return nil
	}
	// pgx.ErrNoRows wraps sql.ErrNoRows, so errors.Is works for both.
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	// Postgres unique-violation SQLSTATE 23505.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return ErrUniqueViolation
		}
	}
	return err
}

// pgTextToPtr converts pgtype.Text to *string for domain types.
func pgTextToPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

// ptrToPgText converts *string to pgtype.Text for query params.
func ptrToPgText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// ---------------------------------------------------------------------------
// Row mappers
// ---------------------------------------------------------------------------

func pgOrg(row pgstore.Org) Org {
	return Org{
		ID:        row.ID,
		Name:      row.Name,
		Slug:      row.Slug,
		CreatedAt: row.CreatedAt,
	}
}

func pgAccount(row pgstore.Account) Account {
	return Account{
		ID:           row.ID,
		Email:        row.Email,
		DisplayName:  row.DisplayName,
		GithubUserID: pgTextToPtr(row.GithubUserID),
		CreatedAt:    row.CreatedAt,
	}
}

func pgOrgMember(row pgstore.OrgMember) OrgMember {
	return OrgMember{
		OrgID:     row.OrgID,
		AccountID: row.AccountID,
		Role:      row.Role,
		CreatedAt: row.CreatedAt,
	}
}

func pgOrgMemberWithAccount(orgID string, row pgstore.ListOrgMembersRow) OrgMemberWithAccount {
	return OrgMemberWithAccount{
		OrgID:            orgID,
		AccountID:        row.ID,
		Email:            row.Email,
		DisplayName:      row.DisplayName,
		GithubUserID:     pgTextToPtr(row.GithubUserID),
		AccountCreatedAt: row.CreatedAt,
		Role:             row.Role,
		CreatedAt:        row.JoinedAt,
	}
}

func pgSession(row pgstore.Session) Session {
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

func pgSessionMember(row pgstore.SessionMember) SessionMember {
	return SessionMember{
		OrgID:     row.OrgID,
		SessionID: row.SessionID,
		AccountID: row.AccountID,
		Role:      row.Role,
		JoinedAt:  row.JoinedAt,
	}
}

func pgSessionMembership(row pgstore.ListSessionMembershipsForAccountRow) SessionMembership {
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

func pgOAuthToken(row pgstore.OauthToken) OAuthToken {
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

func pgMagicLinkToken(row pgstore.MagicLinkToken) MagicLinkToken {
	return MagicLinkToken{
		ID:        row.ID,
		TokenHash: row.TokenHash,
		Email:     row.Email,
		IssuedAt:  row.IssuedAt,
		ExpiresAt: row.ExpiresAt,
		UsedAt:    row.UsedAt,
	}
}

func pgArchivedSession(row pgstore.ArchivedSession) ArchivedSession {
	var ids []string
	_ = json.Unmarshal([]byte(row.MemberAccountIds), &ids)
	if ids == nil {
		ids = []string{}
	}
	// row.EndedAt is *time.Time (global *.ended_at override); NOT NULL in schema.
	var endedAt time.Time
	if row.EndedAt != nil {
		endedAt = *row.EndedAt
	}
	// row.ArchivedAt is pgtype.Timestamptz; extract the Go time.Time.
	var archivedAt time.Time
	if row.ArchivedAt.Valid {
		archivedAt = row.ArchivedAt.Time
	}
	return ArchivedSession{
		SessionID:        row.SessionID,
		OrgID:            row.OrgID,
		Name:             row.Name,
		GoalText:         row.GoalText,
		MemberAccountIDs: ids,
		EndedAt:          endedAt,
		ArchivedAt:       archivedAt,
		EndReason:        row.EndReason,
		FinalBranchName:  pgTextToPtr(row.FinalBranchName),
	}
}

// ---------------------------------------------------------------------------
// OrgStore
// ---------------------------------------------------------------------------

func (a *postgresAdapter) CreateOrg(ctx context.Context, p CreateOrgParams) (Org, error) {
	row, err := a.q.CreateOrg(ctx, pgstore.CreateOrgParams{
		ID:        p.ID,
		Name:      p.Name,
		Slug:      p.Slug,
		CreatedAt: p.CreatedAt,
	})
	if err != nil {
		return Org{}, mapPostgresErr(err)
	}
	return pgOrg(row), nil
}

func (a *postgresAdapter) GetOrgByID(ctx context.Context, id string) (Org, error) {
	row, err := a.q.GetOrgByID(ctx, id)
	if err != nil {
		return Org{}, mapPostgresErr(err)
	}
	return pgOrg(row), nil
}

func (a *postgresAdapter) GetOrgBySlug(ctx context.Context, slug string) (Org, error) {
	row, err := a.q.GetOrgBySlug(ctx, slug)
	if err != nil {
		return Org{}, mapPostgresErr(err)
	}
	return pgOrg(row), nil
}

// ---------------------------------------------------------------------------
// AccountStore
// ---------------------------------------------------------------------------

func (a *postgresAdapter) CreateAccount(ctx context.Context, p CreateAccountParams) (Account, error) {
	row, err := a.q.CreateAccount(ctx, pgstore.CreateAccountParams{
		ID:           p.ID,
		Email:        p.Email,
		DisplayName:  p.DisplayName,
		GithubUserID: ptrToPgText(p.GithubUserID),
		CreatedAt:    p.CreatedAt,
	})
	if err != nil {
		return Account{}, mapPostgresErr(err)
	}
	return pgAccount(row), nil
}

func (a *postgresAdapter) GetAccountByID(ctx context.Context, id string) (Account, error) {
	row, err := a.q.GetAccountByID(ctx, id)
	if err != nil {
		return Account{}, mapPostgresErr(err)
	}
	return pgAccount(row), nil
}

func (a *postgresAdapter) GetAccountByEmail(ctx context.Context, email string) (Account, error) {
	row, err := a.q.GetAccountByEmail(ctx, email)
	if err != nil {
		return Account{}, mapPostgresErr(err)
	}
	return pgAccount(row), nil
}

func (a *postgresAdapter) GetAccountByGitHubUserID(ctx context.Context, githubUserID *string) (Account, error) {
	row, err := a.q.GetAccountByGitHubUserID(ctx, ptrToPgText(githubUserID))
	if err != nil {
		return Account{}, mapPostgresErr(err)
	}
	return pgAccount(row), nil
}

func (a *postgresAdapter) UpdateAccountDisplayName(ctx context.Context, p UpdateAccountDisplayNameParams) error {
	return mapPostgresErr(a.q.UpdateAccountDisplayName(ctx, pgstore.UpdateAccountDisplayNameParams{
		ID:          p.ID,
		DisplayName: p.DisplayName,
	}))
}

// ---------------------------------------------------------------------------
// OrgMemberStore
// ---------------------------------------------------------------------------

func (a *postgresAdapter) AddOrgMember(ctx context.Context, p AddOrgMemberParams) error {
	return mapPostgresErr(a.q.AddOrgMember(ctx, pgstore.AddOrgMemberParams{
		OrgID:     p.OrgID,
		AccountID: p.AccountID,
		Role:      p.Role,
		CreatedAt: p.CreatedAt,
	}))
}

func (a *postgresAdapter) GetOrgMember(ctx context.Context, p GetOrgMemberParams) (OrgMember, error) {
	row, err := a.q.GetOrgMember(ctx, pgstore.GetOrgMemberParams{
		OrgID:     p.OrgID,
		AccountID: p.AccountID,
	})
	if err != nil {
		return OrgMember{}, mapPostgresErr(err)
	}
	return pgOrgMember(row), nil
}

func (a *postgresAdapter) ListOrgsForAccount(ctx context.Context, accountID string) ([]Org, error) {
	rows, err := a.q.ListOrgsForAccount(ctx, accountID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	orgs := make([]Org, len(rows))
	for i, r := range rows {
		orgs[i] = pgOrg(r)
	}
	return orgs, nil
}

func (a *postgresAdapter) ListOrgMembers(ctx context.Context, orgID string) ([]OrgMemberWithAccount, error) {
	rows, err := a.q.ListOrgMembers(ctx, orgID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	members := make([]OrgMemberWithAccount, len(rows))
	for i, r := range rows {
		members[i] = pgOrgMemberWithAccount(orgID, r)
	}
	return members, nil
}

func (a *postgresAdapter) RemoveOrgMember(ctx context.Context, p RemoveOrgMemberParams) error {
	return mapPostgresErr(a.q.RemoveOrgMember(ctx, pgstore.RemoveOrgMemberParams{
		OrgID:     p.OrgID,
		AccountID: p.AccountID,
	}))
}

// ---------------------------------------------------------------------------
// SessionStore
// ---------------------------------------------------------------------------

func (a *postgresAdapter) CreateSession(ctx context.Context, p CreateSessionParams) (Session, error) {
	row, err := a.q.CreateSession(ctx, pgstore.CreateSessionParams{
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
		return Session{}, mapPostgresErr(err)
	}
	return pgSession(row), nil
}

func (a *postgresAdapter) GetSession(ctx context.Context, orgID, id string) (Session, error) {
	row, err := a.q.GetSession(ctx, pgstore.GetSessionParams{
		OrgID: orgID,
		ID:    id,
	})
	if err != nil {
		return Session{}, mapPostgresErr(err)
	}
	return pgSession(row), nil
}

func (a *postgresAdapter) ListSessionsForOrg(ctx context.Context, orgID string) ([]Session, error) {
	rows, err := a.q.ListSessionsForOrg(ctx, orgID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	sessions := make([]Session, len(rows))
	for i, r := range rows {
		sessions[i] = pgSession(r)
	}
	return sessions, nil
}

func (a *postgresAdapter) UpdateSessionStatus(ctx context.Context, p UpdateSessionStatusParams) error {
	return mapPostgresErr(a.q.UpdateSessionStatus(ctx, pgstore.UpdateSessionStatusParams{
		OrgID:  p.OrgID,
		ID:     p.ID,
		Status: p.Status,
	}))
}

func (a *postgresAdapter) SetSessionBaseSHA(ctx context.Context, p SetSessionBaseSHAParams) error {
	return mapPostgresErr(a.q.SetSessionBaseSHA(ctx, pgstore.SetSessionBaseSHAParams{
		OrgID:   p.OrgID,
		ID:      p.ID,
		BaseSha: p.BaseSHA,
	}))
}

func (a *postgresAdapter) DeleteSession(ctx context.Context, p DeleteSessionParams) error {
	return mapPostgresErr(a.q.DeleteSession(ctx, pgstore.DeleteSessionParams{
		OrgID: p.OrgID,
		ID:    p.ID,
	}))
}

// ---------------------------------------------------------------------------
// SessionMemberStore
// ---------------------------------------------------------------------------

func (a *postgresAdapter) AddSessionMember(ctx context.Context, p AddSessionMemberParams) error {
	return mapPostgresErr(a.q.AddSessionMember(ctx, pgstore.AddSessionMemberParams{
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
		AccountID: p.AccountID,
		Role:      p.Role,
		JoinedAt:  p.JoinedAt,
	}))
}

func (a *postgresAdapter) GetSessionMember(ctx context.Context, p GetSessionMemberParams) (SessionMember, error) {
	row, err := a.q.GetSessionMember(ctx, pgstore.GetSessionMemberParams{
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
		AccountID: p.AccountID,
	})
	if err != nil {
		return SessionMember{}, mapPostgresErr(err)
	}
	return pgSessionMember(row), nil
}

func (a *postgresAdapter) ListSessionMembers(ctx context.Context, p ListSessionMembersParams) ([]SessionMember, error) {
	rows, err := a.q.ListSessionMembers(ctx, pgstore.ListSessionMembersParams{
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
	})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	members := make([]SessionMember, len(rows))
	for i, r := range rows {
		members[i] = pgSessionMember(r)
	}
	return members, nil
}

func (a *postgresAdapter) RemoveSessionMember(ctx context.Context, p RemoveSessionMemberParams) error {
	return mapPostgresErr(a.q.RemoveSessionMember(ctx, pgstore.RemoveSessionMemberParams{
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
		AccountID: p.AccountID,
	}))
}

func (a *postgresAdapter) ListSessionMembershipsForAccount(ctx context.Context, accountID string) ([]SessionMembership, error) {
	rows, err := a.q.ListSessionMembershipsForAccount(ctx, accountID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	memberships := make([]SessionMembership, len(rows))
	for i, r := range rows {
		memberships[i] = pgSessionMembership(r)
	}
	return memberships, nil
}

// ---------------------------------------------------------------------------
// OAuthTokenStore
// ---------------------------------------------------------------------------

func (a *postgresAdapter) CreateOAuthToken(ctx context.Context, p CreateOAuthTokenParams) (OAuthToken, error) {
	row, err := a.q.CreateOAuthToken(ctx, pgstore.CreateOAuthTokenParams{
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
		return OAuthToken{}, mapPostgresErr(err)
	}
	return pgOAuthToken(row), nil
}

func (a *postgresAdapter) GetOAuthTokenByHash(ctx context.Context, tokenHash string) (OAuthToken, error) {
	row, err := a.q.GetOAuthTokenByHash(ctx, tokenHash)
	if err != nil {
		return OAuthToken{}, mapPostgresErr(err)
	}
	return pgOAuthToken(row), nil
}

func (a *postgresAdapter) TouchOAuthTokenLastUsed(ctx context.Context, p TouchOAuthTokenLastUsedParams) error {
	return mapPostgresErr(a.q.TouchOAuthTokenLastUsed(ctx, pgstore.TouchOAuthTokenLastUsedParams{
		ID:         p.ID,
		LastUsedAt: p.LastUsedAt,
	}))
}

func (a *postgresAdapter) RevokeOAuthToken(ctx context.Context, p RevokeOAuthTokenParams) error {
	return mapPostgresErr(a.q.RevokeOAuthToken(ctx, pgstore.RevokeOAuthTokenParams{
		ID:        p.ID,
		RevokedAt: p.RevokedAt,
	}))
}

func (a *postgresAdapter) RevokeAllOAuthTokensForAccount(ctx context.Context, p RevokeAllOAuthTokensForAccountParams) error {
	return mapPostgresErr(a.q.RevokeAllOAuthTokensForAccount(ctx, pgstore.RevokeAllOAuthTokensForAccountParams{
		AccountID: p.AccountID,
		RevokedAt: p.RevokedAt,
	}))
}

func (a *postgresAdapter) ListOAuthTokensForAccount(ctx context.Context, accountID string) ([]OAuthToken, error) {
	rows, err := a.q.ListOAuthTokensForAccount(ctx, accountID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	tokens := make([]OAuthToken, len(rows))
	for i, r := range rows {
		tokens[i] = pgOAuthToken(r)
	}
	return tokens, nil
}

// ---------------------------------------------------------------------------
// MagicLinkTokenStore
// ---------------------------------------------------------------------------

func (a *postgresAdapter) CreateMagicLinkToken(ctx context.Context, p CreateMagicLinkTokenParams) (MagicLinkToken, error) {
	row, err := a.q.CreateMagicLinkToken(ctx, pgstore.CreateMagicLinkTokenParams{
		ID:        p.ID,
		TokenHash: p.TokenHash,
		Email:     p.Email,
		IssuedAt:  p.IssuedAt,
		ExpiresAt: p.ExpiresAt,
		UsedAt:    p.UsedAt,
	})
	if err != nil {
		return MagicLinkToken{}, mapPostgresErr(err)
	}
	return pgMagicLinkToken(row), nil
}

func (a *postgresAdapter) GetMagicLinkTokenByHash(ctx context.Context, tokenHash string) (MagicLinkToken, error) {
	row, err := a.q.GetMagicLinkTokenByHash(ctx, tokenHash)
	if err != nil {
		return MagicLinkToken{}, mapPostgresErr(err)
	}
	return pgMagicLinkToken(row), nil
}

func (a *postgresAdapter) ConsumeMagicLinkToken(ctx context.Context, p ConsumeMagicLinkTokenParams) error {
	return mapPostgresErr(a.q.ConsumeMagicLinkToken(ctx, pgstore.ConsumeMagicLinkTokenParams{
		ID:     p.ID,
		UsedAt: p.UsedAt,
	}))
}

// ---------------------------------------------------------------------------
// ArchivedSessionStore
// ---------------------------------------------------------------------------

func (a *postgresAdapter) InsertArchivedSession(ctx context.Context, p InsertArchivedSessionParams) error {
	endedAt := p.EndedAt // time.Time → *time.Time for sqlc-generated param
	return mapPostgresErr(a.q.InsertArchivedSession(ctx, pgstore.InsertArchivedSessionParams{
		SessionID:        p.SessionID,
		OrgID:            p.OrgID,
		Name:             p.Name,
		GoalText:         p.GoalText,
		MemberAccountIds: p.MemberAccountIDs,
		EndedAt:          &endedAt,
		ArchivedAt:       pgtype.Timestamptz{Time: p.ArchivedAt, Valid: true},
		EndReason:        p.EndReason,
		FinalBranchName:  ptrToPgText(p.FinalBranchName),
	}))
}

func (a *postgresAdapter) GetArchivedSession(ctx context.Context, p GetArchivedSessionParams) (ArchivedSession, error) {
	row, err := a.q.GetArchivedSession(ctx, pgstore.GetArchivedSessionParams{
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
	})
	if err != nil {
		return ArchivedSession{}, mapPostgresErr(err)
	}
	return pgArchivedSession(row), nil
}
