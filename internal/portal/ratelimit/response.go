package ratelimit

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"time"
)

// writeTooManyRequests writes a 429 response with the standard httperr envelope
// shape and a Retry-After header. retryAfter is rounded up to the nearest second.
func writeTooManyRequests(w http.ResponseWriter, retryAfter time.Duration) {
	secs := int(math.Ceil(retryAfter.Seconds()))
	if secs < 1 {
		secs = 1
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Retry-After", fmt.Sprintf("%d", secs))
	w.WriteHeader(http.StatusTooManyRequests)

	body := map[string]string{
		"error":   "rate_limited",
		"message": fmt.Sprintf("Too many requests. Retry in %d seconds.", secs),
	}
	_ = json.NewEncoder(w).Encode(body)
}

// clientIP extracts the client IP from r.RemoteAddr (after chi's RealIP
// middleware has already replaced it with the real client IP when behind a
// trusted proxy).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr may already be a bare IP (no port) in some test setups.
		return r.RemoteAddr
	}
	return host
}
