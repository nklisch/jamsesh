# Object Storage Provider Research — GCS and Azure Blob

**Date:** 2026-05-17  
**Context:** `epic-cloud-native-deploy-object-storage-sync-provider-extensions`  
**Decision scope:** Should GCS and Azure Blob use their native Go SDKs, or thin hand-rolled REST clients?

---

## Evaluation criteria

For each provider we evaluate:

1. **SDK dependency weight** — transitive module count and binary size impact
2. **Auth integration** — workload/managed identity, credential chain ergonomics
3. **Conditional-write API** — must support `IfMatch`-style preconditions cleanly
4. **Object-metadata API** — must support arbitrary string metadata (for `jamsesh-fencing-token`)
5. **Decision: native SDK vs thin REST client**, with rationale

---

## Provider 1 — Google Cloud Storage (GCS)

### SDK: `cloud.google.com/go/storage` v1.62.1

#### Dependency weight

The GCS Go SDK (`cloud.google.com/go/storage`) is officially maintained by Google and has reached v1.62. However, it carries significant transitive dependency weight:

- **gRPC stack included by default.** The SDK defaults to using the gRPC API for data-plane operations (uploads, downloads) and the JSON API for control-plane operations. The gRPC stack (`google.golang.org/grpc`, `golang.org/x/net`, protocol buffers, googleapis generated code) adds approximately **20–30 MB** to the linked binary. A GitHub issue from the Google Cloud Go repo documents a 20 MB binary size increase between v1.43 and v1.44 caused by a gRPC module import (`storage: binary size increase by 20Mb` — googleapis/google-cloud-go#11448).
- **Mitigation via build tag.** The SDK supports the build tag `disable_grpc_modules` to opt out of gRPC entirely and use the JSON/HTTP API only. This materially reduces binary size but is a non-standard build requirement that would need to be threaded through our CI and release pipeline.
- **Transitive module count.** Without the gRPC opt-out, the module pulls in `google.golang.org/api`, `google.golang.org/grpc`, `go.opencensus.io`, multiple `cloud.google.com/go/*` sub-modules, and large generated proto libraries. This is several megabytes of compiled code even in a trimmed binary.

**Assessment:** Dependency weight is real and significant. The `disable_grpc_modules` tag is a valid mitigation but adds build complexity.

#### Auth integration

The SDK uses `golang.org/x/oauth2/google` and `google.golang.org/api/option` for credentials. The default credential chain (Application Default Credentials, ADC) covers:

- GKE Workload Identity (via the GKE metadata server)
- GCE instance service accounts
- `GOOGLE_APPLICATION_CREDENTIALS` environment variable (service-account JSON key)
- `gcloud auth application-default login` for local development

ADC works automatically on GKE pods with Workload Identity configured — no extra wiring needed beyond `option.WithoutAuthentication()` being absent. For non-Google environments (e.g. running jamsesh outside GKE), a service-account key JSON is required.

**Assessment:** Auth integration is excellent for GKE/GCE environments. ADC is idiomatic and well-supported.

#### Conditional-write API (critical path)

GCS uses **generation numbers** (int64), not ETag strings, for conditional writes. The API:

```go
obj := client.Bucket(bucket).Object(key)
wc := obj.If(storage.Conditions{GenerationMatch: existingGeneration}).NewWriter(ctx)
```

Key points:
- `GenerationMatch: 0` does NOT mean "create if not exists" — it has no effect (zero value is ignored). The documented issue `googleapis/google-cloud-go#1246` and follow-up `#1288` confirm this historical confusion.
- **`DoesNotExist: true`** is the correct field for "create-only" semantics: `storage.Conditions{DoesNotExist: true}`.
- GCS's `NewWriter` returns the object's **generation** (int64) after the write via `ObjectAttrs.Generation`, NOT an ETag string.
- The `Backend.Put` interface uses ETag strings for `ifMatch` and returns a string ETag. **There is a type mismatch**: GCS generation (int64) vs our ETag (string). We bridge this by encoding the generation as a decimal string, which is stable and round-trippable.
- `Get` returns `ObjectAttrs.Generation` (int64) which we encode as the ETag string.
- `Object.Attrs()` returns metadata (custom metadata map) — the `jamsesh-fencing-token` key is stored in `ObjectAttrs.Metadata` (a `map[string]string`), which GCS supports natively.

**Assessment:** Conditional writes are supported and ergonomic. The type bridging (generation int64 ↔ ETag string) is a one-line encode/decode, not a burden.

#### Verdict: **Native SDK — approved, with note on binary size**

**Rationale:**
- The auth story (GKE Workload Identity via ADC) is the primary reason to use native GCS rather than S3-compat HMAC. Rolling a thin REST client would replicate much of what the SDK does for auth, without the flexibility of the credential chain.
- The conditional-write API is clean enough — `DoesNotExist` for create-only, `GenerationMatch` for CAS. The int64↔string bridge is trivial.
- Binary size concern is real but manageable. We do NOT use the `disable_grpc_modules` tag (adds build complexity); instead we accept the ~20 MB binary growth as a reasonable trade for the dependency being an optional feature only compiled into builds that import the objectstore package. In a future optimization pass, the build tag can be added if binary size becomes a deployment constraint.
- **Alternative considered: GCS via S3-compat (HMAC keys).** GCS supports an S3-compatible API via HMAC keys. This would mean zero new dependencies — operators just set `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` pointing at GCS's HMAC endpoint. However, HMAC keys require manual rotation and don't support Workload Identity — defeating the primary value of the GCS native integration for cloud-native deploys on GKE. This remains a valid stopgap (documented in SELF_HOST.md).

---

## Provider 2 — Azure Blob Storage

### SDK: `github.com/Azure/azure-sdk-for-go/sdk/storage/azblob` v1.7.0

#### Dependency weight

The modern Azure SDK for Go uses a modular structure — `sdk/storage/azblob` is a standalone Go module, not the legacy monolithic `github.com/Azure/azure-sdk-for-go`. Key properties:

- **No gRPC.** The Azure SDK for Go is HTTP/REST-only. No gRPC stack, no protobuf generated code. Binary size impact is significantly lower than GCS.
- **Core modules.** The direct dependencies are `github.com/Azure/azure-sdk-for-go/sdk/azcore` (HTTP pipeline, retry, auth interfaces) and optionally `github.com/Azure/azure-sdk-for-go/sdk/azidentity` (Managed Identity, service principal). These are well-scoped submodules.
- **Estimated binary growth:** ~5–8 MB for azblob + azcore + azidentity combined, based on the module sizes and the absence of gRPC. Substantially lighter than the GCS native SDK.
- **v1 is stable.** The azblob module reached v1.0.0 and the API is stable across v1.x.

**Assessment:** Dependency weight is acceptable — lighter than GCS, no gRPC.

#### Auth integration

`github.com/Azure/azure-sdk-for-go/sdk/azidentity` provides a comprehensive credential chain:

- `azidentity.NewDefaultAzureCredential()` — tries, in order: environment variables (`AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_TENANT_ID`), workload identity federation (Azure Workload Identity on AKS), Azure Managed Identity (via IMDS), `az login` credentials for local development.
- Azure Managed Identity on AKS (via Azure Workload Identity or the older Pod Identity) works identically to GKE Workload Identity — no service-account keys needed in production.
- Account Key auth (`azblob.NewSharedKeyCredential`) is supported for local development and non-Azure environments.

**Assessment:** Auth story is solid, mirrors GKE Workload Identity ergonomics on the Azure side.

#### Conditional-write API (critical path)

Azure Blob Storage uses **ETag strings** for conditional writes — the same model as S3. The API:

```go
// blockblob client
client := service.NewClient(url, cred, nil).
    NewContainerClient(container).
    NewBlockBlobClient(key)

etag := azcore.ETag(ifMatch)
_, err = client.Upload(ctx, streamReader, &blockblob.UploadOptions{
    AccessConditions: &blob.AccessConditions{
        ModifiedAccessConditions: &blob.ModifiedAccessConditions{
            IfMatch: &etag,
        },
    },
})
```

Key points:
- ETag-based — **no type mismatch** with our `Backend` interface. Round-trips naturally.
- `IfNoneMatch: azcore.ETag("*")` implements create-only semantics (fail if object already exists), mirroring S3's `IfNoneMatch: "*"`.
- Error codes: 412 Precondition Failed maps to `bloberror.ConditionNotMet`; 409 Conflict / `BlobAlreadyExists` is returned for create-only conflicts on some operations.
- Error handling pattern: `errors.As(err, &respErr)` where `respErr` is `*azcore.ResponseError`; then check `respErr.ErrorCode`.
- Object metadata: arbitrary `map[string]*string` via `blob.SetMetadataOptions` or inline in upload options. The `jamsesh-fencing-token` key maps naturally.
- List: `container.NewListBlobsFlatPager` — paginates, returns `BlobItem` with `Name`.

**Assessment:** ETag-based conditional writes are clean and match our interface exactly. API is ergonomic.

#### Verdict: **Native SDK — approved**

**Rationale:**
- Azure Managed Identity (AKS Workload Identity) is the primary value of native Azure — same reasoning as GCS. No service-account keys in production.
- ETag-based API exactly matches our `Backend` interface — no type bridging needed.
- Binary size is acceptable (~5–8 MB, no gRPC).
- The SDK is stable (v1), maintained, and well-documented.

---

## Summary decision table

| Provider | SDK | Auth | Conditional write | Metadata | Binary impact | Decision |
|---|---|---|---|---|---|---|
| **GCS** | `cloud.google.com/go/storage` v1.62 | ADC / Workload Identity | `DoesNotExist`/`GenerationMatch` (int64, bridged as string) | `map[string]string` | ~20 MB (gRPC) | **Native SDK** |
| **Azure Blob** | `sdk/storage/azblob` v1.7 + `azidentity` | `DefaultAzureCredential` / Managed Identity | ETag `IfMatch`/`IfNoneMatch` (string, native match) | `map[string]*string` | ~5–8 MB | **Native SDK** |

---

## Implementation notes

### GCS-specific

- **ETag bridging:** Store generation (int64) as a decimal string. `Put` returns `strconv.FormatInt(gen, 10)` as the ETag; `Put` with `ifMatch` calls `strconv.ParseInt(ifMatch, 10, 64)` to recover the generation for `GenerationMatch`. A malformed `ifMatch` returns `ErrPrecondition` (conservative — the precondition is unsatisfiable).
- **DoesNotExist for PutIdempotent:** Use `storage.Conditions{DoesNotExist: true}` for the create-only attempt. On 412, fetch and compare bytes. Return `ErrAlreadyExists` if bytes differ.
- **Metadata format:** `ObjectAttrs.Metadata` is `map[string]string`. GCS lowercases custom metadata keys, so `jamsesh-fencing-token` is safe and consistent.
- **Error code:** GCS returns HTTP 412 on precondition failure. The Go SDK wraps this as `googleapi.Error` with `Code: 412`. Use `errors.As` with `*googleapi.Error`.
- **Delete idempotency:** GCS `Object.Delete()` returns a `googleapi.Error` with `Code: 404` if the object doesn't exist. Map this to nil (idempotent).
- **URL scheme:** `gs://bucket/optional-prefix`

### Azure-specific

- **ETag representation:** `azcore.ETag` is a string type alias. Strip surrounding quotes on return (Azure wraps ETags in `"..."` in some responses) — use `strings.Trim(string(etag), `"`)`.
- **PutIdempotent:** Use `IfNoneMatch: azcore.ETag("*")`. On 412 or `BlobAlreadyExists`, fetch and compare. Return `ErrAlreadyExists` if bytes differ.
- **Error detection:** `errors.As(err, &respErr)` with `*azcore.ResponseError`; check `respErr.ErrorCode` (string). Codes: `"ConditionNotMet"` → `ErrPrecondition`; `"BlobNotFound"` → `ErrNotFound`; `"BlobAlreadyExists"` → treat as precondition in PutIdempotent flow.
- **Metadata format:** `map[string]*string` — values must be pointer-to-string. Use a helper.
- **List:** `NewListBlobsFlatPager` with prefix; `BlobItem.Name` is a `*string`.
- **URL scheme:** `azblob://accountname/containername/optional-prefix`

### Deferred items

- **`disable_grpc_modules` build tag for GCS:** Not applied in this implementation. Can be added in a follow-on optimization story if binary size becomes a concern.
- **GCS S3-compat fallback (HMAC keys):** Operators who cannot use Workload Identity can point the S3-compat backend at `https://storage.googleapis.com` with HMAC keys. Document in SELF_HOST.md.
- **Azurite emulator support:** Tests are gated on `JAMSESH_TEST_AZURE_*` env vars. Azurite (the local Azure emulator) can be pointed at via the account-key credential with `AZURE_STORAGE_ACCOUNT_NAME`/`AZURE_STORAGE_ACCOUNT_KEY`. No test container support in this iteration.
