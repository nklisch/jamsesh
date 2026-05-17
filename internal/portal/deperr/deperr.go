// Package deperr declares sentinel errors that mark a request failure
// as caused by an external dependency (SMTP, DB, OAuth provider, git
// subprocess) rather than a business-logic problem. Handlers wrap
// dep-class failures with the helpers here; the strict-handler
// translator in httperr classifies them into typed envelopes.
package deperr

import (
	"errors"
	"fmt"

	"jamsesh/internal/db/store"
)

// Sentinel errors. Match with errors.Is.
var (
	ErrSMTP          = errors.New("dep: smtp unavailable")
	ErrDB            = errors.New("dep: database unavailable")
	ErrOAuthProvider = errors.New("dep: oauth provider unavailable")
	ErrGitSubprocess = errors.New("dep: git subprocess failed")
)

// WrapSMTP marks err as an SMTP-dep failure. Returns nil when err is nil.
func WrapSMTP(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrSMTP, err)
}

// WrapDB marks err as a DB-dep failure unconditionally. Prefer
// WrapDBIfTransient at call sites where a known business sentinel
// (store.ErrNotFound / store.ErrUniqueViolation) may also be the value.
func WrapDB(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrDB, err)
}

// WrapDBIfTransient returns err unchanged when it is a recognized
// business-class store sentinel; otherwise it wraps as ErrDB. Returns
// nil when err is nil.
func WrapDBIfTransient(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrNotFound) ||
		errors.Is(err, store.ErrUniqueViolation) {
		return err
	}
	return WrapDB(err)
}

// WrapOAuthProvider marks err as an OAuth-provider HTTP failure.
// Returns nil when err is nil.
func WrapOAuthProvider(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrOAuthProvider, err)
}

// WrapGitSubprocess marks err as a git-subprocess failure. Returns nil
// when err is nil.
func WrapGitSubprocess(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrGitSubprocess, err)
}
