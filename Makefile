.PHONY: server test vet run tidy wasm

server: ## build the pokerd binary
	cd server && go build -o ../bin/pokerd ./cmd/pokerd

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
