//go:build e2etest

package testclock

import (
	"encoding/json"
	"net/http"
	"time"
)

// RouteMount returns an http.Handler that serves POST /clock-advance.
// Mount it under /test/ in the portal router so the public surface
// becomes POST /test/clock-advance.
//
// The handler is unauthenticated by design: trust comes exclusively
// from the //go:build e2etest gate. A production binary that does not
// compile this file cannot expose the route.
//
// The returned handler does not match on path — it simply runs the
// advance logic. Path/method matching is the caller's responsibility
// (the portal router registers it on POST /test/clock-advance via
// chi). This keeps the handler reusable across mount strategies and
// avoids the prefix-stripping gotcha when a stdlib ServeMux is
// delegated to from chi.
func RouteMount(clock *AdvanceableClock) http.Handler {
	return http.HandlerFunc(advanceHandler(clock))
}

type advanceRequest struct {
	AdvanceSeconds int64 `json:"advance_seconds"`
}

type advanceResponse struct {
	Now           string `json:"now"`
	OffsetSeconds int64  `json:"offset_seconds"`
}

func advanceHandler(clock *AdvanceableClock) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req advanceRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if req.AdvanceSeconds < 0 {
			http.Error(w, "advance_seconds must be >= 0", http.StatusBadRequest)
			return
		}
		// advance_seconds == 0 is allowed (no-op read of current clock).
		offset := clock.Advance(time.Duration(req.AdvanceSeconds) * time.Second)
		resp := advanceResponse{
			Now:           clock.Now().Format(time.RFC3339Nano),
			OffsetSeconds: int64(offset / time.Second),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
