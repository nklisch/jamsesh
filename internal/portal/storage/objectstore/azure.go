package objectstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
)

// AzureBlobConfig configures an Azure Blob Storage Backend.
type AzureBlobConfig struct {
	// URL is the object-storage location in the form azblob://account/container/optional-prefix.
	// The account maps to <account>.blob.core.windows.net; the container is the Azure blob
	// container; the optional path becomes the key prefix applied to every operation.
	URL string

	// AccountKey is the storage account access key. When non-empty, it is used
	// for SharedKeyCredential authentication (suitable for Azurite local dev and
	// environments without Managed Identity).
	//
	// When empty, the SDK uses DefaultAzureCredential, which resolves in order:
	//   1. Environment variables (AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID)
	//   2. Azure Workload Identity (AKS federated identity — preferred for production)
	//   3. Azure Managed Identity (via IMDS)
	//   4. Azure CLI / Azure Developer CLI credentials (local development)
	AccountKey string

	// ServiceURL overrides the default https://<account>.blob.core.windows.net endpoint.
	// Set this for Azurite local testing (e.g. "http://127.0.0.1:10000/devstoreaccount1").
	ServiceURL string
}

// azureBlobBackend is the Azure Blob Storage implementation of Backend.
//
// ETag semantics: Azure Blob returns ETag strings in the form `"<hex>"` (with
// surrounding double-quotes). We strip the quotes before returning to callers
// and re-add them when sending IfMatch headers, because the Azure SDK accepts
// azcore.ETag values which are the raw (quoted) form.
type azureBlobBackend struct {
	containerClient *container.Client
	keyPrefix       string // without trailing slash
}

// NewAzureBlob constructs an Azure Blob Storage Backend from cfg.
//
// See AzureBlobConfig for authentication and URL documentation.
//
// The returned Backend is safe for concurrent use.
func NewAzureBlob(cfg AzureBlobConfig) (Backend, error) {
	accountName, containerName, keyPrefix, err := parseAzureBlobURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("objectstore/azure: invalid URL %q: %w", cfg.URL, err)
	}

	serviceURL := cfg.ServiceURL
	if serviceURL == "" {
		serviceURL = fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)
	}

	containerURL := strings.TrimRight(serviceURL, "/") + "/" + containerName

	var containerClient *container.Client
	if cfg.AccountKey != "" {
		cred, credErr := service.NewSharedKeyCredential(accountName, cfg.AccountKey)
		if credErr != nil {
			return nil, fmt.Errorf("objectstore/azure: shared-key credential: %w", credErr)
		}
		containerClient, err = container.NewClientWithSharedKeyCredential(containerURL, cred, nil)
	} else {
		cred, credErr := azidentity.NewDefaultAzureCredential(nil)
		if credErr != nil {
			return nil, fmt.Errorf("objectstore/azure: default credential: %w", credErr)
		}
		containerClient, err = container.NewClient(containerURL, cred, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("objectstore/azure: create container client: %w", err)
	}

	return &azureBlobBackend{
		containerClient: containerClient,
		keyPrefix:       keyPrefix,
	}, nil
}

// parseAzureBlobURL parses a URL of the form azblob://account/container/optional/prefix
// and returns (account, container, prefix, nil). The prefix has no leading slash.
func parseAzureBlobURL(rawURL string) (account, containerName, prefix string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", err
	}
	if u.Scheme != "azblob" {
		return "", "", "", fmt.Errorf("scheme must be azblob, got %q", u.Scheme)
	}
	account = u.Host
	if account == "" {
		return "", "", "", fmt.Errorf("account name must not be empty")
	}
	// Path is /container/optional/prefix
	path := strings.TrimPrefix(u.Path, "/")
	if path == "" {
		return "", "", "", fmt.Errorf("container name must not be empty (use azblob://account/container[/prefix])")
	}
	parts := strings.SplitN(path, "/", 2)
	containerName = parts[0]
	if len(parts) == 2 {
		prefix = parts[1]
	}
	return account, containerName, prefix, nil
}

// fullKey returns the full blob name for a logical key, prepending the
// configured key prefix when present.
func (b *azureBlobBackend) fullKey(key string) string {
	if b.keyPrefix == "" {
		return key
	}
	return b.keyPrefix + "/" + key
}

// logicalKey strips the key prefix from a full blob name.
func (b *azureBlobBackend) logicalKey(fullKey string) string {
	if b.keyPrefix == "" {
		return fullKey
	}
	prefix := b.keyPrefix + "/"
	return strings.TrimPrefix(fullKey, prefix)
}

// blobClient returns a BlockBlobClient for the logical key.
func (b *azureBlobBackend) blobClient(key string) *blockblob.Client {
	return b.containerClient.NewBlockBlobClient(b.fullKey(key))
}

// stripAzureEtag removes surrounding double-quotes that Azure wraps ETags in.
// Azure returns ETags as `"abc123"` — we store and compare without quotes so
// callers can round-trip without worrying about quoting convention.
func stripAzureEtag(etag *azcore.ETag) string {
	if etag == nil {
		return ""
	}
	return strings.Trim(string(*etag), `"`)
}

// wrapAzureEtag wraps an unquoted ETag string into the quoted form Azure expects.
func wrapAzureEtag(etag string) azcore.ETag {
	if strings.HasPrefix(etag, `"`) {
		return azcore.ETag(etag) // already quoted
	}
	return azcore.ETag(`"` + etag + `"`)
}

// metaValue converts a string to the *string format required by Azure metadata maps.
func metaValue(s string) *string { return &s }

// Put implements Backend.Put.
//
// Azure Blob uses ETag strings for CAS — an exact match to our Backend interface.
// An empty ifMatch performs an unconditional write. A non-empty ifMatch is sent
// as the IfMatch header; Azure returns 412 on mismatch.
func (b *azureBlobBackend) Put(ctx context.Context, key string, data []byte, fencingToken int64, ifMatch string) (string, error) {
	opts := &blockblob.UploadBufferOptions{
		Metadata: map[string]*string{
			metaKeyFencingToken: metaValue(strconv.FormatInt(fencingToken, 10)),
		},
	}
	if ifMatch != "" {
		wrapped := wrapAzureEtag(ifMatch)
		opts.AccessConditions = &blob.AccessConditions{
			ModifiedAccessConditions: &blob.ModifiedAccessConditions{
				IfMatch: &wrapped,
			},
		}
	}

	resp, err := b.blobClient(key).UploadBuffer(ctx, data, opts)
	if err != nil {
		return "", mapAzureError(err)
	}
	return stripAzureEtag(resp.ETag), nil
}

// PutIdempotent implements Backend.PutIdempotent.
//
// Azure supports create-only semantics via IfNoneMatch: "*". On 412 or
// BlobAlreadyExists, we fetch the current content and compare bytes.
// Returns nil if contents match; ErrAlreadyExists if they differ.
func (b *azureBlobBackend) PutIdempotent(ctx context.Context, key string, data []byte, fencingToken int64) error {
	star := azcore.ETag("*")
	opts := &blockblob.UploadBufferOptions{
		Metadata: map[string]*string{
			metaKeyFencingToken: metaValue(strconv.FormatInt(fencingToken, 10)),
		},
		AccessConditions: &blob.AccessConditions{
			ModifiedAccessConditions: &blob.ModifiedAccessConditions{
				IfNoneMatch: &star,
			},
		},
	}

	_, err := b.blobClient(key).UploadBuffer(ctx, data, opts)
	if err == nil {
		return nil // successfully created
	}

	// If the object already exists, compare content.
	if !bloberror.HasCode(err, bloberror.ConditionNotMet, bloberror.BlobAlreadyExists) {
		return mapAzureError(err)
	}

	existing, _, _, getErr := b.Get(ctx, key)
	if getErr != nil {
		return fmt.Errorf("objectstore/azure: PutIdempotent: read existing: %w", getErr)
	}
	if bytes.Equal(existing, data) {
		return nil // idempotent success
	}
	return ErrAlreadyExists
}

// Get implements Backend.Get.
//
// We use DownloadStream to fetch both the body and the response headers
// (ETag, metadata) in a single request.
func (b *azureBlobBackend) Get(ctx context.Context, key string) ([]byte, string, int64, error) {
	resp, err := b.blobClient(key).DownloadStream(ctx, nil)
	if err != nil {
		return nil, "", 0, mapAzureError(err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", 0, fmt.Errorf("objectstore/azure: read body: %w", err)
	}

	etag := stripAzureEtag(resp.ETag)

	var fencingToken int64
	if resp.Metadata != nil {
		if tokenPtr, ok := resp.Metadata[metaKeyFencingToken]; ok && tokenPtr != nil && *tokenPtr != "" {
			fencingToken, err = strconv.ParseInt(*tokenPtr, 10, 64)
			if err != nil {
				fencingToken = 0 // malformed metadata is not fatal
			}
		}
	}

	return data, etag, fencingToken, nil
}

// Delete implements Backend.Delete.
//
// Azure Blob returns a BlobNotFound error when deleting a non-existent blob.
// We map that to nil to satisfy the idempotent contract.
func (b *azureBlobBackend) Delete(ctx context.Context, key string) error {
	_, err := b.blobClient(key).Delete(ctx, nil)
	if err == nil {
		return nil
	}
	if bloberror.HasCode(err, bloberror.BlobNotFound) {
		return nil // idempotent
	}
	return fmt.Errorf("objectstore/azure: delete %q: %w", key, mapAzureError(err))
}

// List implements Backend.List.
//
// Uses NewListBlobsFlatPager to paginate across all blobs with the given prefix.
// Blob names are returned in lexicographic order by Azure, satisfying the
// Backend contract.
func (b *azureBlobBackend) List(ctx context.Context, prefix string, fn func(key string) error) error {
	fullPrefix := b.fullKey(prefix)
	pager := b.containerClient.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{
		Prefix: &fullPrefix,
	})

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("objectstore/azure: list page: %w", mapAzureError(err))
		}
		for _, item := range page.Segment.BlobItems {
			if item.Name == nil {
				continue
			}
			logKey := b.logicalKey(*item.Name)
			if err := fn(logKey); err != nil {
				return err // caller-requested early exit
			}
		}
	}
	return nil
}

// mapAzureError converts Azure SDK errors to objectstore sentinel errors.
func mapAzureError(err error) error {
	if err == nil {
		return nil
	}
	if bloberror.HasCode(err, bloberror.BlobNotFound) {
		return ErrNotFound
	}
	if bloberror.HasCode(err, bloberror.ConditionNotMet) {
		return ErrPrecondition
	}
	return err
}
