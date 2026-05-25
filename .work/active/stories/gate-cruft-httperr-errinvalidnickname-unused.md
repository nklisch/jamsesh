---
id: gate-cruft-httperr-errinvalidnickname-unused
kind: story
stage: review
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# httperr.ErrInvalidNickname has zero callers

## Confidence
High

## Category
dead function

## Location
`internal/portal/httperr/httperr.go:173-181`

## Evidence
```go
// ErrInvalidNickname is emitted when a join request supplies a nickname that
// violates the 2-24 char, letters/digits/dashes rule. Returns 400.
func ErrInvalidNickname() *Error {
	return &Error{
		Code:       "playground.invalid_nickname",
		Message:    "nickname must be 2-24 characters, letters/digits/dashes only",
		HTTPStatus: http.StatusBadRequest,
	}
}
```

`deadcode -test ./...` reports it unreachable. `grep -rn 'ErrInvalidNickname' --include="*.go"` returns only the declaration site — no callers in production or test code. The actual nickname validation in `internal/portal/playground/handler.go:282` uses an inline `nicknameRE.MatchString` check and emits its own `*Error`.

## Removal
Delete the function and its docstring (lines 173-181). Verify with `go build ./...` and `go test ./...`. No other edits needed.

## Implementation notes
Deleted `ErrInvalidNickname` and its docstring from `internal/portal/httperr/httperr.go`. `go build ./...` and `go test ./internal/portal/httperr/...` pass cleanly.
