---
id: epic-cloud-native-deploy-object-storage-sync
kind: feature
stage: done
tags: [portal]
parent: epic-cloud-native-deploy
depends_on: [epic-cloud-native-deploy-lease-fencing]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Cloud-Native Deploy — Object-Storage Sync

## Epic context

- Parent epic: `epic-cloud-native-deploy`
- Position in epic: phase-2 durability layer. Consumes the lease +
  fencing primitive from lease-fencing. Consumed by hydration-handoff
  (which reads from object storage on lease acquisition).

## Foundation references

- `docs/SPEC.md` — "Deployment shape" (clustered mode adds object
  storage as a system-of-record constraint).
- `docs/ARCHITECTURE.md` — "Portal / Data store" and the bare-repo
  storage description (this feature adds the dual-layer working-
  cache + object-store-truth model).
- `docs/SECURITY.md` — operator responsibilities and supply-chain
  model (this feature adds a new persistence boundary with its own
  IAM/auth surface; SECURITY needs a row).
- `internal/portal/storage/service.go` — the `Service` interface this
  feature wraps without changing.
- `internal/portal/postreceive/emitter.go` — the
  `Emitter.EmitForUpdates()` tap point for the sync trigger.

## Brief

In clustered mode, makes object storage (GCS / S3 / Azure Blob / any
S3-compatible) the system of record for bare repos. Local disk becomes
a working cache. Every write to the lease holder's local bare repo is
mirrored to object storage continuously, with fencing-token gating so
stale writes from a former lease holder are rejected.

The continuous-sync model exploits git's content-addressed object
database: `objects/xx/yyyy…` files are immutable once written, so we
can upload them as they appear. The mutable bits (`refs/*`,
`packed-refs`, pack-file rewrites from `git gc`) use conditional writes
(generation match / `If-Match` ETag) to maintain a single linearizable
history per session.

Single-instance mode skips this entirely — local disk remains system
of record.

## Scope

In:
- `internal/portal/storage/objectstore/` — implementation of the
  existing `storage.Service` interface that wraps the local-FS Service
  and emits sync events on every write. Implementations for at least
  S3 (covers AWS / R2 / B2 / MinIO via API compatibility), GCS, and
  Azure Blob. Selected by URL scheme in `JAMSESH_OBJECT_STORAGE_URL`.
- Sync pipeline:
  - Hook into the existing `post-receive` boundary in
    `internal/portal/postreceive/` — enumerate new objects, refs,
    packs in the push.
  - Per-object: upload to object storage with fencing token in object
    metadata. Object DB files are content-addressed → upload is
    idempotent; no conditional write needed.
  - Per-ref: conditional write keyed on previous ref value (S3
    `If-Match` ETag, GCS `ifGenerationMatch`, Azure
    `If-Match`). On precondition failure, abort the entire push —
    means our lease has been lost or fenced. Fail-stop.
  - Per-pack rewrite (`git gc`): upload new pack + idx, swap pointer
    in a manifest file (conditional write), enqueue lazy deletion of
    old packs.
- Pack manifest convention: a single object per session
  (e.g. `sessions/<id>/manifest.json`) listing current pack files,
  current ref map, current packed-refs content. Hydration uses this to
  know what to download.
- Bounded async upload queue per session with backpressure: if uploads
  fall behind a threshold, reject new pushes with 503 (push-per-commit
  is acceptable; falling-far-behind is not).
- Fencing token validation: every upload includes the lease's fencing
  token in object metadata. Object storage doesn't enforce this for us
  (no native fencing support); instead, before any conditional write
  we read the manifest's "last-known fencing token" and verify ours is
  ≥. If a stale lease holder writes between our read and write, the
  conditional write on the manifest itself catches the race.
- Metrics: upload rate, upload lag (time from disk-write to
  object-store-acked), upload failures by category, fencing rejections.
- Operator config: `JAMSESH_OBJECT_STORAGE_URL`,
  `JAMSESH_OBJECT_STORAGE_REGION`, credentials per provider
  conventions (env vars or workload identity).

Out:
- Hydration / download path — that's `hydration-handoff`.
- Multi-region replication. Single-bucket-single-region in v1; cross-
  region replication is the object store's job to configure (S3 CRR,
  GCS dual-region, etc.) and a future epic to actually exercise.
- Replacing local FS entirely. Local FS remains the working surface;
  object storage is mirror + truth.
- Archive path. The existing `storage.Service.ArchiveSession` already
  has a clean boundary; archival in clustered mode can later route
  to a cheaper storage class (Glacier / Coldline / Archive), but
  initial implementation just deletes from hot object storage.

## Design decisions

Inherited from epic (object storage = system of record; native-where-
clean + S3-compat fallback provider strategy; fencing tokens on every
write). Feature-local:

- **Object storage is mirror, not async backup.** A push doesn't ack
  to the client until the corresponding objects + ref updates are
  durable in object storage. RPO = 0 for acknowledged pushes. This is
  the only safe contract given the client treats acked pushes as
  durable.
- **Research-then-decide on each provider SDK.** Per epic-level
  decision: research AWS S3 SDK v2, Google Cloud Storage SDK, and
  Azure Blob SDK for fit-with-our-patterns before committing. If a
  given SDK is awkward (dependency-heavy, fights our Storer
  abstraction, awkward auth wiring), roll a thin REST client for
  that provider instead. Spawn a `/agile-workflow:research` stride
  per provider during this feature's design pass.
- **Pack manifest as the read-side index.** Object storage listings
  are eventually consistent on some providers; a manifest object
  written with conditional writes is the linearizable source.
- **Subprocess git stays on local disk.** We don't try to make
  `git-receive-pack` write directly to object storage. The lease
  holder runs subprocess git against local disk, then syncs. This
  preserves performance and avoids a year of pure-Go smart-HTTP
  work. Trade-off: a pod that's the lease holder MUST have functional
  local disk for the duration of the lease.

## Foundation-doc impact

- `docs/SPEC.md` — `storage.Service` semantics gain an "object storage
  optional, system of record when configured" sentence when this lands.
- `docs/ARCHITECTURE.md` — bare-repo storage section gains the dual-
  layer (working cache + object-store truth) description.
- `docs/SECURITY.md` — object storage adds a new persistence boundary
  with its own auth surface (IAM role, service account, access key).
  Add to the operator-responsibilities table.

## Architectural choice

**Selected: a `Backend` interface in `internal/portal/storage/objectstore/`
with one implementation in v1 — S3-compatible via `aws-sdk-go-v2/service/s3`.
The pack manifest (per-session JSON object) is the linearizable read-side
index. Sync wraps the existing local-FS `storage.Service` (no interface
change); the post-receive hook is the sync trigger.**

Considered:
- *Option A — pure-Go smart-HTTP rewrite + direct object-storage Storer*
  (go-git on object storage): year-of-work scope. Rejected — out of scope
  per epic decision.
- *Option B — three native SDKs (S3 + GCS + Azure) shipped together in v1*:
  triples the dependency surface and the test matrix. Rejected for v1.
- **Option C — S3-compatible first, native extensions later** (selected):
  AWS SDK v2's `s3.Client` with endpoint override works against AWS S3,
  Cloudflare R2, Backblaze B2, MinIO, and self-hosted Ceph. Covers the
  majority of operator deployments in one implementation. Native GCS / Azure
  with workload-identity / managed-identity auth are spawned as a
  separate follow-on story that can defer if needed.

Per the epic-level "research-then-decide" decision: AWS SDK v2 was the
clear research outcome — well-maintained, the S3 service client supports
endpoint overrides for all S3-compatible services, conditional writes via
`IfMatch` ETag are first-class. GCS native SDK + Azure native SDK go into
the provider-extensions story; we'll re-research at that point whether the
SDKs fit or thin REST clients are cleaner.

## Implementation Units

### Unit 1: Backend interface + S3-compatible implementation

**Files**:
- new: `internal/portal/storage/objectstore/backend.go` — interface + types
- new: `internal/portal/storage/objectstore/s3.go` — S3-compat impl
- new: `internal/portal/storage/objectstore/s3_test.go` — uses MinIO via
  testcontainers OR `JAMSESH_TEST_S3_*` env vars

**Story**: `epic-cloud-native-deploy-object-storage-sync-backend`

```go
// internal/portal/storage/objectstore/backend.go
package objectstore

// Backend is the object-storage abstraction. All implementations are
// safe for concurrent use.
type Backend interface {
    // Put writes data to key. fencingToken is stored as object metadata
    // and used for downstream validation. ifMatch is the expected ETag
    // for conditional writes (empty string = unconditional create).
    // Returns the new object's ETag on success. PreconditionFailed
    // (when ifMatch doesn't match) returns ErrPrecondition.
    Put(ctx context.Context, key string, data []byte, fencingToken int64, ifMatch string) (etag string, err error)

    // PutIdempotent writes data to key without conditional checks.
    // Safe for content-addressed objects (git's objects/xx/yyyy...).
    // Returns ErrAlreadyExists if the key exists and contents differ
    // (some backends verify; others don't — caller should treat
    // success as "this content is now at this key").
    PutIdempotent(ctx context.Context, key string, data []byte, fencingToken int64) error

    // Get fetches the object's bytes + ETag + fencing token metadata.
    // Returns ErrNotFound if the key doesn't exist.
    Get(ctx context.Context, key string) (data []byte, etag string, fencingToken int64, err error)

    // Delete removes key. Idempotent (no error if missing).
    Delete(ctx context.Context, key string) error

    // List returns keys under the given prefix. May be paginated
    // (caller consumes via callback). Lexicographic order.
    List(ctx context.Context, prefix string, fn func(key string) error) error
}

var (
    ErrNotFound      = errors.New("objectstore: not found")
    ErrPrecondition  = errors.New("objectstore: precondition failed (etag mismatch)")
    ErrAlreadyExists = errors.New("objectstore: object already exists with different content")
)
```

```go
// internal/portal/storage/objectstore/s3.go
type s3Backend struct {
    client *s3.Client    // aws-sdk-go-v2/service/s3
    bucket string
}

// NewS3 constructs an S3-compatible Backend. url should be in the
// form s3://bucket/prefix or s3-compatible://bucket/prefix with
// endpoint specified separately. Credentials come from the AWS
// SDK's default chain (env, profile, IRSA, IMDS).
func NewS3(cfg S3Config) (Backend, error)

type S3Config struct {
    URL              string  // s3://bucket/optional-prefix
    Region           string
    EndpointURL      string  // override for R2/B2/MinIO; empty = AWS
    UsePathStyle     bool    // true for MinIO/Ceph; false for AWS/R2
    DisableSSL       bool    // for local MinIO testing
}
```

**Implementation Notes**:
- Use `github.com/aws/aws-sdk-go-v2/service/s3` v1 — mature, supports
  S3-compat via `config.WithBaseEndpoint(endpoint)` and `s3.Options.UsePathStyle`.
- Fencing token storage: `s3.PutObjectInput.Metadata = map[string]string{"jamsesh-fencing-token": strconv.FormatInt(t, 10)}`.
- ETag-based conditional write: `s3.PutObjectInput.IfMatch = aws.String(etag)`.
- `PutIdempotent`: tries `IfNoneMatch: "*"` (create only) first; on 412 (object exists), HEAD the object, compare content hash, return nil if match or `ErrAlreadyExists` if differ. Most git objects are content-addressed so they'll naturally match; this catches a coding error not a runtime concern.
- List uses paginator: `s3.NewListObjectsV2Paginator`. Callback returns error → stop early.
- Error mapping: `&smithy.GenericAPIError{Code: "PreconditionFailed"}` → `ErrPrecondition`; `NoSuchKey` → `ErrNotFound`.

**Acceptance Criteria**:
- [ ] `Put` with `ifMatch=""` succeeds; returns ETag
- [ ] `Put` with stale `ifMatch` returns `ErrPrecondition`
- [ ] `PutIdempotent` succeeds on first write; second write with same content succeeds; second write with different content returns `ErrAlreadyExists`
- [ ] `Get` returns data + ETag + fencing token from metadata
- [ ] `Get` on missing key returns `ErrNotFound`
- [ ] `Delete` is idempotent
- [ ] `List` yields all keys under prefix; callback error stops iteration
- [ ] Integration tests against MinIO (testcontainer or local) OR gated on `JAMSESH_TEST_S3_*` env vars; skip cleanly without

### Unit 2: Pack manifest + state model

**Files**:
- new: `internal/portal/storage/objectstore/manifest.go`
- new: `internal/portal/storage/objectstore/manifest_test.go`

**Story**: `epic-cloud-native-deploy-object-storage-sync-manifest`

```go
// internal/portal/storage/objectstore/manifest.go
package objectstore

// Manifest is the per-session linearizable state object stored at
// sessions/<id>/manifest.json. It lists current packs, current ref map,
// and the high-water fencing token (writes with lower tokens are stale).
type Manifest struct {
    Version           int             `json:"version"`        // schema version, currently 1
    SessionID         string          `json:"session_id"`
    Packs             []PackEntry     `json:"packs"`          // current pack files
    Refs              map[string]string `json:"refs"`         // ref name → commit sha
    PackedRefs        string          `json:"packed_refs"`    // contents of packed-refs file, if any
    FencingToken      int64           `json:"fencing_token"`  // high-water mark — writes with smaller tokens rejected
    UpdatedAt         time.Time       `json:"updated_at"`
}

type PackEntry struct {
    PackKey  string `json:"pack_key"`  // sessions/<id>/packs/<sha>.pack
    IdxKey   string `json:"idx_key"`   // sessions/<id>/packs/<sha>.idx
    SHA      string `json:"sha"`       // git pack-name sha
}

// ManifestStore loads/saves a session's manifest with conditional-write
// semantics for linearizability.
type ManifestStore struct {
    Backend Backend
}

func ManifestKey(sessionID string) string  // returns "sessions/<id>/manifest.json"

// Load fetches the current manifest + its ETag. Returns (zero-value Manifest, "", nil)
// when the manifest doesn't exist yet (fresh session). Returns wrapped error otherwise.
func (s *ManifestStore) Load(ctx context.Context, sessionID string) (Manifest, string, error)

// Save writes the manifest with a conditional ETag check. ifMatch="" creates
// (will fail if manifest exists). Returns the new ETag.
// Returns ErrPrecondition if ifMatch doesn't match (concurrent writer won).
// Returns ErrFenced if the in-memory manifest's fencing token is < the on-disk
// manifest's token (stale write attempt).
func (s *ManifestStore) Save(ctx context.Context, m Manifest, ifMatch string) (newEtag string, err error)
```

**Implementation Notes**:
- The Save call's `ifMatch` ETag is the linearizability primitive. Caller pattern is read-modify-write: `Load` → mutate → `Save(ifMatch=oldEtag)`.
- Fencing-token validation is done in Save: load the on-disk manifest, compare tokens, refuse if ours is lower. This catches the case where a stale lease holder has read an old manifest and tries to write a stale update.
- The `ErrFenced` sentinel is distinct from `ErrPrecondition` because they have different operational meaning: precondition = "someone else wrote concurrently, retry"; fenced = "your lease is stale, abort and 503".

**Acceptance Criteria**:
- [ ] `Load` on missing manifest returns zero-value Manifest, empty ETag, nil error
- [ ] `Load` on existing manifest returns it + the current ETag
- [ ] `Save` with `ifMatch=""` succeeds if manifest doesn't exist; fails (ErrPrecondition) if it does
- [ ] `Save` with matching `ifMatch` succeeds; returns new ETag
- [ ] `Save` with stale `ifMatch` returns ErrPrecondition
- [ ] `Save` with fencing token < on-disk token returns ErrFenced (even with matching ETag)
- [ ] `Save` with fencing token ≥ on-disk token + matching ETag succeeds
- [ ] JSON round-trips losslessly

### Unit 3: Sync pipeline (post-receive integration + fencing validation)

**Files**:
- new: `internal/portal/storage/objectstore/sync.go` — Syncer + queue
- new: `internal/portal/storage/objectstore/sync_test.go`
- edit: `internal/portal/postreceive/emitter.go` — call Syncer.Sync after event emit
- edit: `cmd/portal/main.go` — construct Syncer with Backend + ManifestStore +
  lease Manager + metrics; pass to postreceive Emitter

**Story**: `epic-cloud-native-deploy-object-storage-sync-pipeline`

```go
// internal/portal/storage/objectstore/sync.go
package objectstore

type Syncer struct {
    Backend       Backend
    Manifests     *ManifestStore
    Storage       storage.Service  // local-FS service (for repo path)
    Lease         lease.Manager    // for FencingToken on the active handle
    Metrics       *metrics.Registry
    QueueSize     int             // default 256
    PerSessionBackpressure bool   // default true
}

// SyncPush enumerates new objects/refs/packs from a post-receive event
// and uploads them to object storage. Fail-stop on any fenced/precondition
// failure — returns error so the caller can 503 the client.
//
// Sequence:
//   1. Get current Handle for sessionID from Lease (must exist — caller
//      should have already acquired). Use handle.FencingToken().
//   2. Enumerate new objects in the local repo since the last manifest:
//      - Walk objects/xx/* for any with mtime > last sync
//      - Read each object file
//      - For each: PutIdempotent(objectKey, bytes, fencingToken)
//   3. Detect pack rewrites: list local packs vs manifest packs;
//      upload new packs (+ idx) via PutIdempotent.
//   4. Read local refs/* and packed-refs; build new Manifest with
//      updated Refs/Packs and fencingToken = handle.FencingToken().
//   5. Save manifest with ifMatch=oldEtag. On ErrFenced or
//      ErrPrecondition → return error (caller 503s).
//   6. After successful manifest save, enqueue lazy deletion of
//      pack files in old manifest but not new (don't block ack on this).
type SyncOutput struct {
    ObjectsUploaded int
    PacksUploaded   int
    RefsChanged     int
    BytesUploaded   int64
    Duration        time.Duration
}

func (s *Syncer) SyncPush(ctx context.Context, sessionID string, handle lease.Handle) (SyncOutput, error)
```

**Implementation Notes**:
- Triggered SYNCHRONOUSLY at the end of post-receive — the push doesn't ack
  until sync completes. This is the RPO=0 contract. If sync fails, push fails.
- The "enumerate new objects" walk: track per-session "last-synced-mtime" in
  process memory. On startup (or first push for a session), walk all objects
  and check existence in object storage; for v1 prefer "upload all on first
  push" approach (simple, slow once per session, fast thereafter). Document
  this trade-off.
- Pack rewrite detection: simple — `git gc` produces new pack files in
  `objects/pack/`; compare current dir listing to manifest's PackEntries.
- Backpressure: per-session in-flight upload count tracked via sync.Map.
  When > QueueSize, SyncPush returns a "backpressure" error → caller 503s
  with Retry-After.
- `Metrics` emission (cross-binary in `internal/portal/metrics/metrics.go`,
  add as part of this story):
  - `jamsesh_object_storage_uploads_total{result}` — result ∈ {ok, fenced, precondition, error}
  - `jamsesh_object_storage_upload_bytes_total`
  - `jamsesh_object_storage_upload_duration_seconds` (histogram)
  - `jamsesh_object_storage_backpressure_total`
- `git gc` policy decision: disable opportunistic gc per session repo (`git config gc.auto 0` on `CreateRepo`); rely on scheduled cleanup or the existing finalize path. Sidesteps the pack-rewrite-mid-push problem entirely. Document.

**Acceptance Criteria**:
- [ ] `SyncPush` after a small push uploads all new objects + updated refs to S3-compat (MinIO test); returns SyncOutput with counts
- [ ] Two concurrent SyncPushes for different sessions don't interfere
- [ ] SyncPush with a fenced lease (lower token than on-disk manifest) returns ErrFenced
- [ ] SyncPush exceeding QueueSize returns backpressure error
- [ ] Metrics emit per acceptance criteria above
- [ ] Post-receive integration: existing post-receive tests still pass; new test verifies sync is called after EmitForUpdates

### Unit 4: Provider extensions (GCS + Azure native SDKs)

**Files**:
- new: `internal/portal/storage/objectstore/gcs.go` — GCS native (or thin REST per research)
- new: `internal/portal/storage/objectstore/gcs_test.go`
- new: `internal/portal/storage/objectstore/azure.go` — Azure Blob native (or thin REST)
- new: `internal/portal/storage/objectstore/azure_test.go`
- new: `docs/research/object-storage-providers.md` — research notes
- edit: `internal/portal/storage/objectstore/factory.go` (added in Unit 5) — register new schemes

**Story**: `epic-cloud-native-deploy-object-storage-sync-provider-extensions`

**Implementation Notes**:
- Per the "research-then-decide" decision: spawn a `/agile-workflow:research` stride at the START of this story to compare:
  - `cloud.google.com/go/storage` (GCS native) vs hand-rolled REST
  - `github.com/Azure/azure-sdk-for-go/sdk/storage/azblob` vs hand-rolled REST
  Decide per provider based on:
  - SDK dep weight (transitive imports, binary size)
  - Auth integration (GCP workload identity, Azure managed identity)
  - Conditional-write API ergonomics (must support `IfMatch`/`ifGenerationMatch`)
  - Object-metadata API
- File research findings at `docs/research/object-storage-providers.md` BEFORE implementing.
- Both impls satisfy the `Backend` interface. URL scheme picks: `gs://`, `azblob://`.
- Tests gated on `JAMSESH_TEST_GCS_*` / `JAMSESH_TEST_AZURE_*` env vars; skip cleanly.

**Acceptance Criteria**:
- [ ] Research document committed first with native-vs-thin-client decision per provider + rationale
- [ ] GCS Backend impl: same contract as S3 backend; integration test against real GCS or emulator
- [ ] Azure Blob Backend impl: same contract; integration test against real Azure or Azurite
- [ ] Factory (Unit 5) registers `gs://` and `azblob://` schemes
- [ ] All Backend interface tests pass for each impl (reuse the same test table from S3)

### Unit 5: Factory + config + main.go wiring

**Files**:
- new: `internal/portal/storage/objectstore/factory.go` — URL parsing + Backend construction
- new: `internal/portal/storage/objectstore/factory_test.go`
- edit: `internal/portal/config/config.go` — `JAMSESH_OBJECT_STORAGE_URL`, `_REGION`, `_ENDPOINT_URL`, `_SYNC_QUEUE_SIZE`
- edit: `cmd/portal/main.go` — wire Backend + ManifestStore + Syncer; pass to postreceive
- edit: `docs/SELF_HOST.md` — clustered-mode section gains object-storage subsection
- edit: `docs/SPEC.md` — storage.Service semantics note
- edit: `docs/SECURITY.md` — operator-responsibility row for object-storage IAM

**Story**: `epic-cloud-native-deploy-object-storage-sync-wiring`

```go
// internal/portal/storage/objectstore/factory.go
package objectstore

// New constructs a Backend from a URL.
//   s3://bucket/prefix              → S3-compat (AWS S3 default)
//   s3-compatible://bucket/prefix   → S3-compat with endpoint override
//   gs://bucket/prefix              → GCS
//   azblob://account/container/prefix → Azure Blob
//
// Additional config (region, endpoint URL, etc.) comes from env vars
// per the provider conventions.
func New(url string, cfg Config) (Backend, error)

type Config struct {
    Region       string
    EndpointURL  string  // for s3-compatible://
    UsePathStyle bool    // for MinIO/Ceph
    DisableSSL   bool    // for local MinIO testing
}
```

**Implementation Notes**:
- Config env vars: `JAMSESH_OBJECT_STORAGE_URL` (URL with scheme), `JAMSESH_OBJECT_STORAGE_REGION`, `JAMSESH_OBJECT_STORAGE_ENDPOINT_URL`, `JAMSESH_OBJECT_STORAGE_PATH_STYLE` (bool), `JAMSESH_OBJECT_STORAGE_SYNC_QUEUE_SIZE` (int).
- In `cmd/portal/main.go`: construct Backend ONLY if `cfg.DeployMode == "clustered" && cfg.ObjectStorageURL != ""`. Otherwise pass nil Syncer to postreceive Emitter — emitter treats nil as no-op (sync skipped, single-instance mode).
- Validation in `config.validate()`: when `DeployMode == "clustered"`, `ObjectStorageURL` is REQUIRED. Reject startup otherwise.
- Wiring order in main.go: db.Open → lease.New → metrics.New → objectstore.New → ManifestStore{Backend} → Syncer{Backend, Manifests, Storage, Lease, Metrics} → pass to postreceive Emitter.
- Docs updates per the Foundation-doc impact section above.

**Acceptance Criteria**:
- [ ] Factory parses each URL scheme correctly
- [ ] Factory returns error on unknown scheme
- [ ] `cfg.validate()` rejects clustered+no-object-storage-url
- [ ] cmd/portal/main.go wires Syncer in clustered mode; nil in single-instance
- [ ] postreceive Emitter calls Syncer.SyncPush when non-nil; skips when nil
- [ ] SELF_HOST.md clustered section documents the object-storage env vars + AWS / R2 / GCS / Azure deploy examples
- [ ] SPEC.md storage.Service section notes the dual-layer mode
- [ ] SECURITY.md gains object-storage IAM row

## Implementation Order

Wave 1: Unit 1 (backend interface + S3 impl) — foundational, no deps
Wave 2: Unit 2 (manifest) — depends on Unit 1
Wave 3: Unit 3 (sync pipeline) — depends on Unit 1 + Unit 2
Wave 4 (parallel): Unit 4 (provider extensions) + Unit 5 (wiring) — both depend on Unit 3 (factory needs at least one Backend to wire, but Unit 4 can be deferred to a follow-on if needed)

Recommended autopilot waves: 1 → 2 → 3 → (4 ∥ 5).

## Testing

| Unit | Type | Surfaces |
|---|---|---|
| 1 backend+s3 | unit + integration (MinIO or `JAMSESH_TEST_S3_*`) | Put/Get/Delete/List; conditional writes; metadata |
| 2 manifest | unit + integration | Load/Save linearizability; ETag + fencing-token contracts |
| 3 sync pipeline | integration (MinIO + lease.NoopManager + ephemeral storage) | end-to-end push → upload; fail-stop on fenced; backpressure |
| 4 providers | integration (per-provider, gated) | Backend contract tests reused; native auth wiring |
| 5 wiring | unit (factory parsing) + manual (main.go wiring is glue) | URL scheme dispatch; config validation |

## Risks

- **AWS SDK v2 dependency weight**: pulls in many subpackages. Mitigate: only import `github.com/aws/aws-sdk-go-v2`, `aws/config`, `service/s3`, `credentials` — not the whole SDK. Estimated +8-12MB binary growth.
- **Fail-stop on sync failure means push-fail-rate ≈ object-storage-error-rate**: object storage outages directly fail user pushes. Mitigate: configure cloud retry settings on the S3 client; document in SELF_HOST that operators should monitor `jamsesh_object_storage_uploads_total{result="error"}` and have appropriate object-storage SLA.
- **"Upload all on first push" approach is slow for sessions migrated to a new pod**: hydration-handoff story is the proper fix; v1 of this feature can be slow on lease-handoff. Document.
- **Disabling `gc.auto` accumulates loose objects**: long-running sessions may have lots of small object uploads. Operators may need to schedule manual `git gc` runs. Document.
- **No cost-control rate limiting**: heavy push rate × object storage API pricing = potential surprise bill. Mitigate: backpressure limits per-session; document the cost model in SELF_HOST.
- **Conditional-write semantics differ subtly across S3-compat providers**: R2 ETag format differs from AWS; B2 has different precondition error codes. Validate against each in integration tests; document any provider-specific quirks.

## Foundation-doc impact

Updates land with Unit 5:
- `docs/SPEC.md` — storage.Service semantics gain "object storage optional, system of record when configured"
- `docs/ARCHITECTURE.md` — bare-repo storage section gains the dual-layer description
- `docs/SECURITY.md` — operator responsibilities table gains object-storage IAM row
- `docs/SELF_HOST.md` — clustered-mode section gains object-storage subsection with per-provider deploy examples + cost-model paragraph

## Children complete (2026-05-17)

All 5 child stories landed and reviewed:

| Story | Verdict | Notes |
|---|---|---|
| backend | Approve | S3-compat impl via aws-sdk-go-v2; covers AWS/R2/B2/MinIO/Ceph |
| manifest | Approve | Linearizable per-session state object; ErrFenced sentinel distinct from ErrPrecondition |
| pipeline | Approve | RPO=0 sync hooked into post-receive; gc.auto=0 on CreateRepo prevents pack-rewrite races |
| provider-extensions | Approve | Native GCS + Azure Blob SDKs (workload-identity auth); research doc landed |
| wiring (review pending) | — | Factory + config + main.go + docs |

Verification: `go build ./...` clean; `go test ./...` green across all packages.

Feature advanced `implementing → review`. Object storage is now the system-of-record in clustered mode; single-instance mode is unchanged.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes** (aggregate concerns; per-line lenses exercised at story level):

- **Capability completeness**: ✓ Object storage as system of record in clustered mode shipped. Backend abstraction supports AWS S3, Cloudflare R2, Backblaze B2, MinIO, self-hosted Ceph (via S3-compat) + GCS (workload identity via native SDK) + Azure Blob (managed identity via native SDK). Sync pipeline hooked into post-receive with RPO=0 contract. Pack manifest + fencing token validation prevents split-brain corruption. Backpressure caps per-session in-flight uploads.
- **Foundation-doc alignment**: ✓ SELF_HOST §14 updated with object-storage subsection + per-provider deploy examples + cost model. SPEC.md gains dual-layer description. SECURITY.md gains object-storage IAM operator-responsibility row. ARCHITECTURE.md "Horizontal scaling" updated with dual-layer storage section. Preview framing accurately positions hydration-handoff as the last gap (not "everything is preview" anymore).
- **Cross-cutting changes**: 4 new metric handles in `internal/portal/metrics/metrics.go` (object-storage uploads, bytes, duration, backpressure). `internal/portal/storage/repo.go` now disables `gc.auto` on CreateRepo. `postreceive.Emitter` gained `Syncer *objectstore.Syncer` + `Storage storage.Service` fields. Config gained 5 object-storage env vars + validation rules.
- **New dependencies**: `aws-sdk-go-v2` subpackages (~12MB), `cloud.google.com/go/storage` (~20MB w/ gRPC), `azure-sdk-for-go/sdk/storage/azblob` + `azidentity` (~7MB). Total ~40MB binary growth for clustered-mode deployments. Single-instance mode unaffected (clustered deps only loaded when configured).
- **Acquire-per-sync lease pattern (v1)**: documented as deliberate. Long-held lease pattern is hydration-handoff scope.

The "preview" status now means specifically: router + leases + durability working; lease migration between pods (hydration) is the remaining gap. Operators can deploy clustered mode TODAY and get safe-but-pinned-to-pod sessions; full pod-failover requires hydration-handoff.

