.PHONY: server test vet run tidy

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
