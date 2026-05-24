// Package store — null/text/time converters used by the dialect-specific
// adapters. Co-located here so the structural similarity is visible in one
// place and any future unification (generics-based helpers, code-gen) has a
// single home.
//
// Both dialect families expose the same logical pattern:
//
//	NullableType{Valid bool, <value field>} ↔ *GoType
//
// Go generics cannot cleanly bind across these because field-access is not
// expressible via method constraints. The deferred next step is either a
// generics-based helper (if the constraint gap closes) or a code-gen pass
// that eliminates the per-dialect duplication entirely.
package store

import (
	"database/sql"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// SQLite-side: sql.NullString / sql.NullTime ↔ *string / *time.Time
// ---------------------------------------------------------------------------

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
// Postgres-side: pgtype.Text / pgtype.Timestamptz ↔ *string / *time.Time
// ---------------------------------------------------------------------------

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
