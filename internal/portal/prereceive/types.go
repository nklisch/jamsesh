// Package prereceive implements the pre-receive push-policy library for
// jamsesh portal git sessions. It validates every aspect of a proposed push
// (commit trailers, writable scope, ref namespace, force-push, pack size)
// and returns structured rejection information.
//
// The package is a pure validation library: no database access, no HTTP, no
// event emission. All I/O is via the go-git Repository passed through
// ValidateInput.
package prereceive

import (
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/gitref"

	git "github.com/go-git/go-git/v5"
)

// Error-code constants. These match the push.* codes in docs/PROTOCOL.md.
const (
	CodeScopeViolation       = "push.scope_violation"
	CodeRefNamespaceViolation = "push.ref_namespace_violation"
	CodeMissingTrailer       = "push.missing_trailer"
	CodeSizeLimit            = "push.size_limit"
	CodeForcePushRejected    = "push.force_push_rejected"
)

// Rejection describes a single policy violation found during validation.
// Code identifies the violation class; Message is human-readable; Details
// carries structured context (paths, missing trailer names, ref, etc.) for
// machine consumers.
type Rejection struct {
	Code    string
	Message string
	Details map[string]any
}

// RefUpdate is an alias for gitref.RefUpdate. See gitref.RefUpdate for field
// documentation.
type RefUpdate = gitref.RefUpdate

// ValidateInput is the complete context passed to the validator.
// Repo is a go-git Repository opened against the bare repo on disk.
// Session and Account are the authenticated caller's data from the DB.
// Updates lists every ref being updated by the push.
// PackBytes is the Content-Length of the pushed pack (used for size check).
type ValidateInput struct {
	Repo      *git.Repository
	Session   *store.Session
	Account   *store.Account
	Updates   []RefUpdate
	PackBytes int64
}

// ValidateResult is the outcome of a full validation run.
// OK is true iff Rejections is empty.
type ValidateResult struct {
	OK         bool
	Rejections []Rejection
}
