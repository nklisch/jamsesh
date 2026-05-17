// Package objectstore provides the Backend interface for object-storage
// operations used by the cloud-native deploy sync pipeline.
//
// In clustered mode, object storage becomes the system of record for bare
// repos. Every write to the lease holder's local bare repo is mirrored to
// object storage, with fencing-token gating so stale writes from a former
// lease holder are rejected.
//
// The fencing token is a monotonically increasing integer issued by the lease
// manager at acquisition time. It is stored as object metadata alongside every
// upload and used by downstream readers to detect stale writers.
package objectstore

import (
	"context"
	"errors"
)

// Backend is the object-storage abstraction for the sync pipeline. All
// implementations must be safe for concurrent use.
//
// Keys are opaque strings; the Backend does not interpret them. The caller is
// responsible for namespace partitioning (e.g. sessions/<id>/objects/...).
//
// The fencingToken parameter on Put and PutIdempotent is stored as object
// metadata (not enforced by the Backend itself). Downstream readers can
// inspect it to detect stale data from a previous lease holder. The object
// store does not validate tokens — that invariant lives in the manifest layer.
type Backend interface {
	// Put writes data to key, optionally requiring a matching ETag.
	//
	// fencingToken is stored in object metadata as "jamsesh-fencing-token"
	// for downstream validation.
	//
	// ifMatch controls conditional-write behaviour:
	//   - "" (empty string): unconditional write (create or overwrite).
	//   - non-empty: the write only succeeds if the object's current ETag
	//     matches ifMatch. On mismatch the operation returns ErrPrecondition.
	//     Pass the ETag from a prior Get or Put call to implement
	//     read-modify-write linearizability.
	//
	// On success, Put returns the new object's ETag (without surrounding
	// quotes). ETag values are opaque; treat them as stable comparison tokens
	// only.
	Put(ctx context.Context, key string, data []byte, fencingToken int64, ifMatch string) (etag string, err error)

	// PutIdempotent writes data to key without a conditional ETag check.
	// It is intended for content-addressed objects (e.g. git's
	// objects/xx/yyyy... files) where a given key always holds the same bytes.
	//
	// Behaviour when the key already exists:
	//   - If the existing content is byte-for-byte identical to data, the
	//     call succeeds (returns nil). The upload was a no-op — the object
	//     is already correct.
	//   - If the existing content differs, the call returns ErrAlreadyExists.
	//     This indicates a programming error: two different objects mapped to
	//     the same key. In practice this cannot happen for git's SHA-addressed
	//     objects; the check is a safeguard.
	//
	// fencingToken is stored in metadata identically to Put.
	PutIdempotent(ctx context.Context, key string, data []byte, fencingToken int64) error

	// Get fetches the object at key.
	//
	// Returns:
	//   - data: the raw bytes stored at key.
	//   - etag: the object's current ETag (without surrounding quotes), suitable
	//     for passing to a subsequent Put call as ifMatch.
	//   - fencingToken: the value stored in the object's "jamsesh-fencing-token"
	//     metadata field, or 0 if absent (objects written before fencing was
	//     introduced, or objects from external writers).
	//   - err: ErrNotFound if the key does not exist; a wrapped error otherwise.
	Get(ctx context.Context, key string) (data []byte, etag string, fencingToken int64, err error)

	// Delete removes key from the store. Idempotent: if the key does not exist,
	// Delete returns nil rather than an error.
	Delete(ctx context.Context, key string) error

	// List enumerates every key whose name begins with prefix. Keys are
	// delivered to fn in lexicographic order. Iteration may be paginated
	// internally; fn is called exactly once per key.
	//
	// If fn returns a non-nil error, List stops iteration immediately and
	// returns that error to the caller. This allows callers to implement
	// early exit (e.g. "stop after the first match").
	//
	// The prefix itself is not included in the keys passed to fn unless it
	// happens to be an exact key name. If keyPrefix is non-empty, the Backend
	// strips it from the reported keys so callers see logical names only.
	List(ctx context.Context, prefix string, fn func(key string) error) error
}

// Sentinel errors returned by Backend implementations. Callers should use
// errors.Is rather than direct equality to accommodate wrapped forms.
var (
	// ErrNotFound is returned by Get when the requested key does not exist.
	ErrNotFound = errors.New("objectstore: not found")

	// ErrPrecondition is returned by Put when the supplied ifMatch ETag does
	// not match the object's current ETag. This indicates a concurrent writer
	// has modified the object since the caller last read it. The caller should
	// re-read the object and retry with the new ETag, or treat the conflict as
	// a fencing event and abort.
	ErrPrecondition = errors.New("objectstore: precondition failed (etag mismatch)")

	// ErrAlreadyExists is returned by PutIdempotent when the key already exists
	// with different content. For git's content-addressed object store this
	// should never occur in production; it signals a programming error in key
	// construction.
	ErrAlreadyExists = errors.New("objectstore: object already exists with different content")
)
