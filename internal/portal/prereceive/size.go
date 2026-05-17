package prereceive

import "fmt"

// CheckPackSize checks whether the pushed pack exceeds the configured
// maximum. It returns (Rejection, false) when the limit is exceeded and
// (Rejection{}, true) when the pack is within limits.
//
// If maxBytes is 0 or negative, the check is skipped and the function
// always returns (Rejection{}, true).
func CheckPackSize(packBytes int64, maxBytes int64) (Rejection, bool) {
	if maxBytes <= 0 {
		return Rejection{}, true
	}
	if packBytes > maxBytes {
		return Rejection{
			Code: CodeSizeLimit,
			Message: fmt.Sprintf(
				"push pack is too large: %d bytes exceeds the %d-byte limit",
				packBytes, maxBytes,
			),
			Details: map[string]any{
				"pack_bytes": packBytes,
				"max_bytes":  maxBytes,
			},
		}, false
	}
	return Rejection{}, true
}
