package store

// Generic query-wrapping helpers for dialect adapters.
//
// These helpers collapse the mechanical 5-line scalar-return and 7-line
// list-return wrapper boilerplate that appears in sqlite_adapter.go and
// postgres_adapter.go into single-expression calls. They are
// package-private (lowercase) and have no external callers; the sweep that
// introduces those callers is tracked in the parent feature
// feature-refactor-adapter-generic-wrap-helpers.

// wrap1 wraps a one-row dialect query: returns convert(row) on success,
// or the zero value plus mapErr(err) on failure. Used to collapse the
// scalar-return wrapper methods in the dialect adapters from 5 lines to 1.
func wrap1[R any, D any](row R, err error, mapErr func(error) error, convert func(R) D) (D, error) {
	if err != nil {
		var zero D
		return zero, mapErr(err)
	}
	return convert(row), nil
}

// wrapList wraps a multi-row dialect query: returns a slice of convert(row)
// on success, or nil plus mapErr(err) on failure. Used to collapse the
// list-return wrapper methods from 7 lines to 1.
func wrapList[R any, D any](rows []R, err error, mapErr func(error) error, convert func(R) D) ([]D, error) {
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]D, len(rows))
	for i, r := range rows {
		out[i] = convert(r)
	}
	return out, nil
}
