package store

import (
	"errors"
	"fmt"
	"testing"
)

// itoa is the trivial convert function used throughout these tests:
// it converts an int source row to its string representation.
func itoa(n int) string { return fmt.Sprintf("%d", n) }

// wrapErr is the trivial mapErr function used throughout these tests:
// it prefixes the error message so callers can verify it was invoked.
func wrapErr(err error) error { return fmt.Errorf("wrapped: %w", err) }

// TestWrap1_Success verifies that wrap1 returns convert(row) and nil
// when the incoming error is nil.
func TestWrap1_Success(t *testing.T) {
	result, err := wrap1(42, nil, wrapErr, itoa)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result != "42" {
		t.Fatalf("expected %q, got %q", "42", result)
	}
}

// TestWrap1_Error verifies that wrap1 returns the zero value for D and
// mapErr(err) when the incoming error is non-nil.
func TestWrap1_Error(t *testing.T) {
	sentinel := errors.New("db failure")
	mapErrCalled := false
	customMapErr := func(err error) error {
		mapErrCalled = true
		return wrapErr(err)
	}

	result, err := wrap1(0, sentinel, customMapErr, itoa)

	if result != "" {
		t.Fatalf("expected zero value (empty string), got %q", result)
	}
	if !mapErrCalled {
		t.Fatal("expected mapErr to be called, but it was not")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
}

// TestWrapList_Success verifies that wrapList returns a correctly mapped
// slice and nil error when the incoming error is nil.
func TestWrapList_Success(t *testing.T) {
	rows := []int{1, 2, 3}
	result, err := wrapList(rows, nil, wrapErr, itoa)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	expected := []string{"1", "2", "3"}
	if len(result) != len(expected) {
		t.Fatalf("expected len %d, got %d", len(expected), len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Fatalf("result[%d]: expected %q, got %q", i, v, result[i])
		}
	}
}

// TestWrapList_Error verifies that wrapList returns nil and mapErr(err)
// when the incoming error is non-nil.
func TestWrapList_Error(t *testing.T) {
	sentinel := errors.New("db list failure")
	mapErrCalled := false
	customMapErr := func(err error) error {
		mapErrCalled = true
		return wrapErr(err)
	}

	result, err := wrapList([]int{1, 2}, sentinel, customMapErr, itoa)

	if result != nil {
		t.Fatalf("expected nil slice on error, got %v", result)
	}
	if !mapErrCalled {
		t.Fatal("expected mapErr to be called, but it was not")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
}

// TestWrapList_EmptySlice verifies that wrapList handles an empty input
// slice correctly: it returns a zero-length (non-nil) slice and a nil
// error, and never calls mapErr.
func TestWrapList_EmptySlice(t *testing.T) {
	mapErrCalled := false
	customMapErr := func(err error) error {
		mapErrCalled = true
		return wrapErr(err)
	}

	result, err := wrapList([]int{}, nil, customMapErr, itoa)

	if err != nil {
		t.Fatalf("expected nil error for empty input, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil slice for empty input, got nil")
	}
	if len(result) != 0 {
		t.Fatalf("expected zero-length slice, got len %d", len(result))
	}
	if mapErrCalled {
		t.Fatal("expected mapErr NOT to be called for empty success path, but it was")
	}
}
