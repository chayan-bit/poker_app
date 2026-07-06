.PHONY: server test vet run tidy wasm client-build dist docker-build compose-up

# VERSION is stamped into the pokerd binary (main.version) and the container
# image. Falls back to "dev" outside a git checkout.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

server: ## build the pokerd binary (version-stamped)
	cd server && go build -trimpath -ldflags "-X main.version=$(VERSION)" \
		-o ../bin/pokerd ./cmd/pokerd

run: ## run the game server (:8080)
	cd server && go run ./cmd/pokerd

test: ## run all Go tests
	cd server && go test ./...

vet: ## static analysis
	cd server && go vet ./...

tidy: ## sync go.mod/go.sum
	cd server && go mod tidy

docker-build: ## build the pokerd container image
	docker build -t pokerd:local -f Dockerfile server

compose-up: ## start pokerd + postgres via docker compose
	docker compose up --build

# wasm builds the local table core (offline LAN play, issue #27) to WebAssembly
# and copies Go's wasm_exec.js loader beside it. Output lands in client/public so
# Vite serves both as static assets. -trimpath + -s -w keep the binary lean
# (< 6 MB pre-gzip); the localcore dependency graph excludes net/http and pgx.
wasm: ## build client/public/tablecore.wasm + wasm_exec.js
	cd server && GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" \
		-o ../client/public/tablecore.wasm ./cmd/tablewasm
	install -m 0644 "$$(cd server && go env GOROOT)/lib/wasm/wasm_exec.js" client/public/wasm_exec.js

# client-build produces the production web bundle. The WASM core is a required
# runtime asset for offline nearby mode but is gitignored, so it MUST be rebuilt
# before `vite build` on a fresh clone - otherwise the deployed bundle 404s on
# tablecore.wasm and offline mode is dead. Pass VITE_API_URL / VITE_WS_URL in the
# environment to point the bundle at your server (see docs/DEPLOY.md).
client-build: wasm ## build the production web client (includes the WASM core)
	cd client && npm ci && npm run build

dist: server client-build ## build the server binary and the web client together
