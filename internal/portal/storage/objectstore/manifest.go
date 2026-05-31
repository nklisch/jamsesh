package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrFenced is returned by Save when the caller's manifest carries a fencing
// token that is strictly less than the token recorded on-disk. This means the
// caller is operating from a stale lease — the lease has been handed to a
// newer holder whose writes have already advanced the high-water mark.
//
// ErrFenced is distinct from ErrPrecondition: a precondition failure means a
// concurrent writer raced us (retry may help); a fenced failure means the
// caller's lease is permanently stale (abort and 503 is the right response).
var ErrFenced = errors.New("objectstore: write blocked by higher on-disk fencing token (stale lease)")

// ErrCorruptManifest is returned by Load when a manifest object exists in
// storage but is not a readable schema-version-1 manifest for the requested
// session: it decodes to an unknown/zero Version, carries an empty SessionID,
// or carries a SessionID that does not match the key it was fetched under.
//
// Such a manifest cannot be trusted to describe the session's pack/ref state,
// so Load refuses to return it. Callers (hydration, sync pre-flight) MUST fail
// fast rather than treat a corrupt manifest as a clean slate — silently
// hydrating an empty repo from an unreadable manifest would drop the session's
// real history (silent truncation / data loss). This realises the contract
// stated on the Manifest type: "Unknown versions should be treated as
// unreadable by future code — return an error rather than silently
// mis-parsing."
var ErrCorruptManifest = errors.New("objectstore: manifest exists but is corrupt (unreadable schema)")

// manifestSchemaVersion is the only manifest schema version this code accepts.
// Save normalises a zero Version to this value on write, so every manifest
// legitimately written by the system carries exactly this version on disk.
const manifestSchemaVersion = 1

// Manifest is the per-session linearizable state object stored at
// sessions/<id>/manifest.json. It lists the current pack files, the current
// ref map, the raw packed-refs content, and the high-water fencing token.
//
// Hydration uses the Manifest to know exactly which objects to download when
// acquiring the lease for a session. Pack and ref lists are maintained here
// rather than by walking object-storage listings (which are eventually
// consistent on some providers).
//
// Schema version is 1. Callers MUST NOT write a version other than 1 (or 0,
// which ManifestStore.Save normalises to 1). Unknown versions should be
// treated as unreadable by future code — return an error rather than
// silently mis-parsing.
type Manifest struct {
	// Version is the schema version. Always 1 in the current implementation.
	Version int `json:"version"`

	// SessionID is the jam session this manifest belongs to. Stored in the
	// body (redundant with the key) so readers can validate they fetched
	// the right manifest.
	SessionID string `json:"session_id"`

	// Packs lists every pack file currently stored in object storage for
	// this session. Order is not significant; hydration downloads all of
	// them.
	Packs []PackEntry `json:"packs"`

	// Refs maps each git ref name (e.g. "refs/heads/main") to its current
	// commit SHA. This is the canonical ref state that the object-storage
	// system of record tracks.
	Refs map[string]string `json:"refs"`

	// PackedRefs holds the raw content of the session's packed-refs file,
	// if one exists. Empty string means no packed-refs file is present.
	PackedRefs string `json:"packed_refs"`

	// FencingToken is the high-water mark for writes to this manifest.
	// Any Save attempt with a FencingToken strictly less than the value
	// already on disk is rejected with ErrFenced.
	FencingToken int64 `json:"fencing_token"`

	// UpdatedAt records when this manifest was last written. Set
	// automatically by ManifestStore.Save — callers do not need to set it.
	UpdatedAt time.Time `json:"updated_at"`
}

// PackEntry describes a single pack file (and its companion index) stored in
// object storage for the session.
type PackEntry struct {
	// PackKey is the object-storage key for the .pack file,
	// e.g. "sessions/<id>/packs/<sha>.pack".
	PackKey string `json:"pack_key"`

	// IdxKey is the object-storage key for the companion .idx file,
	// e.g. "sessions/<id>/packs/<sha>.idx".
	IdxKey string `json:"idx_key"`

	// SHA is the git pack-name SHA — the hex string embedded in the pack
	// and idx file names.
	SHA string `json:"sha"`
}

// ManifestStore loads and saves a session's Manifest with conditional-write
// semantics for linearizability.
//
// The caller pattern is read-modify-write:
//
//	m, etag, err := store.Load(ctx, sessionID)
//	// mutate m …
//	newEtag, err := store.Save(ctx, m, etag)
//
// ManifestStore is safe for concurrent use; each Load/Save call is a
// separate Backend round-trip.
type ManifestStore struct {
	Backend Backend
}

// ManifestKey returns the object-storage key for the given session's manifest.
// The key format is "sessions/<sessionID>/manifest.json".
func ManifestKey(sessionID string) string {
	return "sessions/" + sessionID + "/manifest.json"
}

// Load fetches the current manifest for sessionID and its ETag.
//
// If no manifest exists yet (fresh session), Load returns a zero-value
// Manifest, an empty ETag string, and a nil error. Callers should treat this
// case as "clean slate" and proceed with Save(ifMatch="") to create the
// manifest.
//
// If a manifest object exists but does not decode to a readable
// schema-version-1 manifest for sessionID (unknown/zero Version, empty or
// mismatched SessionID, or a body that does not unmarshal), Load returns
// ErrCorruptManifest (wrapped). Callers must fail fast rather than treat a
// corrupt manifest as a clean slate.
//
// Any other Backend error is returned wrapped with context.
func (s *ManifestStore) Load(ctx context.Context, sessionID string) (Manifest, string, error) {
	data, etag, _, err := s.Backend.Get(ctx, ManifestKey(sessionID))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Fresh session — no manifest yet. This is not an error.
			return Manifest{}, "", nil
		}
		return Manifest{}, "", fmt.Errorf("objectstore: load manifest for session %q: %w", sessionID, err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, "", fmt.Errorf("objectstore: unmarshal manifest for session %q: %w", sessionID, err)
	}

	// Validate the decoded manifest. encoding/json is lenient: a body of `null`,
	// `{}`, `{"version":99,...}`, or a manifest whose session_id was tampered
	// decodes without error into a degenerate struct. Returning such a manifest
	// would let hydration treat a corrupt object as an empty/fresh session and
	// silently lose the real history. Reject anything that is not a readable
	// schema-version-1 manifest for the session we asked for.
	if m.Version != manifestSchemaVersion {
		return Manifest{}, "", fmt.Errorf(
			"objectstore: manifest for session %q has unsupported schema version %d (want %d): %w",
			sessionID, m.Version, manifestSchemaVersion, ErrCorruptManifest)
	}
	if m.SessionID != sessionID {
		return Manifest{}, "", fmt.Errorf(
			"objectstore: manifest fetched for session %q carries session_id %q (key/body mismatch): %w",
			sessionID, m.SessionID, ErrCorruptManifest)
	}

	return m, etag, nil
}

// Save writes m to object storage as the session's canonical manifest.
//
// now is the caller's clock value; it is stamped onto m.UpdatedAt. Callers
// should pass their Clock.Now() — e.g. Syncer passes time.Now().UTC() at the
// call boundary since Syncer does not yet carry a Clock field. This follows
// the parameter-passing form of auth.FindOrProvisionAt.
//
// Conditional-write semantics (via ifMatch):
//   - ifMatch = "" creates the manifest; returns ErrPrecondition if one
//     already exists (concurrent writer beat us to the first write).
//   - ifMatch = <etag> overwrites the manifest only if the stored ETag matches;
//     returns ErrPrecondition if it does not (concurrent writer modified it).
//
// Fencing-token pre-flight:
// Before writing, Save loads the current on-disk manifest and compares its
// FencingToken with m.FencingToken. If the on-disk token is strictly greater
// than m.FencingToken, the caller is operating from a stale lease and Save
// returns ErrFenced without issuing the write. This guard catches stale
// lease-holder writes that a pure ETag check would not catch.
//
// Save sets m.UpdatedAt = now and, if m.Version == 0, sets m.Version = 1
// before marshalling. These mutations happen on the local copy only; the
// caller's m is not modified.
//
// On success, Save returns the new ETag assigned by the Backend.
func (s *ManifestStore) Save(ctx context.Context, m Manifest, ifMatch string, now time.Time) (string, error) {
	// Fencing-token pre-flight: load on-disk manifest and compare tokens.
	//
	// We deliberately load directly here rather than trusting the caller's
	// copy, because the caller may have read the manifest long ago and the
	// disk may have been advanced by a newer lease holder in the interim.
	//
	// A missing on-disk manifest means onDiskEtag is "", which also drives
	// the create-only guard below.
	onDisk, onDiskEtag, err := s.Load(ctx, m.SessionID)
	if err != nil {
		return "", fmt.Errorf("objectstore: save manifest pre-flight load for session %q: %w", m.SessionID, err)
	}
	if onDisk.FencingToken > m.FencingToken {
		return "", ErrFenced
	}

	// Create-only guard: when ifMatch="" the caller intends to create a new
	// manifest. If one already exists the write must be rejected with
	// ErrPrecondition — "someone else created it first, re-read and retry."
	//
	// The Backend.Put contract treats ifMatch="" as an unconditional overwrite,
	// so this guard must live here rather than in the Backend.
	if ifMatch == "" && onDiskEtag != "" {
		return "", ErrPrecondition
	}

	// Apply server-side defaults.
	m.UpdatedAt = now
	if m.Version == 0 {
		m.Version = 1
	}

	data, err := json.Marshal(m)
	if err != nil {
		// json.Marshal on a struct with only basic types and time.Time
		// cannot fail in practice, but handle it for correctness.
		return "", fmt.Errorf("objectstore: marshal manifest for session %q: %w", m.SessionID, err)
	}

	etag, err := s.Backend.Put(ctx, ManifestKey(m.SessionID), data, m.FencingToken, ifMatch)
	if err != nil {
		// ErrPrecondition is propagated as-is; callers distinguish it from
		// ErrFenced by errors.Is.
		return "", err
	}

	return etag, nil
}
