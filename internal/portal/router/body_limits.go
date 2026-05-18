package router

import "net/http"

// BodyLimit returns a middleware that wraps r.Body with http.MaxBytesReader,
// capping the number of bytes the handler may read from the request body.
//
// Apply only to JSON API routes; git smart-HTTP routes have their own per-route
// body limits (see internal/portal/githttp/receive_pack.go).
//
// When a handler (or the JSON decoder it calls) reads past the limit,
// http.MaxBytesReader sets a flag on the ResponseWriter that causes
// net/http to reply 413 Request Entity Too Large automatically.
func BodyLimit(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}
