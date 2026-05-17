---
id: epic-cloud-native-deploy-object-storage-sync-provider-extensions
kind: story
stage: review
tags: [portal, research]
parent: epic-cloud-native-deploy-object-storage-sync
depends_on: [epic-cloud-native-deploy-object-storage-sync-backend]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Object-Storage Sync — Native provider extensions (GCS + Azure)

## Scope

Per the epic-level "research-then-decide" decision: investigate
`cloud.google.com/go/storage` (GCS) and
`github.com/Azure/azure-sdk-for-go/sdk/storage/azblob` (Azure Blob),
compare against thin-REST alternatives, decide per provider, implement.
File research findings BEFORE implementing.

Implements **Unit 4** of `epic-cloud-native-deploy-object-storage-sync`.

## Files

New:
- `docs/research/object-storage-providers.md` — research findings +
  per-provider decision (native SDK vs thin REST client)
- `internal/portal/storage/objectstore/gcs.go` — GCS impl
- `internal/portal/storage/objectstore/gcs_test.go`
- `internal/portal/storage/objectstore/azure.go` — Azure Blob impl
- `internal/portal/storage/objectstore/azure_test.go`

Edit (if Unit 5 has already landed):
- `internal/portal/storage/objectstore/factory.go` — register `gs://`
  and `azblob://` schemes

## Research criteria (record in docs/research/object-storage-providers.md)

For each provider, document:
- SDK dep weight (transitive imports, binary size impact)
- Auth integration (workload identity / managed identity story)
- Conditional-write API ergonomics (must support `IfMatch` or
  `ifGenerationMatch`)
- Object-metadata API (must support fencing-token metadata)
- Decision: native SDK or thin REST client + rationale

## Acceptance criteria

- [ ] `docs/research/object-storage-providers.md` committed FIRST with
  the per-provider decision
- [ ] GCS Backend impl satisfies the `Backend` interface contract from
  story `epic-cloud-native-deploy-object-storage-sync-backend`
- [ ] Azure Blob Backend impl satisfies the same contract
- [ ] Both pass the same integration test suite as the S3 impl
  (re-use the test table; provider-specific test wrappers)
- [ ] Tests gated on `JAMSESH_TEST_GCS_*` / `JAMSESH_TEST_AZURE_*` env
  vars; skip cleanly without
- [ ] Factory registers `gs://` and `azblob://` URL schemes

## Notes

- This story may legitimately defer to a follow-on if scope feels too
  large or the SDK research reveals significant blockers. The S3-compat
  path covers AWS S3, R2, B2, MinIO, Ceph — that's enough for v1
  cloud-native deploy. GCS via S3-compat (HMAC keys instead of workload
  identity) is also possible as a stopgap.
- Document any provider-specific quirks in the research doc — e.g. GCS
  uses `ifGenerationMatch` (int64) instead of ETag string.

## Implementation notes

### Research decisions

**GCS — native SDK (`cloud.google.com/go/storage` v1.62.1):**
- ADC / GKE Workload Identity auth works automatically. No key rotation needed
  in production on GKE.
- Conditional-write API uses `storage.Conditions{GenerationMatch: int64}` and
  `DoesNotExist: true` for create-only. GCS uses int64 generation numbers, not
  ETag strings; bridged by encoding the generation as a decimal string in the
  ETag field. Callers round-trip this value opaquely — no semantic burden.
- Binary size: +~20 MB linked binary growth from gRPC stack. Accepted as a
  trade for workload-identity auth. The `disable_grpc_modules` build tag
  can reduce this in a follow-on if binary size becomes a deployment constraint.
- GCS alternative (S3-compat with HMAC keys) remains valid as a stopgap for
  operators who cannot use Workload Identity.

**Azure Blob — native SDK (`sdk/storage/azblob` v1.7.0 + `azidentity` v1.13.1):**
- DefaultAzureCredential resolves AKS Workload Identity / Managed Identity
  automatically. No key rotation in production.
- ETag-based conditional writes (`IfMatch`/`IfNoneMatch`) match the Backend
  interface exactly — no type bridging needed. Uses `bloberror.HasCode` for
  clean error mapping.
- Binary size: +~5–8 MB (no gRPC).
- Error codes: `ConditionNotMet` → `ErrPrecondition`; `BlobNotFound` →
  `ErrNotFound`; `BlobAlreadyExists` → checked in `PutIdempotent` flow.

### Implementation details

- URL schemes: `gs://bucket/optional-prefix` (GCS), `azblob://account/container/optional-prefix` (Azure)
- `metaKeyFencingToken = "jamsesh-fencing-token"` consistent with S3 impl
- Both `Delete` implementations are idempotent (404 → nil)
- Tests gated on `JAMSESH_TEST_GCS_BUCKET` / `JAMSESH_TEST_AZURE_URL`; skip
  cleanly with descriptive messages when absent
- Factory registration (`gs://`, `azblob://` schemes) deferred to Unit 5
  (`epic-cloud-native-deploy-object-storage-sync-wiring`) — factory.go does
  not exist yet; story body correctly anticipates this
- Full project test suite passes: `go test ./...` all green
