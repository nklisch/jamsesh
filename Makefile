.PHONY: generate generate-db generate-api generate-api-go generate-api-ts

generate: generate-db generate-api

generate-db:
	sqlc generate

generate-api: generate-api-go generate-api-ts

generate-api-go:
	go generate ./internal/api/openapi/...

generate-api-ts:
	cd frontend && npm install --silent && npm run generate
