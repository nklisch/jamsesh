.PHONY: generate generate-db generate-api generate-api-go generate-api-ts frontend-build build go-build

generate: generate-db generate-api

generate-db:
	sqlc generate

generate-api: generate-api-go generate-api-ts

generate-api-go:
	go generate ./internal/api/openapi/...

generate-api-ts:
	cd frontend && npm install --silent && npm run generate

# frontend-build: install deps, compile the Svelte SPA, and copy the output to
# internal/portal/assets/dist/ where //go:embed all:dist picks it up.
# internal/portal/assets/dist/ ships a .gitkeep so `go build ./...` compiles
# on a fresh checkout before this target has run; the .gitignore in that dir
# keeps the actual build artifacts out of the repo.
frontend-build:
	cd frontend && npm install --silent && npm run build
	rm -rf internal/portal/assets/dist
	mkdir -p internal/portal/assets/dist
	cp -r frontend/dist/. internal/portal/assets/dist/
	touch internal/portal/assets/dist/.gitkeep

# go-build: compile the Go binaries. Depends on frontend-build so the embed
# directive in internal/portal/assets/assets.go finds a populated dist/.
go-build: frontend-build
	go build ./...

# build: full project build — codegen, frontend, Go.
build: generate frontend-build
	go build ./...
