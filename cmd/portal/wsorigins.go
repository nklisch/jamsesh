package main

import "strings"

// parseAllowOrigins parses the JAMSESH_WS_ALLOW_ORIGINS env var value into
// the AllowOrigins slice accepted by wsgateway.Gateway. The value is a
// comma-separated list of origins; entries are trimmed and empty/blank
// entries are dropped. An empty or all-blank input returns nil (not an
// empty slice) — wsgateway treats nil/empty as deny-all-cross-origin, so
// the distinction is purely for clarity at call sites.
func parseAllowOrigins(v string) []string {
	if v == "" {
		return nil
	}
	var origins []string
	for _, o := range strings.Split(v, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}
