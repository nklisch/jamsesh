.PHONY: generate generate-db generate-api generate-api-go generate-api-ts frontend-build build go-build test-portal-image test-portal-image-clean test-router-image test-router-image-clean

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
# Installs npm deps and Playwright browsers on first run (idempotent).
# No-ops cleanly when tests/e2e/playwright/ has not been bootstrapped yet.
test-e2e-playwright:
	@if [ -d tests/e2e/playwright ]; then \
		cd tests/e2e/playwright \
			&& npm install --silent \
			&& npx playwright install --with-deps chromium \
			&& npx playwright test; \
	else \
		echo "playwright not bootstrapped yet, skipping"; \
	fi

# test-e2e: run the full e2e suite (Go then Playwright).
test-e2e: test-e2e-go test-e2e-playwright

.PHONY: test-fuzz test-fuzz-mcp

# test-fuzz: run fuzz harnesses with a 30s budget per harness.
# For deeper continuous fuzzing, run individually with -fuzztime=10m.
test-fuzz:
	go test -fuzz=FuzzCommitTrailerParse -fuzztime=30s ./internal/portal/prereceive/
	go test -fuzz=FuzzRefNamespaceValidate -fuzztime=30s ./internal/portal/prereceive/
	go test -fuzz=FuzzPathScopeValidate -fuzztime=30s ./internal/portal/prereceive/
	go test -fuzz=FuzzPathScopeEmpty -fuzztime=30s ./internal/portal/prereceive/

# test-fuzz-mcp: property-based fuzz harness for the portal's /mcp endpoint.
# Drives real HTTP POSTs with generated JSON bodies and asserts no 5xx responses.
# Control iteration count via MCP_FUZZ_COUNT (default: 200).
# Reproduce a specific run via MCP_FUZZ_SEED=<seed>.
test-fuzz-mcp:
	cd tests/e2e && go test ./fuzz/ -run TestMCPToolInputFuzz -v -timeout 300s

# test-portal-image: build the portal Docker image used by e2e Testcontainers
# fixtures. Uses the production Dockerfile (alpine:3.21 + git + ca-certificates)
# — the same image production runs — so e2e tests that exercise git smart-HTTP
# (session creation, push, fetch) hit the same git binary path as production.
# The portal binary is built CGO_ENABLED=0 for a fully static executable
# compatible with Alpine's musl libc.
test-portal-image: frontend-build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags e2etest -o portal-linux-amd64 ./cmd/portal
	docker build --build-arg BINARY=portal --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -t jamsesh/portal:e2e .
	@rm -f portal-linux-amd64

# test-portal-image-clean: remove the e2e portal image tag.
test-portal-image-clean:
	-docker rmi jamsesh/portal:e2e

# test-router-image: build the router Docker image used by e2e Testcontainers
# fixtures. The router binary is built CGO_ENABLED=0 for a fully static
# executable compatible with Alpine's musl libc. No git or frontend build
# dependency — the router does not embed any assets.
test-router-image:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o jamsesh-router-linux-amd64 ./cmd/jamsesh-router
	docker build -f Dockerfile.router --build-arg BINARY=jamsesh-router --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -t jamsesh/router:e2e .
	@rm -f jamsesh-router-linux-amd64

# test-router-image-clean: remove the e2e router image tag.
test-router-image-clean:
	-docker rmi jamsesh/router:e2e

.PHONY: test-clean

# test-clean: kill leaked/dangling vitest processes. The vitest-2 default
# `forks` pool spawns child Node processes ("node (vitest N)") that get
# reparented to init and spin (multi-GB RSS, pegged CPU) when `vitest run` is
# hard-killed — terminal close, agent/CI interrupt, or the OOM-killer.
# frontend/vite.config.ts now uses the `threads` pool, which can't orphan that
# way; this stays as a manual safety net. NOTE: this also kills any live
# `vitest`/`npm test` run you have going in another terminal.
test-clean:
	@pkill -TERM -f 'node.*vitest' 2>/dev/null || true
	@sleep 1
	@pkill -KILL -f 'node.*vitest' 2>/dev/null || true
	@echo "cleared dangling vitest processes"

.PHONY: dev dev-down dev-down-v dev-rebuild

# dev: bring up the local development stack via docker compose.
# Builds the dev image on first run; subsequent runs reuse the build cache.
# For hot frontend reload, run `cd frontend && npm run dev` in another terminal.
dev:
	docker compose up

# dev-down: tear down the dev stack. Use `dev-down-v` to also drop .data/.
dev-down:
	docker compose down

dev-down-v:
	docker compose down -v
	rm -rf .data

# dev-rebuild: rebuild the dev image (use after go.mod / Dockerfile.dev edits).
dev-rebuild:
	docker compose up --build
