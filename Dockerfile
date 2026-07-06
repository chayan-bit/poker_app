# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# VERSION is stamped into the binary (main.version) so /healthz and logs can
# report exactly which build is running. Defaults to "dev" for local builds.
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/pokerd ./cmd/pokerd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/pokerd /pokerd
USER nonroot:nonroot
EXPOSE 8080
# The distroless static image has no shell or curl, so the container-level
# health of pokerd is asserted by the reverse proxy / orchestrator probing
# GET /healthz (see docs/DEPLOY.md). Postgres has its own compose healthcheck.
ENTRYPOINT ["/pokerd"]
