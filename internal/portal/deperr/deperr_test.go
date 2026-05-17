package deperr_test

import (
	"errors"
	"testing"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
)

func TestWrapSMTP_NilPassthrough(t *testing.T) {
	if got := deperr.WrapSMTP(nil); got != nil {
		t.Errorf("WrapSMTP(nil) = %v, want nil", got)
	}
}

func TestWrapSMTP_MatchesSentinel(t *testing.T) {
	cause := errors.New("tls handshake failed")
	wrapped := deperr.WrapSMTP(cause)
	if !errors.Is(wrapped, deperr.ErrSMTP) {
		t.Errorf("errors.Is(wrapped, deperr.ErrSMTP) = false, want true")
	}
}

func TestWrapDB_NilPassthrough(t *testing.T) {
	if got := deperr.WrapDB(nil); got != nil {
		t.Errorf("WrapDB(nil) = %v, want nil", got)
	}
}

func TestWrapDB_MatchesSentinel(t *testing.T) {
	cause := errors.New("conn refused")
	wrapped := deperr.WrapDB(cause)
	if !errors.Is(wrapped, deperr.ErrDB) {
		t.Errorf("errors.Is(wrapped, deperr.ErrDB) = false, want true")
	}
}

func TestWrapOAuthProvider_NilPassthrough(t *testing.T) {
	if got := deperr.WrapOAuthProvider(nil); got != nil {
		t.Errorf("WrapOAuthProvider(nil) = %v, want nil", got)
	}
}

func TestWrapOAuthProvider_MatchesSentinel(t *testing.T) {
	cause := errors.New("502 bad gateway")
	wrapped := deperr.WrapOAuthProvider(cause)
	if !errors.Is(wrapped, deperr.ErrOAuthProvider) {
		t.Errorf("errors.Is(wrapped, deperr.ErrOAuthProvider) = false, want true")
	}
}

func TestWrapGitSubprocess_NilPassthrough(t *testing.T) {
	if got := deperr.WrapGitSubprocess(nil); got != nil {
		t.Errorf("WrapGitSubprocess(nil) = %v, want nil", got)
	}
}

func TestWrapGitSubprocess_MatchesSentinel(t *testing.T) {
	cause := errors.New("exit status 128")
	wrapped := deperr.WrapGitSubprocess(cause)
	if !errors.Is(wrapped, deperr.ErrGitSubprocess) {
		t.Errorf("errors.Is(wrapped, deperr.ErrGitSubprocess) = false, want true")
	}
}

func TestWrapDBIfTransient_NilPassthrough(t *testing.T) {
	if got := deperr.WrapDBIfTransient(nil); got != nil {
		t.Errorf("WrapDBIfTransient(nil) = %v, want nil", got)
	}
}

func TestWrapDBIfTransient_PreservesNotFound(t *testing.T) {
	got := deperr.WrapDBIfTransient(store.ErrNotFound)
	// Must NOT be wrapped — business sentinels are returned unchanged so
	// downstream handlers still see them via errors.Is.
	if got != store.ErrNotFound {
		t.Errorf("WrapDBIfTransient(ErrNotFound) = %v, want unchanged ErrNotFound", got)
	}
	if errors.Is(got, deperr.ErrDB) {
		t.Errorf("WrapDBIfTransient(ErrNotFound) wrapped as ErrDB; should be preserved")
	}
}

func TestWrapDBIfTransient_PreservesUniqueViolation(t *testing.T) {
	got := deperr.WrapDBIfTransient(store.ErrUniqueViolation)
	if got != store.ErrUniqueViolation {
		t.Errorf("WrapDBIfTransient(ErrUniqueViolation) = %v, want unchanged ErrUniqueViolation", got)
	}
	if errors.Is(got, deperr.ErrDB) {
		t.Errorf("WrapDBIfTransient(ErrUniqueViolation) wrapped as ErrDB; should be preserved")
	}
}

func TestWrapDBIfTransient_PreservesWrappedNotFound(t *testing.T) {
	wrappedNotFound := errors.Join(store.ErrNotFound, errors.New("extra context"))
	got := deperr.WrapDBIfTransient(wrappedNotFound)
	// errors.Is should still see ErrNotFound through the join, and
	// WrapDBIfTransient should preserve it (no ErrDB wrap).
	if !errors.Is(got, store.ErrNotFound) {
		t.Errorf("WrapDBIfTransient lost ErrNotFound through errors.Join wrapping")
	}
	if errors.Is(got, deperr.ErrDB) {
		t.Errorf("WrapDBIfTransient wrapped a known-sentinel error as ErrDB")
	}
}

func TestWrapDBIfTransient_WrapsUnknownError(t *testing.T) {
	cause := errors.New("conn refused")
	got := deperr.WrapDBIfTransient(cause)
	if !errors.Is(got, deperr.ErrDB) {
		t.Errorf("WrapDBIfTransient(unknown) didn't wrap as ErrDB; got %v", got)
	}
}
