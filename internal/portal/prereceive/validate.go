package prereceive

import (
	"context"
	"encoding/json"
	"fmt"
)

// Validator is the top-level pre-receive policy enforcer. It is constructed
// once (typically at server startup with the configured limits) and its
// Validate method is called for each incoming push.
type Validator struct {
	// MaxPackBytes is the maximum number of bytes a pushed pack may contain.
	// A value of 0 or negative disables the size check.
	MaxPackBytes int64
}

// Validate runs all pre-receive policy checks for a push and returns a
// ValidateResult. The result's OK field is true iff no rejections were found.
//
// The checks performed, in order:
//  1. Pack size (CheckPackSize) — rejects the whole push early if too large.
//  2. For each ref update:
//     a. Ref namespace + force-push check (ValidateRef).
//     b. Per-commit trailer + writable-scope check (WalkAndValidate).
//
// All rejections are accumulated; the first failure does NOT abort the run so
// that the caller can return a complete list of violations to the client.
//
// Validate returns a non-nil error only for unexpected internal failures
// (e.g. a malformed writable_scope JSON in the session row). Policy
// rejections are always expressed through ValidateResult.Rejections.
func (v *Validator) Validate(ctx context.Context, in ValidateInput) (ValidateResult, error) {
	var rejections []Rejection

	// 1. Pack size.
	if r, ok := CheckPackSize(in.PackBytes, v.MaxPackBytes); !ok {
		rejections = append(rejections, r)
	}

	// 2. Compile the writable scope once for all updates.
	globs, err := parseWritableScope(in.Session.WritableScope)
	if err != nil {
		return ValidateResult{}, fmt.Errorf("prereceive: parse writable_scope: %w", err)
	}
	scope, err := CompileScope(globs)
	if err != nil {
		return ValidateResult{}, fmt.Errorf("prereceive: compile scope: %w", err)
	}

	// 3. Per-ref checks.
	for _, u := range in.Updates {
		rejections = append(rejections, ValidateRef(ctx, in.Repo, in.Session.ID, in.Account.ID, u)...)
		rejections = append(rejections, WalkAndValidate(ctx, in.Repo, u, scope)...)
	}

	return ValidateResult{
		OK:         len(rejections) == 0,
		Rejections: rejections,
	}, nil
}

// parseWritableScope unmarshals the JSON-encoded glob array stored in
// session.WritableScope. An empty or whitespace-only string is treated as an
// empty scope (deny-by-default). The field is stored as a JSON array of
// strings, e.g. `["src/**","docs/**"]`.
func parseWritableScope(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var globs []string
	if err := json.Unmarshal([]byte(raw), &globs); err != nil {
		return nil, fmt.Errorf("invalid JSON in writable_scope %q: %w", raw, err)
	}
	return globs, nil
}
