# Deploying pokerd

This document describes the one-VM deployment recipe for pokerd and the poker_app client.
It covers the Docker image, docker-compose stack, TLS/WebSocket termination with Caddy, the full environment variable reference, and static hosting of the client build.

## Overview

pokerd is a single static Go binary that serves the game engine over HTTP and WebSockets.
Postgres is optional: without `POKERD_DATABASE_URL` set, pokerd falls back to in-memory stores that reset on every restart.
The client is a static Vite build that talks to pokerd over HTTP and WebSocket; it can be hosted on any static file host, including the same VM behind Caddy.

## One-VM recipe

This recipe assumes a single Linux VM with Docker and Docker Compose installed, and a DNS record pointing at the VM's public IP.

### 1. Clone the repo and configure environment

```
git clone <repo-url> poker_app
cd poker_app
cp .env.example .env
```

Edit `.env` and set real values for `POSTGRES_PASSWORD`, `POKERD_DATABASE_URL`, `POKERD_AUTH_SECRET`, and `POKERD_ALLOWED_ORIGINS`.
Generate a strong auth secret with `openssl rand -hex 32`.
Set `POKERD_ALLOWED_ORIGINS` to the exact origin(s) the client is served from, for example `https://poker.example.com`.

### 2. Build and start the server stack

```
docker compose up -d --build
```

This starts the `postgres` service (with a named volume for data persistence across restarts) and the `pokerd` service, which waits for Postgres to report healthy before starting.
Check status with `docker compose ps` and logs with `docker compose logs -f pokerd`.

### 3. Build the client

Build from the repo root with `make client-build`, NOT a bare `npm run build`:

```
VITE_API_URL=https://poker.example.com VITE_WS_URL=wss://poker.example.com/ws \
	make client-build
```

`make client-build` runs `make wasm` first, then `npm ci && npm run build`.
This matters: the offline "Play Nearby" mode loads `client/public/tablecore.wasm` at runtime, but that file is gitignored and only produced by `make wasm`.
A fresh clone that runs a bare `npm run build` ships a bundle that 404s on `tablecore.wasm`, silently breaking offline mode.

This produces a static build in `client/dist/` (including `tablecore.wasm` and `wasm_exec.js`).
Copy `client/dist/` to the VM, for example to `/srv/poker-client`.

`VITE_API_URL` and `VITE_WS_URL` are inlined at build time. If you omit them the web build falls back to same-origin, which works behind the Caddy recipe below. The packaged mobile builds (iOS/Android) MUST have them set, since there is no same-origin server inside the app shell - see `client/MOBILE.md`.

### 4. Terminate TLS and proxy with Caddy

Install Caddy on the VM and use a Caddyfile like the one below.
Caddy obtains and renews TLS certificates automatically and proxies both plain HTTP requests and the WebSocket upgrade to pokerd on `127.0.0.1:8080`.

```
poker.example.com {
	# Static client build.
	root * /srv/poker-client
	file_server
	try_files {path} /index.html

	# API and WebSocket traffic goes to pokerd.
	handle /api/* {
		reverse_proxy 127.0.0.1:8080
	}
	handle /ws {
		reverse_proxy 127.0.0.1:8080
	}
	handle /healthz {
		reverse_proxy 127.0.0.1:8080
	}
}
```

Caddy proxies the `Upgrade`/`Connection` headers for WebSocket connections automatically; no extra configuration is needed for `/ws` to work over `wss://`.
Reload Caddy after any Caddyfile change with `caddy reload`.

### 5. Verify

Visit `https://poker.example.com` and confirm the client loads and connects.
Play a hand, restart the `pokerd` container with `docker compose restart pokerd`, and confirm balances and hand history persist across the restart (this only holds when `POKERD_DATABASE_URL` is set; without it, state is in-memory and resets on restart by design).

## Environment variable reference

| Variable | Used by | Required | Default | Notes |
|---|---|---|---|---|
| `PORT` | not read by pokerd directly | no | n/a | pokerd does not read `PORT`; use `POKERD_ADDR` instead. Listed here because it is a common convention on some PaaS platforms. |
| `POKERD_ADDR` | pokerd | no | `:8080` | Listen address, for example `:8080` or `0.0.0.0:8080`. |
| `POKERD_DATABASE_URL` | pokerd | no | unset | Postgres connection string, for example `postgres://user:pass@host:5432/dbname?sslmode=disable`. When unset, pokerd uses in-memory stores and logs a warning. |
| `POKERD_AUTH_SECRET` | pokerd | recommended | random ephemeral value | HMAC secret used to sign auth tokens. When unset, pokerd generates a random secret at startup and logs a warning; every restart invalidates all existing sessions. Always set this in production. |
| `POKERD_ENV` | pokerd | recommended | unset (dev) | Set to `production` to make missing `POKERD_DATABASE_URL` or `POKERD_AUTH_SECRET` a hard startup failure instead of a warning-plus-fallback. The compose stack sets this for you. |
| `POKERD_ALLOWED_ORIGINS` | pokerd | recommended | localhost/127.0.0.1 dev origins only | Comma-separated list of allowed WebSocket origins, for example `https://poker.example.com`. When unset, pokerd allows only local development origins (`localhost:*` / `127.0.0.1:*`), NOT all origins - a remote client with an unset value is rejected. |
| `POKERD_VERSION` | docker-compose build arg | no | `dev` | Stamped into the pokerd binary as `main.version` and reported by `GET /healthz`; set to a git tag/sha in CI so you can confirm which build is live. |
| `POSTGRES_PASSWORD` | docker-compose (postgres service) | yes, if using compose's postgres service | none | Superuser password for the bundled Postgres container. Also used when constructing `POKERD_DATABASE_URL` in `.env`. |
| `VITE_API_URL` | client build | no | same-origin | Base URL the client uses for HTTP API calls, for example `https://poker.example.com`. |
| `VITE_WS_URL` | client build | no | derived from current origin | WebSocket URL the client connects to, for example `wss://poker.example.com/ws`. |

## Configuration safety semantics

In development (`POKERD_ENV` unset) pokerd starts even when optional configuration is missing, logging explicit warnings:

- **Ephemeral auth secret**: if `POKERD_AUTH_SECRET` is not set, pokerd generates a random 32-byte secret at startup and logs `WARNING: POKERD_AUTH_SECRET not set; using a random ephemeral secret`.
  Every process restart invalidates all previously issued auth tokens, forcing every client to re-authenticate.
- **In-memory stores**: if `POKERD_DATABASE_URL` is not set, pokerd logs `WARNING: POKERD_DATABASE_URL not set; using in-memory stores` and keeps accounts, balances, and hand histories only in process memory, lost on every restart or crash.

In production (`POKERD_ENV=production`) these are NOT warnings - pokerd refuses to start if `POKERD_DATABASE_URL` or `POKERD_AUTH_SECRET` is missing, so a typo in `.env` can never silently bring up a data-losing, token-rotating server.
The compose stack sets `POKERD_ENV=production` and uses `:?` guards on the required variables, so `docker compose up` fails fast with a clear message rather than starting a misconfigured server.

## Backups and restore

The compose `postgres` service stores data in the named volume `pokerd_pgdata`. A named volume survives `docker compose down` but NOT `docker compose down -v`, and it is not a backup - a single disk failure or an accidental `-v` loses every account, balance, and hand history. Take regular logical backups:

```
# Nightly logical dump (run from the repo dir on the VM, e.g. via cron).
docker compose exec -T postgres pg_dump -U pokerd -d pokerd --format=custom \
	> "backups/pokerd-$(date +%F).dump"
```

Restore into a fresh database:

```
docker compose exec -T postgres pg_restore -U pokerd -d pokerd --clean --if-exists \
	< backups/pokerd-YYYY-MM-DD.dump
```

Keep backups off the VM (object storage, another host) so a full-VM loss is recoverable, and periodically test a restore into a throwaway database - an untested backup is not a backup.

## Health and versioning

`GET /healthz` returns the liveness status and the running build version (stamped from the `POKERD_VERSION` build arg / `-X main.version` ldflag). The reverse proxy or orchestrator should probe it: the distroless container image has no shell or curl, so pokerd cannot self-probe via a Docker `HEALTHCHECK`; use the proxy's health check or an external uptime monitor hitting `/healthz` over TLS.

## Client static hosting notes

The client is a plain static site after `npm run build`; it has no server-side runtime requirement.
It can be hosted on the same VM behind Caddy (as shown above), or on any static host (S3 + CloudFront, Netlify, Vercel static, GitHub Pages, and so on).
Set `VITE_API_URL` and `VITE_WS_URL` at build time, since Vite inlines `import.meta.env.*` values into the built bundle; changing them requires rebuilding, not just redeploying.
If `VITE_API_URL`/`VITE_WS_URL` are omitted, the client falls back to same-origin requests, which works when the static build is served from the same host and path structure as pokerd (as in the Caddy recipe above).
