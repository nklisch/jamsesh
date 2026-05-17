.PHONY: generate generate-db generate-api generate-api-go generate-api-ts frontend-build build go-build test-portal-image test-portal-image-clean

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

.PHONY: test-e2e test-e2e-go test-e2e-playwright

# test-e2e-go: run the Go-based e2e test suite.
test-e2e-go:
	cd tests/e2e && go test ./...

# test-e2e-playwright: run Playwright browser tests.
# No-ops cleanly when tests/e2e/playwright/ has not been bootstrapped yet.
test-e2e-playwright:
	@test -d tests/e2e/playwright \
		&& (cd tests/e2e/playwright && npm test) \
		|| echo "playwright not bootstrapped yet, skipping"

# test-e2e: run the full e2e suite (Go then Playwright).
test-e2e: test-e2e-go test-e2e-playwright

# test-portal-image: build the portal Docker image used by e2e Testcontainers
# fixtures. Reuses the project's existing Dockerfile (distroless-static) which
# expects ${BINARY}-${TARGETOS}-${TARGETARCH}. Builds a static linux/amd64
# binary (CGO_ENABLED=0) to satisfy distroless-static's no-libc constraint,
# then removes the intermediate file after docker build.
test-portal-image: frontend-build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o portal-linux-amd64 ./cmd/portal
	docker build --build-arg BINARY=portal --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -t jamsesh/portal:e2e .
	@rm -f portal-linux-amd64

# test-portal-image-clean: remove the e2e portal image tag.
test-portal-image-clean:
	-docker rmi jamsesh/portal:e2e
