package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// Ping verifies the postgres connection pool is alive.
func (a *postgresAdapter) Ping(ctx context.Context) error {
	return a.pool.Ping(ctx)
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

// pgTimestamptzToPtr converts pgtype.Timestamptz to *time.Time for domain types.
func pgTimestamptzToPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

// ptrToPgTimestamptz converts *time.Time to pgtype.Timestamptz for query params.
func ptrToPgTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
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
		EndReason:                 pgTextToPtr(row.EndReason),
		FinalizeLockedByAccountID: pgTextToPtr(row.FinalizeLockedByAccountID),
	}
}

func pgRefMode(row pgstore.RefMode) RefMode {
	return RefMode{
		SessionID: row.SessionID,
		Ref:       row.Ref,
		Mode:      row.Mode,
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

func (a *postgresAdapter) ListSessionsForOrgWithCursor(ctx context.Context, p ListSessionsForOrgWithCursorParams) ([]Session, error) {
	rows, err := a.q.ListSessionsForOrgWithCursor(ctx, pgstore.ListSessionsForOrgWithCursorParams{
		OrgID:     p.OrgID,
		CreatedAt: p.Before,
		Limit:     int32(p.Limit),
	})
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

func (a *postgresAdapter) UpdateSessionGoalScopeMode(ctx context.Context, p UpdateSessionGoalScopeModeParams) error {
	return mapPostgresErr(a.q.UpdateSessionGoalScopeMode(ctx, pgstore.UpdateSessionGoalScopeModeParams{
		OrgID:         p.OrgID,
		ID:            p.ID,
		Goal:          p.Goal,
		WritableScope: p.WritableScope,
		DefaultMode:   p.DefaultMode,
	}))
}

func (a *postgresAdapter) SetSessionEndReason(ctx context.Context, p SetSessionEndReasonParams) error {
	return mapPostgresErr(a.q.SetSessionEndReason(ctx, pgstore.SetSessionEndReasonParams{
		OrgID:     p.OrgID,
		ID:        p.ID,
		EndReason: ptrToPgText(p.EndReason),
		EndedAt:   p.EndedAt,
	}))
}

func (a *postgresAdapter) SetFinalizeLock(ctx context.Context, p SetFinalizeLockParams) error {
	return mapPostgresErr(a.q.SetFinalizeLock(ctx, pgstore.SetFinalizeLockParams{
		OrgID:                     p.OrgID,
		ID:                        p.ID,
		FinalizeLockedByAccountID: ptrToPgText(p.AccountID),
	}))
}

func (a *postgresAdapter) ClearFinalizeLock(ctx context.Context, p ClearFinalizeLockParams) error {
	return mapPostgresErr(a.q.ClearFinalizeLock(ctx, pgstore.ClearFinalizeLockParams{
		OrgID: p.OrgID,
		ID:    p.ID,
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

// ---------------------------------------------------------------------------
// OAuthStateStore
// ---------------------------------------------------------------------------

func (a *postgresAdapter) InsertOAuthState(ctx context.Context, p InsertOAuthStateParams) error {
	return mapPostgresErr(a.q.InsertOAuthState(ctx, pgstore.InsertOAuthStateParams{
		Nonce:       p.Nonce,
		Provider:    p.Provider,
		RedirectUri: p.RedirectURI,
		CreatedAt:   p.CreatedAt,
		ExpiresAt:   p.ExpiresAt,
	}))
}

func (a *postgresAdapter) ConsumeOAuthState(ctx context.Context, nonce string) (OAuthState, error) {
	row, err := a.q.ConsumeOAuthState(ctx, nonce)
	if err != nil {
		return OAuthState{}, mapPostgresErr(err)
	}
	return OAuthState{
		Nonce:       row.Nonce,
		Provider:    row.Provider,
		RedirectURI: row.RedirectUri,
		CreatedAt:   row.CreatedAt,
		ExpiresAt:   row.ExpiresAt,
	}, nil
}

func (a *postgresAdapter) CleanupExpiredOAuthState(ctx context.Context, before time.Time) error {
	return mapPostgresErr(a.q.CleanupExpiredOAuthState(ctx, before))
}

// ---------------------------------------------------------------------------
// EventLogStore
// ---------------------------------------------------------------------------

func (a *postgresAdapter) EnsureEventSeqRow(ctx context.Context, sessionID string) error {
	return mapPostgresErr(a.q.EnsureEventSeqRow(ctx, sessionID))
}

func (a *postgresAdapter) AllocateNextSeq(ctx context.Context, sessionID string) (int64, error) {
	seq, err := a.q.AllocateNextSeq(ctx, sessionID)
	return int64(seq), mapPostgresErr(err)
}

func (a *postgresAdapter) AllocateNextSeqN(ctx context.Context, sessionID string, n int64) (int64, error) {
	seq, err := a.q.AllocateNextSeqN(ctx, pgstore.AllocateNextSeqNParams{
		Next:      int32(n),
		SessionID: sessionID,
	})
	return int64(seq), mapPostgresErr(err)
}

func (a *postgresAdapter) InsertEvent(ctx context.Context, p InsertEventParams) error {
	return mapPostgresErr(a.q.InsertEvent(ctx, pgstore.InsertEventParams{
		ID:        p.ID,
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
		Seq:       int32(p.Seq),
		Type:      p.Type,
		Payload:   p.Payload,
		CreatedAt: p.CreatedAt,
	}))
}

func (a *postgresAdapter) ListEventsSince(ctx context.Context, p ListEventsSinceParams) ([]Event, error) {
	rows, err := a.q.ListEventsSince(ctx, pgstore.ListEventsSinceParams{
		SessionID: p.SessionID,
		Seq:       int32(p.SinceSeq),
		Limit:     int32(p.Limit),
	})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{
			ID:        r.ID,
			OrgID:     r.OrgID,
			SessionID: r.SessionID,
			Seq:       int64(r.Seq),
			Type:      r.Type,
			Payload:   r.Payload,
			CreatedAt: r.CreatedAt,
		}
	}
	return events, nil
}

func (a *postgresAdapter) ListEventsSinceForDigest(ctx context.Context, p ListEventsSinceForDigestParams) ([]Event, error) {
	rows, err := a.q.ListEventsSinceForDigest(ctx, pgstore.ListEventsSinceForDigestParams{
		SessionID: p.SessionID,
		Seq:       int32(p.SinceSeq),
		Limit:     int32(p.Limit),
	})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{
			ID:        r.ID,
			OrgID:     r.OrgID,
			SessionID: r.SessionID,
			Seq:       int64(r.Seq),
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

func (a *postgresAdapter) UpsertPresence(ctx context.Context, p UpsertPresenceParams) error {
	return mapPostgresErr(a.q.UpsertPresence(ctx, pgstore.UpsertPresenceParams{
		OrgID:        p.OrgID,
		SessionID:    p.SessionID,
		AccountID:    p.AccountID,
		Ref:          p.Ref,
		CurrentSha:   p.CurrentSHA,
		LastActiveAt: pgtype.Timestamptz{Time: p.LastActiveAt, Valid: true},
	}))
}

func (a *postgresAdapter) ListPresenceForSession(ctx context.Context, sessionID string) ([]PresenceRow, error) {
	rows, err := a.q.ListPresenceForSession(ctx, sessionID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	out := make([]PresenceRow, len(rows))
	for i, r := range rows {
		var lastActiveAt time.Time
		if r.LastActiveAt.Valid {
			lastActiveAt = r.LastActiveAt.Time
		}
		out[i] = PresenceRow{
			OrgID:        r.OrgID,
			SessionID:    r.SessionID,
			AccountID:    r.AccountID,
			Ref:          r.Ref,
			CurrentSHA:   r.CurrentSha,
			LastActiveAt: lastActiveAt,
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// RefModeStore
// ---------------------------------------------------------------------------

func (a *postgresAdapter) UpsertRefMode(ctx context.Context, p UpsertRefModeParams) error {
	return mapPostgresErr(a.q.UpsertRefMode(ctx, pgstore.UpsertRefModeParams{
		SessionID: p.SessionID,
		Ref:       p.Ref,
		Mode:      p.Mode,
	}))
}

func (a *postgresAdapter) GetRefMode(ctx context.Context, p GetRefModeParams) (RefMode, error) {
	row, err := a.q.GetRefMode(ctx, pgstore.GetRefModeParams{
		SessionID: p.SessionID,
		Ref:       p.Ref,
	})
	if err != nil {
		return RefMode{}, mapPostgresErr(err)
	}
	return pgRefMode(row), nil
}

func (a *postgresAdapter) ListRefModesForSession(ctx context.Context, sessionID string) ([]RefMode, error) {
	rows, err := a.q.ListRefModesForSession(ctx, sessionID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	out := make([]RefMode, len(rows))
	for i, r := range rows {
		out[i] = pgRefMode(r)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// WithTx
// ---------------------------------------------------------------------------

// postgresTxStore wraps a *pgstore.Queries scoped to a transaction and
// satisfies TxStore.
type postgresTxStore struct {
	q *pgstore.Queries
}

var _ TxStore = (*postgresTxStore)(nil)

func (a *postgresAdapter) WithTx(ctx context.Context, fn func(TxStore) error) error {
	tx, err := a.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}
	txq := pgstore.New(tx)
	ts := &postgresTxStore{q: txq}
	if err := fn(ts); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit tx: %w", mapPostgresErr(err))
	}
	return nil
}

// ---------------------------------------------------------------------------
// OrgInviteStore
// ---------------------------------------------------------------------------

func pgOrgInvite(row pgstore.OrgInvite) OrgInvite {
	return OrgInvite{
		ID:                  row.ID,
		OrgID:               row.OrgID,
		InviterAccountID:    row.InviterAccountID,
		RecipientEmail:      row.RecipientEmail,
		TokenHash:           row.TokenHash,
		CreatedAt:           row.CreatedAt,
		ExpiresAt:           row.ExpiresAt,
		AcceptedAt:          row.AcceptedAt,
		AcceptedByAccountID: pgTextToPtr(row.AcceptedByAccountID),
	}
}

func (a *postgresAdapter) InsertOrgInvite(ctx context.Context, p InsertOrgInviteParams) (OrgInvite, error) {
	row, err := a.q.InsertOrgInvite(ctx, pgstore.InsertOrgInviteParams{
		ID:                  p.ID,
		OrgID:               p.OrgID,
		InviterAccountID:    p.InviterAccountID,
		RecipientEmail:      p.RecipientEmail,
		TokenHash:           p.TokenHash,
		CreatedAt:           p.CreatedAt,
		ExpiresAt:           p.ExpiresAt,
		AcceptedAt:          p.AcceptedAt,
		AcceptedByAccountID: ptrToPgText(p.AcceptedByAccountID),
	})
	if err != nil {
		return OrgInvite{}, mapPostgresErr(err)
	}
	return pgOrgInvite(row), nil
}

func (a *postgresAdapter) GetOrgInviteByID(ctx context.Context, id string) (OrgInvite, error) {
	row, err := a.q.GetOrgInviteByID(ctx, id)
	if err != nil {
		return OrgInvite{}, mapPostgresErr(err)
	}
	return pgOrgInvite(row), nil
}

func (a *postgresAdapter) GetOrgInviteByTokenHash(ctx context.Context, tokenHash string) (OrgInvite, error) {
	row, err := a.q.GetOrgInviteByTokenHash(ctx, tokenHash)
	if err != nil {
		return OrgInvite{}, mapPostgresErr(err)
	}
	return pgOrgInvite(row), nil
}

func (a *postgresAdapter) MarkOrgInviteAccepted(ctx context.Context, p MarkOrgInviteAcceptedParams) error {
	return mapPostgresErr(a.q.MarkOrgInviteAccepted(ctx, pgstore.MarkOrgInviteAcceptedParams{
		ID:                  p.ID,
		AcceptedAt:          &p.AcceptedAt,
		AcceptedByAccountID: ptrToPgText(&p.AcceptedByAccountID),
	}))
}

func (a *postgresAdapter) ListPendingOrgInvitesForOrg(ctx context.Context, p ListPendingOrgInvitesForOrgParams) ([]OrgInvite, error) {
	rows, err := a.q.ListPendingOrgInvitesForOrg(ctx, pgstore.ListPendingOrgInvitesForOrgParams{
		OrgID:     p.OrgID,
		ExpiresAt: p.Now,
	})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	invites := make([]OrgInvite, len(rows))
	for i, r := range rows {
		invites[i] = pgOrgInvite(r)
	}
	return invites, nil
}

func (a *postgresAdapter) ListPendingOrgInvitesForEmail(ctx context.Context, p ListPendingOrgInvitesForEmailParams) ([]OrgInvite, error) {
	rows, err := a.q.ListPendingOrgInvitesForEmail(ctx, pgstore.ListPendingOrgInvitesForEmailParams{
		RecipientEmail: p.Email,
		ExpiresAt:      p.Now,
	})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	invites := make([]OrgInvite, len(rows))
	for i, r := range rows {
		invites[i] = pgOrgInvite(r)
	}
	return invites, nil
}

// ---------------------------------------------------------------------------
// SessionInviteStore
// ---------------------------------------------------------------------------

func pgSessionInvite(row pgstore.SessionInvite) SessionInvite {
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
		AcceptedByAccountID: pgTextToPtr(row.AcceptedByAccountID),
	}
}

func (a *postgresAdapter) InsertSessionInvite(ctx context.Context, p InsertSessionInviteParams) (SessionInvite, error) {
	row, err := a.q.InsertSessionInvite(ctx, pgstore.InsertSessionInviteParams{
		ID:                  p.ID,
		OrgID:               p.OrgID,
		SessionID:           p.SessionID,
		InviterAccountID:    p.InviterAccountID,
		InviteeEmail:        p.InviteeEmail,
		TokenHash:           p.TokenHash,
		CreatedAt:           p.CreatedAt,
		ExpiresAt:           p.ExpiresAt,
		AcceptedAt:          p.AcceptedAt,
		AcceptedByAccountID: ptrToPgText(p.AcceptedByAccountID),
	})
	if err != nil {
		return SessionInvite{}, mapPostgresErr(err)
	}
	return pgSessionInvite(row), nil
}

func (a *postgresAdapter) GetSessionInviteByID(ctx context.Context, id string) (SessionInvite, error) {
	row, err := a.q.GetSessionInviteByID(ctx, id)
	if err != nil {
		return SessionInvite{}, mapPostgresErr(err)
	}
	return pgSessionInvite(row), nil
}

func (a *postgresAdapter) GetSessionInviteByTokenHash(ctx context.Context, tokenHash string) (SessionInvite, error) {
	row, err := a.q.GetSessionInviteByTokenHash(ctx, tokenHash)
	if err != nil {
		return SessionInvite{}, mapPostgresErr(err)
	}
	return pgSessionInvite(row), nil
}

func (a *postgresAdapter) MarkSessionInviteAccepted(ctx context.Context, p MarkSessionInviteAcceptedParams) error {
	return mapPostgresErr(a.q.MarkSessionInviteAccepted(ctx, pgstore.MarkSessionInviteAcceptedParams{
		ID:                  p.ID,
		AcceptedAt:          &p.AcceptedAt,
		AcceptedByAccountID: ptrToPgText(&p.AcceptedByAccountID),
	}))
}

func (a *postgresAdapter) ListPendingSessionInvitesForSession(ctx context.Context, p ListPendingSessionInvitesForSessionParams) ([]SessionInvite, error) {
	rows, err := a.q.ListPendingSessionInvitesForSession(ctx, pgstore.ListPendingSessionInvitesForSessionParams{
		SessionID: p.SessionID,
		ExpiresAt: p.Now,
	})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	invites := make([]SessionInvite, len(rows))
	for i, r := range rows {
		invites[i] = pgSessionInvite(r)
	}
	return invites, nil
}

// Delegate all TxStore methods to the underlying *pgstore.Queries.
// OrgStore
func (s *postgresTxStore) CreateOrg(ctx context.Context, p CreateOrgParams) (Org, error) {
	row, err := s.q.CreateOrg(ctx, pgstore.CreateOrgParams{ID: p.ID, Name: p.Name, Slug: p.Slug, CreatedAt: p.CreatedAt})
	if err != nil {
		return Org{}, mapPostgresErr(err)
	}
	return pgOrg(row), nil
}
func (s *postgresTxStore) GetOrgByID(ctx context.Context, id string) (Org, error) {
	row, err := s.q.GetOrgByID(ctx, id)
	if err != nil {
		return Org{}, mapPostgresErr(err)
	}
	return pgOrg(row), nil
}
func (s *postgresTxStore) GetOrgBySlug(ctx context.Context, slug string) (Org, error) {
	row, err := s.q.GetOrgBySlug(ctx, slug)
	if err != nil {
		return Org{}, mapPostgresErr(err)
	}
	return pgOrg(row), nil
}

// AccountStore
func (s *postgresTxStore) CreateAccount(ctx context.Context, p CreateAccountParams) (Account, error) {
	row, err := s.q.CreateAccount(ctx, pgstore.CreateAccountParams{ID: p.ID, Email: p.Email, DisplayName: p.DisplayName, GithubUserID: ptrToPgText(p.GithubUserID), CreatedAt: p.CreatedAt})
	if err != nil {
		return Account{}, mapPostgresErr(err)
	}
	return pgAccount(row), nil
}
func (s *postgresTxStore) GetAccountByID(ctx context.Context, id string) (Account, error) {
	row, err := s.q.GetAccountByID(ctx, id)
	if err != nil {
		return Account{}, mapPostgresErr(err)
	}
	return pgAccount(row), nil
}
func (s *postgresTxStore) GetAccountByEmail(ctx context.Context, email string) (Account, error) {
	row, err := s.q.GetAccountByEmail(ctx, email)
	if err != nil {
		return Account{}, mapPostgresErr(err)
	}
	return pgAccount(row), nil
}
func (s *postgresTxStore) GetAccountByGitHubUserID(ctx context.Context, githubUserID *string) (Account, error) {
	row, err := s.q.GetAccountByGitHubUserID(ctx, ptrToPgText(githubUserID))
	if err != nil {
		return Account{}, mapPostgresErr(err)
	}
	return pgAccount(row), nil
}
func (s *postgresTxStore) UpdateAccountDisplayName(ctx context.Context, p UpdateAccountDisplayNameParams) error {
	return mapPostgresErr(s.q.UpdateAccountDisplayName(ctx, pgstore.UpdateAccountDisplayNameParams{ID: p.ID, DisplayName: p.DisplayName}))
}

// OrgMemberStore
func (s *postgresTxStore) AddOrgMember(ctx context.Context, p AddOrgMemberParams) error {
	return mapPostgresErr(s.q.AddOrgMember(ctx, pgstore.AddOrgMemberParams{OrgID: p.OrgID, AccountID: p.AccountID, Role: p.Role, CreatedAt: p.CreatedAt}))
}
func (s *postgresTxStore) GetOrgMember(ctx context.Context, p GetOrgMemberParams) (OrgMember, error) {
	row, err := s.q.GetOrgMember(ctx, pgstore.GetOrgMemberParams{OrgID: p.OrgID, AccountID: p.AccountID})
	if err != nil {
		return OrgMember{}, mapPostgresErr(err)
	}
	return pgOrgMember(row), nil
}
func (s *postgresTxStore) ListOrgsForAccount(ctx context.Context, accountID string) ([]Org, error) {
	rows, err := s.q.ListOrgsForAccount(ctx, accountID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	orgs := make([]Org, len(rows))
	for i, r := range rows {
		orgs[i] = pgOrg(r)
	}
	return orgs, nil
}
func (s *postgresTxStore) ListOrgMembers(ctx context.Context, orgID string) ([]OrgMemberWithAccount, error) {
	rows, err := s.q.ListOrgMembers(ctx, orgID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	members := make([]OrgMemberWithAccount, len(rows))
	for i, r := range rows {
		members[i] = pgOrgMemberWithAccount(orgID, r)
	}
	return members, nil
}
func (s *postgresTxStore) RemoveOrgMember(ctx context.Context, p RemoveOrgMemberParams) error {
	return mapPostgresErr(s.q.RemoveOrgMember(ctx, pgstore.RemoveOrgMemberParams{OrgID: p.OrgID, AccountID: p.AccountID}))
}

// SessionStore
func (s *postgresTxStore) CreateSession(ctx context.Context, p CreateSessionParams) (Session, error) {
	row, err := s.q.CreateSession(ctx, pgstore.CreateSessionParams{ID: p.ID, OrgID: p.OrgID, Name: p.Name, Goal: p.Goal, WritableScope: p.WritableScope, DefaultMode: p.DefaultMode, BaseSha: p.BaseSHA, Status: p.Status, CreatedAt: p.CreatedAt, EndedAt: p.EndedAt})
	if err != nil {
		return Session{}, mapPostgresErr(err)
	}
	return pgSession(row), nil
}
func (s *postgresTxStore) GetSession(ctx context.Context, orgID, id string) (Session, error) {
	row, err := s.q.GetSession(ctx, pgstore.GetSessionParams{OrgID: orgID, ID: id})
	if err != nil {
		return Session{}, mapPostgresErr(err)
	}
	return pgSession(row), nil
}
func (s *postgresTxStore) ListSessionsForOrg(ctx context.Context, orgID string) ([]Session, error) {
	rows, err := s.q.ListSessionsForOrg(ctx, orgID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	sessions := make([]Session, len(rows))
	for i, r := range rows {
		sessions[i] = pgSession(r)
	}
	return sessions, nil
}
func (s *postgresTxStore) ListSessionsForOrgWithCursor(ctx context.Context, p ListSessionsForOrgWithCursorParams) ([]Session, error) {
	rows, err := s.q.ListSessionsForOrgWithCursor(ctx, pgstore.ListSessionsForOrgWithCursorParams{OrgID: p.OrgID, CreatedAt: p.Before, Limit: int32(p.Limit)})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	sessions := make([]Session, len(rows))
	for i, r := range rows {
		sessions[i] = pgSession(r)
	}
	return sessions, nil
}
func (s *postgresTxStore) UpdateSessionStatus(ctx context.Context, p UpdateSessionStatusParams) error {
	return mapPostgresErr(s.q.UpdateSessionStatus(ctx, pgstore.UpdateSessionStatusParams{OrgID: p.OrgID, ID: p.ID, Status: p.Status}))
}
func (s *postgresTxStore) SetSessionBaseSHA(ctx context.Context, p SetSessionBaseSHAParams) error {
	return mapPostgresErr(s.q.SetSessionBaseSHA(ctx, pgstore.SetSessionBaseSHAParams{OrgID: p.OrgID, ID: p.ID, BaseSha: p.BaseSHA}))
}
func (s *postgresTxStore) UpdateSessionGoalScopeMode(ctx context.Context, p UpdateSessionGoalScopeModeParams) error {
	return mapPostgresErr(s.q.UpdateSessionGoalScopeMode(ctx, pgstore.UpdateSessionGoalScopeModeParams{OrgID: p.OrgID, ID: p.ID, Goal: p.Goal, WritableScope: p.WritableScope, DefaultMode: p.DefaultMode}))
}
func (s *postgresTxStore) SetSessionEndReason(ctx context.Context, p SetSessionEndReasonParams) error {
	return mapPostgresErr(s.q.SetSessionEndReason(ctx, pgstore.SetSessionEndReasonParams{OrgID: p.OrgID, ID: p.ID, EndReason: ptrToPgText(p.EndReason), EndedAt: p.EndedAt}))
}
func (s *postgresTxStore) SetFinalizeLock(ctx context.Context, p SetFinalizeLockParams) error {
	return mapPostgresErr(s.q.SetFinalizeLock(ctx, pgstore.SetFinalizeLockParams{OrgID: p.OrgID, ID: p.ID, FinalizeLockedByAccountID: ptrToPgText(p.AccountID)}))
}
func (s *postgresTxStore) ClearFinalizeLock(ctx context.Context, p ClearFinalizeLockParams) error {
	return mapPostgresErr(s.q.ClearFinalizeLock(ctx, pgstore.ClearFinalizeLockParams{OrgID: p.OrgID, ID: p.ID}))
}
func (s *postgresTxStore) DeleteSession(ctx context.Context, p DeleteSessionParams) error {
	return mapPostgresErr(s.q.DeleteSession(ctx, pgstore.DeleteSessionParams{OrgID: p.OrgID, ID: p.ID}))
}

// SessionMemberStore
func (s *postgresTxStore) AddSessionMember(ctx context.Context, p AddSessionMemberParams) error {
	return mapPostgresErr(s.q.AddSessionMember(ctx, pgstore.AddSessionMemberParams{OrgID: p.OrgID, SessionID: p.SessionID, AccountID: p.AccountID, Role: p.Role, JoinedAt: p.JoinedAt}))
}
func (s *postgresTxStore) GetSessionMember(ctx context.Context, p GetSessionMemberParams) (SessionMember, error) {
	row, err := s.q.GetSessionMember(ctx, pgstore.GetSessionMemberParams{OrgID: p.OrgID, SessionID: p.SessionID, AccountID: p.AccountID})
	if err != nil {
		return SessionMember{}, mapPostgresErr(err)
	}
	return pgSessionMember(row), nil
}
func (s *postgresTxStore) ListSessionMembers(ctx context.Context, p ListSessionMembersParams) ([]SessionMember, error) {
	rows, err := s.q.ListSessionMembers(ctx, pgstore.ListSessionMembersParams{OrgID: p.OrgID, SessionID: p.SessionID})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	members := make([]SessionMember, len(rows))
	for i, r := range rows {
		members[i] = pgSessionMember(r)
	}
	return members, nil
}
func (s *postgresTxStore) RemoveSessionMember(ctx context.Context, p RemoveSessionMemberParams) error {
	return mapPostgresErr(s.q.RemoveSessionMember(ctx, pgstore.RemoveSessionMemberParams{OrgID: p.OrgID, SessionID: p.SessionID, AccountID: p.AccountID}))
}
func (s *postgresTxStore) ListSessionMembershipsForAccount(ctx context.Context, accountID string) ([]SessionMembership, error) {
	rows, err := s.q.ListSessionMembershipsForAccount(ctx, accountID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	memberships := make([]SessionMembership, len(rows))
	for i, r := range rows {
		memberships[i] = pgSessionMembership(r)
	}
	return memberships, nil
}

// OAuthTokenStore
func (s *postgresTxStore) CreateOAuthToken(ctx context.Context, p CreateOAuthTokenParams) (OAuthToken, error) {
	row, err := s.q.CreateOAuthToken(ctx, pgstore.CreateOAuthTokenParams{ID: p.ID, AccountID: p.AccountID, TokenHash: p.TokenHash, Kind: p.Kind, IssuedAt: p.IssuedAt, ExpiresAt: p.ExpiresAt, LastUsedAt: p.LastUsedAt, RevokedAt: p.RevokedAt})
	if err != nil {
		return OAuthToken{}, mapPostgresErr(err)
	}
	return pgOAuthToken(row), nil
}
func (s *postgresTxStore) GetOAuthTokenByHash(ctx context.Context, tokenHash string) (OAuthToken, error) {
	row, err := s.q.GetOAuthTokenByHash(ctx, tokenHash)
	if err != nil {
		return OAuthToken{}, mapPostgresErr(err)
	}
	return pgOAuthToken(row), nil
}
func (s *postgresTxStore) TouchOAuthTokenLastUsed(ctx context.Context, p TouchOAuthTokenLastUsedParams) error {
	return mapPostgresErr(s.q.TouchOAuthTokenLastUsed(ctx, pgstore.TouchOAuthTokenLastUsedParams{ID: p.ID, LastUsedAt: p.LastUsedAt}))
}
func (s *postgresTxStore) RevokeOAuthToken(ctx context.Context, p RevokeOAuthTokenParams) error {
	return mapPostgresErr(s.q.RevokeOAuthToken(ctx, pgstore.RevokeOAuthTokenParams{ID: p.ID, RevokedAt: p.RevokedAt}))
}
func (s *postgresTxStore) RevokeAllOAuthTokensForAccount(ctx context.Context, p RevokeAllOAuthTokensForAccountParams) error {
	return mapPostgresErr(s.q.RevokeAllOAuthTokensForAccount(ctx, pgstore.RevokeAllOAuthTokensForAccountParams{AccountID: p.AccountID, RevokedAt: p.RevokedAt}))
}
func (s *postgresTxStore) ListOAuthTokensForAccount(ctx context.Context, accountID string) ([]OAuthToken, error) {
	rows, err := s.q.ListOAuthTokensForAccount(ctx, accountID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	tokens := make([]OAuthToken, len(rows))
	for i, r := range rows {
		tokens[i] = pgOAuthToken(r)
	}
	return tokens, nil
}

// MagicLinkTokenStore
func (s *postgresTxStore) CreateMagicLinkToken(ctx context.Context, p CreateMagicLinkTokenParams) (MagicLinkToken, error) {
	row, err := s.q.CreateMagicLinkToken(ctx, pgstore.CreateMagicLinkTokenParams{ID: p.ID, TokenHash: p.TokenHash, Email: p.Email, IssuedAt: p.IssuedAt, ExpiresAt: p.ExpiresAt, UsedAt: p.UsedAt})
	if err != nil {
		return MagicLinkToken{}, mapPostgresErr(err)
	}
	return pgMagicLinkToken(row), nil
}
func (s *postgresTxStore) GetMagicLinkTokenByHash(ctx context.Context, tokenHash string) (MagicLinkToken, error) {
	row, err := s.q.GetMagicLinkTokenByHash(ctx, tokenHash)
	if err != nil {
		return MagicLinkToken{}, mapPostgresErr(err)
	}
	return pgMagicLinkToken(row), nil
}
func (s *postgresTxStore) ConsumeMagicLinkToken(ctx context.Context, p ConsumeMagicLinkTokenParams) error {
	return mapPostgresErr(s.q.ConsumeMagicLinkToken(ctx, pgstore.ConsumeMagicLinkTokenParams{ID: p.ID, UsedAt: p.UsedAt}))
}

// ArchivedSessionStore
func (s *postgresTxStore) InsertArchivedSession(ctx context.Context, p InsertArchivedSessionParams) error {
	endedAt := p.EndedAt
	return mapPostgresErr(s.q.InsertArchivedSession(ctx, pgstore.InsertArchivedSessionParams{SessionID: p.SessionID, OrgID: p.OrgID, Name: p.Name, GoalText: p.GoalText, MemberAccountIds: p.MemberAccountIDs, EndedAt: &endedAt, ArchivedAt: pgtype.Timestamptz{Time: p.ArchivedAt, Valid: true}, EndReason: p.EndReason, FinalBranchName: ptrToPgText(p.FinalBranchName)}))
}
func (s *postgresTxStore) GetArchivedSession(ctx context.Context, p GetArchivedSessionParams) (ArchivedSession, error) {
	row, err := s.q.GetArchivedSession(ctx, pgstore.GetArchivedSessionParams{OrgID: p.OrgID, SessionID: p.SessionID})
	if err != nil {
		return ArchivedSession{}, mapPostgresErr(err)
	}
	return pgArchivedSession(row), nil
}

// OAuthStateStore
func (s *postgresTxStore) InsertOAuthState(ctx context.Context, p InsertOAuthStateParams) error {
	return mapPostgresErr(s.q.InsertOAuthState(ctx, pgstore.InsertOAuthStateParams{Nonce: p.Nonce, Provider: p.Provider, RedirectUri: p.RedirectURI, CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt}))
}
func (s *postgresTxStore) ConsumeOAuthState(ctx context.Context, nonce string) (OAuthState, error) {
	row, err := s.q.ConsumeOAuthState(ctx, nonce)
	if err != nil {
		return OAuthState{}, mapPostgresErr(err)
	}
	return OAuthState{Nonce: row.Nonce, Provider: row.Provider, RedirectURI: row.RedirectUri, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}, nil
}
func (s *postgresTxStore) CleanupExpiredOAuthState(ctx context.Context, before time.Time) error {
	return mapPostgresErr(s.q.CleanupExpiredOAuthState(ctx, before))
}

// EventLogStore
func (s *postgresTxStore) EnsureEventSeqRow(ctx context.Context, sessionID string) error {
	return mapPostgresErr(s.q.EnsureEventSeqRow(ctx, sessionID))
}
func (s *postgresTxStore) AllocateNextSeq(ctx context.Context, sessionID string) (int64, error) {
	seq, err := s.q.AllocateNextSeq(ctx, sessionID)
	return int64(seq), mapPostgresErr(err)
}
func (s *postgresTxStore) AllocateNextSeqN(ctx context.Context, sessionID string, n int64) (int64, error) {
	seq, err := s.q.AllocateNextSeqN(ctx, pgstore.AllocateNextSeqNParams{Next: int32(n), SessionID: sessionID})
	return int64(seq), mapPostgresErr(err)
}
func (s *postgresTxStore) InsertEvent(ctx context.Context, p InsertEventParams) error {
	return mapPostgresErr(s.q.InsertEvent(ctx, pgstore.InsertEventParams{ID: p.ID, OrgID: p.OrgID, SessionID: p.SessionID, Seq: int32(p.Seq), Type: p.Type, Payload: p.Payload, CreatedAt: p.CreatedAt}))
}
func (s *postgresTxStore) ListEventsSince(ctx context.Context, p ListEventsSinceParams) ([]Event, error) {
	rows, err := s.q.ListEventsSince(ctx, pgstore.ListEventsSinceParams{SessionID: p.SessionID, Seq: int32(p.SinceSeq), Limit: int32(p.Limit)})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{ID: r.ID, OrgID: r.OrgID, SessionID: r.SessionID, Seq: int64(r.Seq), Type: r.Type, Payload: r.Payload, CreatedAt: r.CreatedAt}
	}
	return events, nil
}
func (s *postgresTxStore) ListEventsSinceForDigest(ctx context.Context, p ListEventsSinceForDigestParams) ([]Event, error) {
	rows, err := s.q.ListEventsSinceForDigest(ctx, pgstore.ListEventsSinceForDigestParams{SessionID: p.SessionID, Seq: int32(p.SinceSeq), Limit: int32(p.Limit)})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{ID: r.ID, OrgID: r.OrgID, SessionID: r.SessionID, Seq: int64(r.Seq), Type: r.Type, Payload: r.Payload, CreatedAt: r.CreatedAt}
	}
	return events, nil
}

// PresenceStore
func (s *postgresTxStore) UpsertPresence(ctx context.Context, p UpsertPresenceParams) error {
	return mapPostgresErr(s.q.UpsertPresence(ctx, pgstore.UpsertPresenceParams{OrgID: p.OrgID, SessionID: p.SessionID, AccountID: p.AccountID, Ref: p.Ref, CurrentSha: p.CurrentSHA, LastActiveAt: pgtype.Timestamptz{Time: p.LastActiveAt, Valid: true}}))
}
func (s *postgresTxStore) ListPresenceForSession(ctx context.Context, sessionID string) ([]PresenceRow, error) {
	rows, err := s.q.ListPresenceForSession(ctx, sessionID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	out := make([]PresenceRow, len(rows))
	for i, r := range rows {
		var lastActiveAt time.Time
		if r.LastActiveAt.Valid {
			lastActiveAt = r.LastActiveAt.Time
		}
		out[i] = PresenceRow{OrgID: r.OrgID, SessionID: r.SessionID, AccountID: r.AccountID, Ref: r.Ref, CurrentSHA: r.CurrentSha, LastActiveAt: lastActiveAt}
	}
	return out, nil
}

// OrgInviteStore
func (s *postgresTxStore) InsertOrgInvite(ctx context.Context, p InsertOrgInviteParams) (OrgInvite, error) {
	row, err := s.q.InsertOrgInvite(ctx, pgstore.InsertOrgInviteParams{ID: p.ID, OrgID: p.OrgID, InviterAccountID: p.InviterAccountID, RecipientEmail: p.RecipientEmail, TokenHash: p.TokenHash, CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt, AcceptedAt: p.AcceptedAt, AcceptedByAccountID: ptrToPgText(p.AcceptedByAccountID)})
	if err != nil {
		return OrgInvite{}, mapPostgresErr(err)
	}
	return pgOrgInvite(row), nil
}
func (s *postgresTxStore) GetOrgInviteByID(ctx context.Context, id string) (OrgInvite, error) {
	row, err := s.q.GetOrgInviteByID(ctx, id)
	if err != nil {
		return OrgInvite{}, mapPostgresErr(err)
	}
	return pgOrgInvite(row), nil
}
func (s *postgresTxStore) GetOrgInviteByTokenHash(ctx context.Context, tokenHash string) (OrgInvite, error) {
	row, err := s.q.GetOrgInviteByTokenHash(ctx, tokenHash)
	if err != nil {
		return OrgInvite{}, mapPostgresErr(err)
	}
	return pgOrgInvite(row), nil
}
func (s *postgresTxStore) MarkOrgInviteAccepted(ctx context.Context, p MarkOrgInviteAcceptedParams) error {
	return mapPostgresErr(s.q.MarkOrgInviteAccepted(ctx, pgstore.MarkOrgInviteAcceptedParams{ID: p.ID, AcceptedAt: &p.AcceptedAt, AcceptedByAccountID: ptrToPgText(&p.AcceptedByAccountID)}))
}
func (s *postgresTxStore) ListPendingOrgInvitesForOrg(ctx context.Context, p ListPendingOrgInvitesForOrgParams) ([]OrgInvite, error) {
	rows, err := s.q.ListPendingOrgInvitesForOrg(ctx, pgstore.ListPendingOrgInvitesForOrgParams{OrgID: p.OrgID, ExpiresAt: p.Now})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	invites := make([]OrgInvite, len(rows))
	for i, r := range rows {
		invites[i] = pgOrgInvite(r)
	}
	return invites, nil
}
func (s *postgresTxStore) ListPendingOrgInvitesForEmail(ctx context.Context, p ListPendingOrgInvitesForEmailParams) ([]OrgInvite, error) {
	rows, err := s.q.ListPendingOrgInvitesForEmail(ctx, pgstore.ListPendingOrgInvitesForEmailParams{RecipientEmail: p.Email, ExpiresAt: p.Now})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	invites := make([]OrgInvite, len(rows))
	for i, r := range rows {
		invites[i] = pgOrgInvite(r)
	}
	return invites, nil
}

// RefModeStore
func (s *postgresTxStore) UpsertRefMode(ctx context.Context, p UpsertRefModeParams) error {
	return mapPostgresErr(s.q.UpsertRefMode(ctx, pgstore.UpsertRefModeParams{SessionID: p.SessionID, Ref: p.Ref, Mode: p.Mode}))
}
func (s *postgresTxStore) GetRefMode(ctx context.Context, p GetRefModeParams) (RefMode, error) {
	row, err := s.q.GetRefMode(ctx, pgstore.GetRefModeParams{SessionID: p.SessionID, Ref: p.Ref})
	if err != nil {
		return RefMode{}, mapPostgresErr(err)
	}
	return pgRefMode(row), nil
}
func (s *postgresTxStore) ListRefModesForSession(ctx context.Context, sessionID string) ([]RefMode, error) {
	rows, err := s.q.ListRefModesForSession(ctx, sessionID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	out := make([]RefMode, len(rows))
	for i, r := range rows {
		out[i] = pgRefMode(r)
	}
	return out, nil
}

// SessionInviteStore (TxStore)
func (s *postgresTxStore) InsertSessionInvite(ctx context.Context, p InsertSessionInviteParams) (SessionInvite, error) {
	row, err := s.q.InsertSessionInvite(ctx, pgstore.InsertSessionInviteParams{ID: p.ID, OrgID: p.OrgID, SessionID: p.SessionID, InviterAccountID: p.InviterAccountID, InviteeEmail: p.InviteeEmail, TokenHash: p.TokenHash, CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt, AcceptedAt: p.AcceptedAt, AcceptedByAccountID: ptrToPgText(p.AcceptedByAccountID)})
	if err != nil {
		return SessionInvite{}, mapPostgresErr(err)
	}
	return pgSessionInvite(row), nil
}
func (s *postgresTxStore) GetSessionInviteByID(ctx context.Context, id string) (SessionInvite, error) {
	row, err := s.q.GetSessionInviteByID(ctx, id)
	if err != nil {
		return SessionInvite{}, mapPostgresErr(err)
	}
	return pgSessionInvite(row), nil
}
func (s *postgresTxStore) GetSessionInviteByTokenHash(ctx context.Context, tokenHash string) (SessionInvite, error) {
	row, err := s.q.GetSessionInviteByTokenHash(ctx, tokenHash)
	if err != nil {
		return SessionInvite{}, mapPostgresErr(err)
	}
	return pgSessionInvite(row), nil
}
func (s *postgresTxStore) MarkSessionInviteAccepted(ctx context.Context, p MarkSessionInviteAcceptedParams) error {
	return mapPostgresErr(s.q.MarkSessionInviteAccepted(ctx, pgstore.MarkSessionInviteAcceptedParams{ID: p.ID, AcceptedAt: &p.AcceptedAt, AcceptedByAccountID: ptrToPgText(&p.AcceptedByAccountID)}))
}
func (s *postgresTxStore) ListPendingSessionInvitesForSession(ctx context.Context, p ListPendingSessionInvitesForSessionParams) ([]SessionInvite, error) {
	rows, err := s.q.ListPendingSessionInvitesForSession(ctx, pgstore.ListPendingSessionInvitesForSessionParams{SessionID: p.SessionID, ExpiresAt: p.Now})
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	invites := make([]SessionInvite, len(rows))
	for i, r := range rows {
		invites[i] = pgSessionInvite(r)
	}
	return invites, nil
}

// ---------------------------------------------------------------------------
// ConflictEventStore (outer adapter)
// ---------------------------------------------------------------------------

func (a *postgresAdapter) InsertConflictEvent(ctx context.Context, p InsertConflictEventParams) error {
	return mapPostgresErr(a.q.InsertConflictEvent(ctx, pgstore.InsertConflictEventParams{
		ID:                 p.ID,
		OrgID:              p.OrgID,
		SessionID:          p.SessionID,
		SourceCommit:       p.SourceCommit,
		DraftTip:           p.DraftTip,
		Ancestor:           p.Ancestor,
		Conflicts:          p.Conflicts,
		AddressedTo:        p.AddressedTo,
		Status:             p.Status,
		ResolvingCommitSha: ptrToPgText(p.ResolvingCommitSHA),
		CreatedAt:          p.CreatedAt,
		ResolvedAt:         ptrToPgTimestamptz(p.ResolvedAt),
	}))
}

func (a *postgresAdapter) GetConflictEventByID(ctx context.Context, id string) (ConflictEvent, error) {
	row, err := a.q.GetConflictEventByID(ctx, id)
	if err != nil {
		return ConflictEvent{}, mapPostgresErr(err)
	}
	return pgConflictEvent(row), nil
}

func (a *postgresAdapter) MarkConflictEventResolved(ctx context.Context, p MarkConflictEventResolvedParams) error {
	return mapPostgresErr(a.q.MarkConflictEventResolved(ctx, pgstore.MarkConflictEventResolvedParams{
		ID:                 p.ID,
		SessionID:          p.SessionID,
		ResolvingCommitSha: ptrToPgText(&p.ResolvingCommitSHA),
		ResolvedAt:         ptrToPgTimestamptz(&p.ResolvedAt),
	}))
}

func (a *postgresAdapter) ListOpenConflictEventsForSession(ctx context.Context, sessionID string) ([]ConflictEvent, error) {
	rows, err := a.q.ListOpenConflictEventsForSession(ctx, sessionID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	out := make([]ConflictEvent, len(rows))
	for i, r := range rows {
		out[i] = pgConflictEvent(r)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// ConflictEventStore (TxStore)
// ---------------------------------------------------------------------------

func (s *postgresTxStore) InsertConflictEvent(ctx context.Context, p InsertConflictEventParams) error {
	return mapPostgresErr(s.q.InsertConflictEvent(ctx, pgstore.InsertConflictEventParams{
		ID:                 p.ID,
		OrgID:              p.OrgID,
		SessionID:          p.SessionID,
		SourceCommit:       p.SourceCommit,
		DraftTip:           p.DraftTip,
		Ancestor:           p.Ancestor,
		Conflicts:          p.Conflicts,
		AddressedTo:        p.AddressedTo,
		Status:             p.Status,
		ResolvingCommitSha: ptrToPgText(p.ResolvingCommitSHA),
		CreatedAt:          p.CreatedAt,
		ResolvedAt:         ptrToPgTimestamptz(p.ResolvedAt),
	}))
}

func (s *postgresTxStore) GetConflictEventByID(ctx context.Context, id string) (ConflictEvent, error) {
	row, err := s.q.GetConflictEventByID(ctx, id)
	if err != nil {
		return ConflictEvent{}, mapPostgresErr(err)
	}
	return pgConflictEvent(row), nil
}

func (s *postgresTxStore) MarkConflictEventResolved(ctx context.Context, p MarkConflictEventResolvedParams) error {
	return mapPostgresErr(s.q.MarkConflictEventResolved(ctx, pgstore.MarkConflictEventResolvedParams{
		ID:                 p.ID,
		SessionID:          p.SessionID,
		ResolvingCommitSha: ptrToPgText(&p.ResolvingCommitSHA),
		ResolvedAt:         ptrToPgTimestamptz(&p.ResolvedAt),
	}))
}

func (s *postgresTxStore) ListOpenConflictEventsForSession(ctx context.Context, sessionID string) ([]ConflictEvent, error) {
	rows, err := s.q.ListOpenConflictEventsForSession(ctx, sessionID)
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	out := make([]ConflictEvent, len(rows))
	for i, r := range rows {
		out[i] = pgConflictEvent(r)
	}
	return out, nil
}

// pgConflictEvent converts a pgstore.ConflictEvent to domain ConflictEvent.
func pgConflictEvent(r pgstore.ConflictEvent) ConflictEvent {
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
		ResolvingCommitSHA: pgTextToPtr(r.ResolvingCommitSha),
		CreatedAt:          r.CreatedAt,
		ResolvedAt:         pgTimestamptzToPtr(r.ResolvedAt),
	}
}

// ---------------------------------------------------------------------------
// CommentStore (outer adapter)
// ---------------------------------------------------------------------------

func (a *postgresAdapter) InsertComment(ctx context.Context, p InsertCommentParams) error {
	return mapPostgresErr(a.q.InsertComment(ctx, pgInsertCommentParams(p)))
}

func (a *postgresAdapter) GetCommentByID(ctx context.Context, id string) (Comment, error) {
	row, err := a.q.GetCommentByID(ctx, id)
	if err != nil {
		return Comment{}, mapPostgresErr(err)
	}
	return pgComment(row), nil
}

func (a *postgresAdapter) ResolveComment(ctx context.Context, p ResolveCommentParams) error {
	return mapPostgresErr(a.q.ResolveComment(ctx, pgstore.ResolveCommentParams{
		ID:                  p.ID,
		SessionID:           p.SessionID,
		ResolvedAt:          pgtype.Timestamptz{Time: p.ResolvedAt, Valid: true},
		ResolvedByAccountID: pgtype.Text{String: p.ResolvedByAccountID, Valid: true},
		ResolutionNote:      ptrToPgText(p.ResolutionNote),
	}))
}

func (a *postgresAdapter) ListCommentsForSession(ctx context.Context, p ListCommentsForSessionParams) ([]Comment, error) {
	rows, err := a.q.ListCommentsForSession(ctx, pgListCommentsParams(p))
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	out := make([]Comment, len(rows))
	for i, r := range rows {
		out[i] = pgComment(r)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// CommentStore (TxStore)
// ---------------------------------------------------------------------------

func (s *postgresTxStore) InsertComment(ctx context.Context, p InsertCommentParams) error {
	return mapPostgresErr(s.q.InsertComment(ctx, pgInsertCommentParams(p)))
}

func (s *postgresTxStore) GetCommentByID(ctx context.Context, id string) (Comment, error) {
	row, err := s.q.GetCommentByID(ctx, id)
	if err != nil {
		return Comment{}, mapPostgresErr(err)
	}
	return pgComment(row), nil
}

func (s *postgresTxStore) ResolveComment(ctx context.Context, p ResolveCommentParams) error {
	return mapPostgresErr(s.q.ResolveComment(ctx, pgstore.ResolveCommentParams{
		ID:                  p.ID,
		SessionID:           p.SessionID,
		ResolvedAt:          pgtype.Timestamptz{Time: p.ResolvedAt, Valid: true},
		ResolvedByAccountID: pgtype.Text{String: p.ResolvedByAccountID, Valid: true},
		ResolutionNote:      ptrToPgText(p.ResolutionNote),
	}))
}

func (s *postgresTxStore) ListCommentsForSession(ctx context.Context, p ListCommentsForSessionParams) ([]Comment, error) {
	rows, err := s.q.ListCommentsForSession(ctx, pgListCommentsParams(p))
	if err != nil {
		return nil, mapPostgresErr(err)
	}
	out := make([]Comment, len(rows))
	for i, r := range rows {
		out[i] = pgComment(r)
	}
	return out, nil
}

// pgInsertCommentParams converts domain InsertCommentParams to pgstore.InsertCommentParams.
func pgInsertCommentParams(p InsertCommentParams) pgstore.InsertCommentParams {
	var lineStart, lineEnd pgtype.Int4
	if p.AnchorLineStart != nil {
		lineStart = pgtype.Int4{Int32: *p.AnchorLineStart, Valid: true}
	}
	if p.AnchorLineEnd != nil {
		lineEnd = pgtype.Int4{Int32: *p.AnchorLineEnd, Valid: true}
	}
	return pgstore.InsertCommentParams{
		ID:                  p.ID,
		OrgID:               p.OrgID,
		SessionID:           p.SessionID,
		AuthorAccountID:     p.AuthorAccountID,
		AuthorKind:          p.AuthorKind,
		AnchorCommitSha:     p.AnchorCommitSHA,
		AnchorFilePath:      ptrToPgText(p.AnchorFilePath),
		AnchorLineStart:     lineStart,
		AnchorLineEnd:       lineEnd,
		Body:                p.Body,
		AddressedTo:         ptrToPgText(p.AddressedTo),
		Kind:                p.Kind,
		CreatedAt:           p.CreatedAt,
		ResolvedAt:          ptrToPgTimestamptz(p.ResolvedAt),
		ResolvedByAccountID: ptrToPgText(p.ResolvedByAccountID),
		ResolutionNote:      ptrToPgText(p.ResolutionNote),
	}
}

// pgListCommentsParams converts domain ListCommentsForSessionParams to pgstore.ListCommentsForSessionParams.
func pgListCommentsParams(p ListCommentsForSessionParams) pgstore.ListCommentsForSessionParams {
	addrFilter := ""
	if p.AddressedTo != "" {
		addrFilter = p.AddressedTo
	}
	return pgstore.ListCommentsForSessionParams{
		SessionID:       p.SessionID,
		Column2:         addrFilter,
		Column3:         pgtype.Text{String: addrFilter, Valid: addrFilter != ""},
		Column4:         p.Kind,
		Kind:            p.Kind,
		Column6:         p.ResolvedFilter,
		Column7:         p.ResolvedFilter,
		Column8:         p.ResolvedFilter,
		Column9:         p.AnchorCommitSHA,
		AnchorCommitSha: p.AnchorCommitSHA,
		Column11:        p.AnchorFilePath,
		AnchorFilePath:  pgtype.Text{String: p.AnchorFilePath, Valid: p.AnchorFilePath != ""},
		CreatedAt:       p.Before,
		Limit:           int32(p.Limit),
	}
}

// pgComment converts a pgstore.Comment to domain Comment.
func pgComment(r pgstore.Comment) Comment {
	var lineStart, lineEnd *int32
	if r.AnchorLineStart.Valid {
		v := r.AnchorLineStart.Int32
		lineStart = &v
	}
	if r.AnchorLineEnd.Valid {
		v := r.AnchorLineEnd.Int32
		lineEnd = &v
	}
	return Comment{
		ID:                  r.ID,
		OrgID:               r.OrgID,
		SessionID:           r.SessionID,
		AuthorAccountID:     r.AuthorAccountID,
		AuthorKind:          r.AuthorKind,
		AnchorCommitSHA:     r.AnchorCommitSha,
		AnchorFilePath:      pgTextToPtr(r.AnchorFilePath),
		AnchorLineStart:     lineStart,
		AnchorLineEnd:       lineEnd,
		Body:                r.Body,
		AddressedTo:         pgTextToPtr(r.AddressedTo),
		Kind:                r.Kind,
		CreatedAt:           r.CreatedAt,
		ResolvedAt:          pgTimestamptzToPtr(r.ResolvedAt),
		ResolvedByAccountID: pgTextToPtr(r.ResolvedByAccountID),
		ResolutionNote:      pgTextToPtr(r.ResolutionNote),
	}
}

// ---------------------------------------------------------------------------
// FinalizeLockStore (outer adapter)
// ---------------------------------------------------------------------------

func (a *postgresAdapter) InsertFinalizeLock(ctx context.Context, p InsertFinalizeLockParams) error {
	return mapPostgresErr(a.q.InsertFinalizeLock(ctx, pgInsertFinalizeLockParams(p)))
}

func (a *postgresAdapter) GetFinalizeLockByID(ctx context.Context, id string) (FinalizeLock, error) {
	row, err := a.q.GetFinalizeLockByID(ctx, id)
	if err != nil {
		return FinalizeLock{}, mapPostgresErr(err)
	}
	return pgFinalizeLock(row), nil
}

func (a *postgresAdapter) GetActiveFinalizeLockForSession(ctx context.Context, sessionID string) (FinalizeLock, error) {
	row, err := a.q.GetActiveFinalizeLockForSession(ctx, sessionID)
	if err != nil {
		return FinalizeLock{}, mapPostgresErr(err)
	}
	return pgFinalizeLock(row), nil
}

func (a *postgresAdapter) UpdateFinalizeLockCuration(ctx context.Context, p UpdateFinalizeLockCurationParams) error {
	baseSHA := p.BaseSHA
	return mapPostgresErr(a.q.UpdateFinalizeLockCuration(ctx, pgstore.UpdateFinalizeLockCurationParams{
		ID:                 p.ID,
		SelectedCommitShas: []byte(p.SelectedCommitSHAs),
		TargetBranch:       p.TargetBranch,
		BaseSha:            &baseSHA,
		Mode:               p.Mode,
		CommitMessage:      ptrToPgText(p.CommitMessage),
		LastActivityAt:     pgtype.Timestamptz{Time: p.LastActivityAt, Valid: true},
	}))
}

func (a *postgresAdapter) TouchFinalizeLock(ctx context.Context, p TouchFinalizeLockParams) error {
	return mapPostgresErr(a.q.TouchFinalizeLock(ctx, pgstore.TouchFinalizeLockParams{
		ID:             p.ID,
		LastActivityAt: pgtype.Timestamptz{Time: p.LastActivityAt, Valid: true},
	}))
}

func (a *postgresAdapter) ReleaseFinalizeLock(ctx context.Context, p ReleaseFinalizeLockParams) error {
	return mapPostgresErr(a.q.ReleaseFinalizeLock(ctx, pgstore.ReleaseFinalizeLockParams{
		ID:         p.ID,
		ReleasedAt: pgtype.Timestamptz{Time: p.ReleasedAt, Valid: true},
	}))
}

func (a *postgresAdapter) SupersedeFinalizeLock(ctx context.Context, p SupersedeFinalizeLockParams) error {
	return mapPostgresErr(a.q.SupersedeFinalizeLock(ctx, pgstore.SupersedeFinalizeLockParams{
		ID:                 p.ID,
		SupersededByLockID: pgtype.Text{String: p.SupersededByLockID, Valid: true},
	}))
}

// ---------------------------------------------------------------------------
// FinalizeLockStore (TxStore)
// ---------------------------------------------------------------------------

func (s *postgresTxStore) InsertFinalizeLock(ctx context.Context, p InsertFinalizeLockParams) error {
	return mapPostgresErr(s.q.InsertFinalizeLock(ctx, pgInsertFinalizeLockParams(p)))
}

func (s *postgresTxStore) GetFinalizeLockByID(ctx context.Context, id string) (FinalizeLock, error) {
	row, err := s.q.GetFinalizeLockByID(ctx, id)
	if err != nil {
		return FinalizeLock{}, mapPostgresErr(err)
	}
	return pgFinalizeLock(row), nil
}

func (s *postgresTxStore) GetActiveFinalizeLockForSession(ctx context.Context, sessionID string) (FinalizeLock, error) {
	row, err := s.q.GetActiveFinalizeLockForSession(ctx, sessionID)
	if err != nil {
		return FinalizeLock{}, mapPostgresErr(err)
	}
	return pgFinalizeLock(row), nil
}

func (s *postgresTxStore) UpdateFinalizeLockCuration(ctx context.Context, p UpdateFinalizeLockCurationParams) error {
	baseSHA := p.BaseSHA
	return mapPostgresErr(s.q.UpdateFinalizeLockCuration(ctx, pgstore.UpdateFinalizeLockCurationParams{
		ID:                 p.ID,
		SelectedCommitShas: []byte(p.SelectedCommitSHAs),
		TargetBranch:       p.TargetBranch,
		BaseSha:            &baseSHA,
		Mode:               p.Mode,
		CommitMessage:      ptrToPgText(p.CommitMessage),
		LastActivityAt:     pgtype.Timestamptz{Time: p.LastActivityAt, Valid: true},
	}))
}

func (s *postgresTxStore) TouchFinalizeLock(ctx context.Context, p TouchFinalizeLockParams) error {
	return mapPostgresErr(s.q.TouchFinalizeLock(ctx, pgstore.TouchFinalizeLockParams{
		ID:             p.ID,
		LastActivityAt: pgtype.Timestamptz{Time: p.LastActivityAt, Valid: true},
	}))
}

func (s *postgresTxStore) ReleaseFinalizeLock(ctx context.Context, p ReleaseFinalizeLockParams) error {
	return mapPostgresErr(s.q.ReleaseFinalizeLock(ctx, pgstore.ReleaseFinalizeLockParams{
		ID:         p.ID,
		ReleasedAt: pgtype.Timestamptz{Time: p.ReleasedAt, Valid: true},
	}))
}

func (s *postgresTxStore) SupersedeFinalizeLock(ctx context.Context, p SupersedeFinalizeLockParams) error {
	return mapPostgresErr(s.q.SupersedeFinalizeLock(ctx, pgstore.SupersedeFinalizeLockParams{
		ID:                 p.ID,
		SupersededByLockID: pgtype.Text{String: p.SupersededByLockID, Valid: true},
	}))
}

// pgInsertFinalizeLockParams converts domain InsertFinalizeLockParams to pgstore.InsertFinalizeLockParams.
func pgInsertFinalizeLockParams(p InsertFinalizeLockParams) pgstore.InsertFinalizeLockParams {
	baseSHA := p.BaseSHA
	return pgstore.InsertFinalizeLockParams{
		ID:                  p.ID,
		OrgID:               p.OrgID,
		SessionID:           p.SessionID,
		AcquiredByAccountID: p.AcquiredByAccountID,
		AcquiredAt:          pgtype.Timestamptz{Time: p.AcquiredAt, Valid: true},
		LastActivityAt:      pgtype.Timestamptz{Time: p.LastActivityAt, Valid: true},
		SelectedCommitShas:  []byte(p.SelectedCommitSHAs),
		TargetBranch:        p.TargetBranch,
		BaseSha:             &baseSHA,
		Mode:                p.Mode,
		CommitMessage:       ptrToPgText(p.CommitMessage),
		SupersededByLockID:  ptrToPgText(p.SupersededByLockID),
		ReleasedAt:          ptrToPgTimestamptz(p.ReleasedAt),
	}
}

// pgFinalizeLock converts a pgstore.FinalizeLock to domain FinalizeLock.
func pgFinalizeLock(r pgstore.FinalizeLock) FinalizeLock {
	baseSHA := ""
	if r.BaseSha != nil {
		baseSHA = *r.BaseSha
	}
	return FinalizeLock{
		ID:                  r.ID,
		OrgID:               r.OrgID,
		SessionID:           r.SessionID,
		AcquiredByAccountID: r.AcquiredByAccountID,
		AcquiredAt:          r.AcquiredAt.Time,
		LastActivityAt:      r.LastActivityAt.Time,
		SelectedCommitSHAs:  string(r.SelectedCommitShas),
		TargetBranch:        r.TargetBranch,
		BaseSHA:             baseSHA,
		Mode:                r.Mode,
		CommitMessage:       pgTextToPtr(r.CommitMessage),
		SupersededByLockID:  pgTextToPtr(r.SupersededByLockID),
		ReleasedAt:          pgTimestamptzToPtr(r.ReleasedAt),
	}
}

// ---------------------------------------------------------------------------
// LeaseStore (outer adapter)
// ---------------------------------------------------------------------------

func (a *postgresAdapter) IssueLeaseFencingToken(ctx context.Context) (int64, error) {
	token, err := a.q.IssueLeaseFencingToken(ctx)
	return token, mapPostgresErr(err)
}

func (a *postgresAdapter) InsertLease(ctx context.Context, p InsertLeaseParams) (Lease, error) {
	row, err := a.q.InsertLease(ctx, pgstore.InsertLeaseParams{
		SessionID:    p.SessionID,
		PodID:        p.PodID,
		FencingToken: p.FencingToken,
	})
	if err != nil {
		return Lease{}, mapPostgresErr(err)
	}
	return pgLease(row), nil
}

func (a *postgresAdapter) MarkLeaseReleased(ctx context.Context, sessionID string) error {
	return mapPostgresErr(a.q.MarkLeaseReleased(ctx, sessionID))
}

func (a *postgresAdapter) UpdateLeaseHeartbeat(ctx context.Context, sessionID string) error {
	return mapPostgresErr(a.q.UpdateLeaseHeartbeat(ctx, sessionID))
}

func (a *postgresAdapter) DeleteReleasedLeasesOlderThan(ctx context.Context, before time.Time) error {
	return mapPostgresErr(a.q.DeleteReleasedLeasesOlderThan(ctx, before))
}

// ---------------------------------------------------------------------------
// LeaseStore (TxStore)
// ---------------------------------------------------------------------------

func (s *postgresTxStore) IssueLeaseFencingToken(ctx context.Context) (int64, error) {
	token, err := s.q.IssueLeaseFencingToken(ctx)
	return token, mapPostgresErr(err)
}

func (s *postgresTxStore) InsertLease(ctx context.Context, p InsertLeaseParams) (Lease, error) {
	row, err := s.q.InsertLease(ctx, pgstore.InsertLeaseParams{
		SessionID:    p.SessionID,
		PodID:        p.PodID,
		FencingToken: p.FencingToken,
	})
	if err != nil {
		return Lease{}, mapPostgresErr(err)
	}
	return pgLease(row), nil
}

func (s *postgresTxStore) MarkLeaseReleased(ctx context.Context, sessionID string) error {
	return mapPostgresErr(s.q.MarkLeaseReleased(ctx, sessionID))
}

func (s *postgresTxStore) UpdateLeaseHeartbeat(ctx context.Context, sessionID string) error {
	return mapPostgresErr(s.q.UpdateLeaseHeartbeat(ctx, sessionID))
}

func (s *postgresTxStore) DeleteReleasedLeasesOlderThan(ctx context.Context, before time.Time) error {
	return mapPostgresErr(s.q.DeleteReleasedLeasesOlderThan(ctx, before))
}

// pgLease converts a pgstore.Lease to a domain Lease.
func pgLease(r pgstore.Lease) Lease {
	return Lease{
		SessionID:    r.SessionID,
		PodID:        r.PodID,
		FencingToken: r.FencingToken,
		AcquiredAt:   r.AcquiredAt,
		ReleasedAt:   pgTimestamptzToPtr(r.ReleasedAt),
		HeartbeatAt:  r.HeartbeatAt,
	}
}
