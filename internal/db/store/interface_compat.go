// Package store will hold the hand-written Store interface and dialect
// adapters (delivered in the store-and-adapters story). This file exists
// now to provide the compile-time interface compatibility check between
// the two sqlc-generated dialect packages.
package store

// Compile-time assertions that each dialect's *Queries satisfies its own
// generated Querier interface. sqlc emits these in querier.go already, but
// repeating them here makes the check visible at the store package boundary
// and ensures the generated files are present and importable.
//
// Note on dialect divergence: the two Querier interfaces are NOT
// cross-assignable because nullable columns produce dialect-specific types:
//   - sqlitestore: sql.NullString  for nullable TEXT
//   - pgstore:     pgtype.Text     for nullable TEXT
//
// This is expected and documented. The store adapter (delivered in
// epic-portal-foundation-data-layer-store-and-adapters) translates between
// dialect row types and canonical domain types, keeping handler code
// dialect-agnostic.
//
// To add a stronger method-set check, see interface_compat_test.go.

import (
	"jamsesh/internal/db/pgstore"
	"jamsesh/internal/db/sqlitestore"
)

var (
	// Each generated *Queries must satisfy its own Querier.
	_ sqlitestore.Querier = (*sqlitestore.Queries)(nil)
	_ pgstore.Querier     = (*pgstore.Queries)(nil)
)
