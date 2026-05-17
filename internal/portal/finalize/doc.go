// Package finalize implements the portal-side surface that backs the
// finalize flow: a durable finalize-lock substrate, the plan-generation
// endpoint that composes a cherry-pick script from curated SHAs, an
// ephemeral fetch-token mint for the HTTPS-fallback git fetch, and the
// manual "mark shipped" status transition.
//
// This file is the package skeleton: Handler struct + New constructor +
// shared collaborator wiring. Concrete endpoints live in per-file siblings
// (lock_acquire.go, lock_patch.go, lock_release.go, plan.go, fetch_token.go,
// mark_shipped.go).
//
// The lock-state machine is the durable artifact: one finalize_locks row
// per in-flight finalize coordination. The 30-minute idle release is
// checked on read — every endpoint that touches a lock first compares
// now - last_activity_at against the TTL. No background sweeper.
//
// Architectural choice (locked at epic-design): a new
// internal/portal/finalize/ package satisfying the oapi-codegen
// StrictServerInterface methods for these endpoints, wired into the
// existing combinedHandler in cmd/portal/main.go.
package finalize
