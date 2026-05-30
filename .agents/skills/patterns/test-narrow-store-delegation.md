# Pattern: Test narrow-store delegation wrapper

Test files inject typed failures into a single store method by wrapping
a real store with an explicit method-by-method delegation wrapper named
`failing<Verb><Entity>Store`. Every method delegates to `realStore`
except the one method-under-test, which returns the simulated error.
The wrapper satisfies the consumer package's narrow `<pkg>Store`
interface, *not* the umbrella `store.Store`.

## Rationale

The `package-private-composed-store-interface` pattern narrows handler
constructors to 5–10 store methods. Test fixtures that embed
`store.Store` to "satisfy the interface for free" hide which methods
the test actually exercises and let drift creep in (a method gets
removed from the narrow interface but the embedding hides the
breakage). Explicit delegation makes the boundary visible:

- Every store method the consumer actually depends on shows up in the
  wrapper, so reviewers can count the dependency surface at a glance.
- The single overridden method documents exactly which call site the
  test is simulating a failure at.
- If the consumer's narrow interface gains a method, the wrapper fails
  to compile until the test author adds the explicit delegation — no
  silent satisfaction via embedding.

## Examples

### Example 1: accounts — fail `ListOrgsForAccount` only

**File**: `internal/portal/accounts/handlers_test.go:467`
```go
// failingListOrgsStore wraps a real store and returns a transient error from
// ListOrgsForAccount, simulating a DB connection failure.
//
// Implements accountsStore (OrgStore + OrgMemberStore + OrgInviteStore +
// WithTx), delegating all methods except ListOrgsForAccount to the real store.
type failingListOrgsStore struct {
	realStore store.Store
}

// OrgStore delegation
func (f *failingListOrgsStore) CreateOrg(ctx context.Context, p store.CreateOrgParams) (store.Org, error) {
	return f.realStore.CreateOrg(ctx, p)
}
// ... 15 more delegation methods ...

// The one method under test:
func (f *failingListOrgsStore) ListOrgsForAccount(_ context.Context, _ string) ([]store.Org, error) {
	return nil, errors.New("conn refused")
}

// WithTx delegation
func (f *failingListOrgsStore) WithTx(ctx context.Context, fn func(store.TxStore) error) error {
	return f.realStore.WithTx(ctx, fn)
}
```

### Example 2: comments — fail `ListCommentsForSession` only

**File**: `internal/portal/comments/service_test.go:822`
```go
type failingListCommentsStore struct {
	realStore store.Store
}

// CommentStore delegation (ListCommentsForSession overridden below)
func (f *failingListCommentsStore) InsertComment(ctx context.Context, p store.InsertCommentParams) error {
	return f.realStore.InsertComment(ctx, p)
}
func (f *failingListCommentsStore) GetCommentByID(ctx context.Context, id string) (store.Comment, error) {
	return f.realStore.GetCommentByID(ctx, id)
}
func (f *failingListCommentsStore) ResolveComment(ctx context.Context, p store.ResolveCommentParams) error {
	return f.realStore.ResolveComment(ctx, p)
}
func (f *failingListCommentsStore) ListCommentsForSession(_ context.Context, _ store.ListCommentsForSessionParams) ([]store.Comment, error) {
	return nil, errors.New("conn refused")
}
// ... 25 more delegation methods (SessionStore, SessionMemberStore,
//     PlaygroundSessionStore, WithTx) ...
```

### Example 3: finalize — fail `ReleaseFinalizeLock` only

**File**: `internal/portal/finalize/lock_release_test.go:128`
```go
type failingReleaseLockStore struct {
	realStore store.Store
}

// ... explicit delegation for every method in finalizeStore ...

func (f *failingReleaseLockStore) ReleaseFinalizeLock(ctx context.Context, p store.ReleaseFinalizeLockParams) error {
	return errors.New("release lock: conn lost")
}
```

(6 wrappers across 5 files: `failingListOrgsStore`,
`failingGetOrgMemberStore`, `failingListSessionsStore`,
`failingListCommentsStore`, `failingResetIdleTimerStore`,
`failingReleaseLockStore`.)

## When to Use

- Any test that needs to assert handler behaviour when a *specific*
  store method returns an error (e.g. `503 dep.db_unavailable` mapping,
  best-effort warn-and-swallow on `ResetSessionIdleTimer` failure).
- Any test where the narrow consumer interface (`accountsStore`,
  `commentsStore`, `finalizeStore`) needs to be satisfied without the
  full umbrella.

## When NOT to Use

- **`WithTx` callbacks** — failures inside a TX are simulated by
  returning an error from the callback body, not by wrapping the store.
- **Asserting business-logic correctness on the happy path** — use the
  real store unwrapped.
- **Mocking 3+ methods at once** — at that point a per-test mock
  struct is clearer than a wrapper. The pattern's value is naming a
  single failure injection.

## Common Violations

- **`struct { store.Store }` embedding** to "get all the delegation for
  free." This hides the consumer's actual dependency surface and lets
  the test silently survive interface drift. The
  `story-store-partition-test-fixture-sweep` removed all such
  embeddings in v0.4.0; new tests should not reintroduce them.
- **Returning `nil` from delegated methods** instead of delegating to
  `realStore`. The test then exercises a broken happy path and the
  failure assertion becomes meaningless.
- **Naming the wrapper `mockStore` or `fakeStore`** when only one
  method is overridden. `failing<Verb><Entity>Store` (or
  `<adjective><Verb><Entity>Store` for non-failure variants like
  `protectedMutationGuardStore`) signals at the type name exactly
  what the wrapper does.

#### Index entry
- **test-narrow-store-delegation**: Test files inject typed store failures via `failing<Verb><Entity>Store` wrappers that delegate every consumer-interface method to a real store except the single method-under-test; never `struct { store.Store }` embedding.
