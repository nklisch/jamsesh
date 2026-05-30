# Pattern: Package-private composed store interface

Handler / service / worker packages declare a **lowercase, unexported**
`<pkg>Store interface { ... }` in their own package, composed from the
domain sub-interfaces published by `internal/db/store/`. The constructor
accepts that narrow type (`New(s <pkg>Store, ...) *Handler`) rather than
the umbrella `store.Store`. `cmd/portal/main.go` passes the real adapter
(which satisfies the umbrella) and Go's structural typing erases the
difference at the call site.

## Rationale

The umbrella `store.Store` aggregates ~150 methods. Accepting it
everywhere made handlers tautologically depend on the entire data layer
and forced test mocks to stub every method. Partitioning the umbrella
into per-package narrow unions makes each consumer's actual data-layer
dependency visible at its signature, keeps test mocks small (5–10
methods, not 150), and isolates churn: adding a new query to one
sub-interface doesn't ripple into unrelated packages.

The interface lives in the **consumer's** package (`accountsStore` in
`accounts/`, not `store.AccountsStore` in `db/store/`) so a new consumer
can compose whatever union it needs without polluting the producer
package with hyper-specific composites. The producer publishes
single-domain interfaces (`store.CommentStore`, `store.OrgStore`) and
the consumer assembles its union.

`WithTx(ctx, fn func(store.TxStore) error) error` is the lone exception:
it stays in the composed interface because transaction scope is a
separate concern from consumer scope, and `TxStore` is the umbrella
that callbacks need.

## Examples

### Example 1: sessions handler — 9-interface composite

**File**: `internal/portal/sessions/handler.go:35`
```go
// sessionsStore is the minimal store interface consumed by Handler.
type sessionsStore interface {
	store.SessionStore
	store.SessionMemberStore
	store.OrgStore
	store.OrgMemberStore
	store.AccountStore
	store.PlaygroundSessionStore
	store.SessionInviteStore
	store.RefModeStore
	store.EventLogStore
	WithTx(ctx context.Context, fn func(store.TxStore) error) error
}

type Handler struct {
	store     sessionsStore
	// ...
}

func New(s sessionsStore, stor storage.Service, log *events.Log, ...) *Handler {
	return NewWithClock(s, stor, log, ...)
}
```

### Example 2: playground handler — 3-interface composite

**File**: `internal/portal/playground/handler.go:57`
```go
// handlerStore is the minimal store interface consumed by Handler.
type handlerStore interface {
	store.SessionStore
	store.SessionMemberStore
	store.TombstoneStore
	WithTx(ctx context.Context, fn func(store.TxStore) error) error
}

type Handler struct {
	Store   handlerStore
	Tokens  tokens.Service
	// ...
}
```

### Example 3: comments service — 4-interface composite

**File**: `internal/portal/comments/service.go:37`
```go
// commentsStore is the minimal store interface consumed by the comments
// package (both Service and Handler).
type commentsStore interface {
	store.CommentStore
	store.SessionStore
	store.SessionMemberStore
	store.PlaygroundSessionStore
	WithTx(ctx context.Context, fn func(store.TxStore) error) error
}

type Service struct {
	Store commentsStore
	Log   *events.Log
	// ...
}
```

### Example 4: handlerauth — two narrow interfaces side-by-side

**File**: `internal/portal/handlerauth/handlerauth.go:32`
```go
type orgMemberStore interface {
	store.OrgMemberStore
}

type sessionMemberStore interface {
	store.SessionMemberStore
	store.OrgMemberStore
}
```

(20 production packages currently follow this shape: accounts, automerger×2,
auth×3, comments, events, finalize, githttp, handlerauth×2, mcpendpoint,
playground×3, sessions, storage, tokens, wsgateway.)

## When to Use

- Any new portal package (handler, service, worker) that consumes the
  store layer — declare a `<pkg>Store interface` even if it has only
  one sub-interface today, so adding a second domain later doesn't
  change the constructor signature shape.
- Any time a handler's test would need to mock the store: the narrow
  interface keeps the mock 5–10 methods instead of 150.

## When NOT to Use

- **`WithTx` callbacks** — these receive `store.TxStore` (the umbrella)
  because a single transaction can mutate multiple domains.
- **`cmd/portal/main.go` wiring** — the adapter built there is the full
  umbrella; pass it to constructors directly and let Go's structural
  typing match it against each narrow interface.
- **Single-domain consumers with no cross-cutting needs** — if a package
  legitimately uses only `store.OrgStore`, accept `store.OrgStore`
  directly without wrapping it in a one-line composed interface.

## Common Violations

- **Accepting `store.Store` in a constructor** to "avoid the type
  declaration." This re-couples the package to the full data layer and
  forces test mocks back to 150 methods. The `feature-refactor-store-narrow-handler-signatures`
  refactor moved 54 such call sites to narrow interfaces; new code
  should land narrow from day one.
- **Exporting the composed interface** (`SessionsStore` capitalized).
  The composed interface is an implementation detail of the consumer;
  exporting it tempts other packages to depend on it instead of the
  store sub-interfaces they actually need.
- **Embedding `store.Store` in a test fixture** to satisfy the narrow
  interface. Use explicit method-by-method delegation
  (`test-narrow-store-delegation` pattern) so the test's actual
  data-layer dependency stays visible.

#### Index entry
- **package-private-composed-store-interface**: Portal consumer packages declare a lowercase `<pkg>Store interface` composed from `store.*` sub-interfaces (plus `WithTx`) and accept it in their constructor; `cmd/portal/main.go` passes the full adapter and structural typing matches each narrow interface.
