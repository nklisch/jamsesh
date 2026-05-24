package prereceive

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// Validator is the top-level pre-receive policy enforcer. It is constructed
// once (typically at server startup with the configured limits) and its
// Validate method is called for each incoming push.
type Validator struct {
	// MaxPackBytes is the maximum number of bytes a pushed pack may contain.
	// A value of 0 or negative disables the size check.
	MaxPackBytes int64

	// PlaygroundMaxContentBytes is the per-session accumulated repo content
	// cap in bytes for playground sessions. A push that would push the total
	// repo size (current on-disk size + incoming pack) beyond this value is
	// rejected with playground.size_exceeded. A value of 0 or negative
	// disables the playground content-size check.
	PlaygroundMaxContentBytes int64

	// Logger is used for best-effort warnings during playground-caps checks
	// (e.g. when the repo-size walk fails). When nil, slog.Default() is used.
	Logger *slog.Logger
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
		// Base-ref first push exemption: the seed commits a user pushes to
		// refs/heads/jam/<sessionID>/base predate the session and by
		// definition cannot carry session-aware trailers (Jam-Session,
		// Jam-Turn, Jam-Author). Trailer/scope enforcement applies to
		// subsequent collaborative pushes only — applying it to the
		// bootstrap push is a category error (chicken-and-egg).
		//
		// Narrow exemption: only the base ref (refs/heads/jam/<id>/base),
		// only the first push (OldSHA empty). Subsequent base-ref
		// updates (rare; usually force-push for reseeding, which is also
		// blocked by ValidateRef's force-push check) still go through
		// full per-commit validation.
		if isBaseRefFirstPush(in.Session.ID, u) {
			continue
		}
		rejections = append(rejections, WalkAndValidate(ctx, in.Repo, u, scope)...)
	}

	// 4. Playground-specific caps (content-size). Fires only for pushes to
	// the reserved playground org. Durable session pushes return nil
	// immediately — the org_id branch is the guard.
	if r, ok := v.CheckPlaygroundCaps(ctx, in); !ok {
		rejections = append(rejections, r)
	}

	return ValidateResult{
		OK:         len(rejections) == 0,
		Rejections: rejections,
	}, nil
}

// isBaseRefFirstPush reports whether u is the inaugural push to the
// session's base ref (refs/heads/jam/<sessionID>/base with empty OldSHA).
// The seed commits a user pushes here are their pre-session working-tree
// commits — they predate the session and so cannot carry session-aware
// trailers (Jam-Session, Jam-Turn, Jam-Author) naming this session.
// Trailer enforcement is for collaborative session work, not bootstrap.
//
// Returns true exactly when both conditions hold:
//   - The ref name is the two-segment base ref: refs/heads/jam/<sessionID>/base
//   - The update has no parent SHA (OldSHA is empty — i.e. this is the
//     create-ref operation, not a subsequent update)
func isBaseRefFirstPush(sessionID string, u RefUpdate) bool {
	return u.OldSHA == "" && u.Ref == "refs/heads/jam/"+sessionID+"/base"
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
