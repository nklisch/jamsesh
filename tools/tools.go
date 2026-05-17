//go:build tools

// Package tools pins tool dependencies so `go mod tidy` does not remove them.
// Run `go generate ./internal/api/openapi/...` (or `make generate-api-go`)
// to regenerate server.gen.go.
package tools

import (
	_ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
)
