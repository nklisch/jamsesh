package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// AdvanceClock POSTs to /test/clock-advance and advances the portal's
// process-global clock by the given duration.
//
// The portal must have been built with -tags e2etest (the standard
// `make test-portal-image` target does this). If the portal returns 404
// the test fails with a message that names the make target — the most
// common cause is a stale portal image without the build tag.
//
// The advance is cumulative and forward-only: a second call adds to the
// offset, and the clock cannot be rewound. Because the offset is process-
// global, subtests sharing a portal instance must order themselves
// carefully — see the magic_link_ttl_expiry comment in
// tests/e2e/failure/interrupted_ops_test.go.
func (p *Portal) AdvanceClock(ctx context.Context, t *testing.T, d time.Duration) {
	t.Helper()
	body := fmt.Sprintf(`{"advance_seconds":%d}`, int64(d.Seconds()))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.URL+"/test/clock-advance", strings.NewReader(body))
	if err != nil {
		t.Fatalf("portal.AdvanceClock: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("portal.AdvanceClock: do request: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("portal.AdvanceClock: portal returned 404. " +
			"The portal image must be built with -tags e2etest. " +
			"Run `make test-portal-image` to rebuild jamsesh/portal:e2e.")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("portal.AdvanceClock: POST /test/clock-advance: status %d: %s",
			resp.StatusCode, respBody)
	}

	// Decode and assert shape — fail fast if the endpoint contract drifts.
	var r struct {
		Now           string `json:"now"`
		OffsetSeconds int64  `json:"offset_seconds"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		t.Fatalf("portal.AdvanceClock: decode response: %v (body=%s)", err, respBody)
	}
	if r.Now == "" {
		t.Fatalf("portal.AdvanceClock: response missing now field: %s", respBody)
	}
	t.Logf("portal.AdvanceClock: advanced by %s, new offset=%ds, server now=%s",
		d, r.OffsetSeconds, r.Now)
}
