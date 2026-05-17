// Package probes provides readiness-check primitives for the portal's /readyz
// endpoint. Each check runs with a 2-second timeout; checks run concurrently
// so the total response time equals the slowest individual check, not their sum.
package probes

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Check is a named readiness probe. Name appears in the JSON response body.
// Fn returns nil for healthy, error for unhealthy. Each check runs with a
// 2-second timeout; a check that does not return within the timeout reports
// error: "timeout".
type Check struct {
	Name string
	Fn   func(ctx context.Context) error
}

// checkTimeout is the per-check context deadline.
const checkTimeout = 2 * time.Second

// checkResult carries the outcome of a single Check.
type checkResult struct {
	Name  string  `json:"name"`
	OK    bool    `json:"ok"`
	Error *string `json:"error,omitempty"`
}

// response is the JSON envelope written for all /readyz responses.
type response struct {
	Status string        `json:"status"`
	Checks []checkResult `json:"checks"`
}

// Handler returns an http.Handler that runs every check in parallel and
// responds 200 (all ok) or 503 (any failed) with a JSON body:
//
//	{"status": "ready"|"not_ready",
//	 "checks": [{"name": "...", "ok": true|false, "error": "..."}]}
//
// "error" is omitted on passing checks. A check that exceeds the 2-second
// timeout reports error: "timeout".
func Handler(checks []Check) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		results := make([]checkResult, len(checks))

		var wg sync.WaitGroup
		for i, c := range checks {
			wg.Add(1)
			go func(idx int, check Check) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
				defer cancel()

				err := check.Fn(ctx)
				if err == nil {
					results[idx] = checkResult{Name: check.Name, OK: true}
					return
				}
				// Distinguish a timeout (context deadline exceeded while the
				// check was blocked) from any other error.
				msg := err.Error()
				if ctx.Err() != nil {
					msg = "timeout"
				}
				results[idx] = checkResult{Name: check.Name, OK: false, Error: &msg}
			}(i, c)
		}
		wg.Wait()

		allOK := true
		for _, r := range results {
			if !r.OK {
				allOK = false
				break
			}
		}

		status := "ready"
		httpCode := http.StatusOK
		if !allOK {
			status = "not_ready"
			httpCode = http.StatusServiceUnavailable
		}

		body, _ := json.Marshal(response{Status: status, Checks: results})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(httpCode)
		_, _ = w.Write(body)
	})
}
