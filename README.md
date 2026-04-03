# HookWatch

Local run instructions for the `hookwatch` app.

Additional docs:

- [`docs/auth.md`](./docs/auth.md) for running the app in `none`, `local`, and `oidc` auth modes
- [`docs/action_transformation.md`](./docs/action_transformation.md) for the action pipeline and transform behavior
- [`docs/advanced_requests.md`](./docs/advanced_requests.md) for replay, diffing, OpenAPI generation, persistence, and signature validation

## What It Runs

HookWatch is a single Go server that serves:

- the JSON API under `/api`
- the webhook capture endpoints under `/{tokenId}`
- the Svelte frontend from an embedded static build

The simplest local workflow is Docker Compose. A direct host run is also available if you have Go and Node installed.

## Run With Docker Compose

Requirements:

- Docker
- Docker Compose

From the repo root:

```bash
cd hookwatch
docker compose up --build
```

Then open:

```text
http://localhost:8080
```

What this does:

- builds the frontend
- builds the Go binary
- starts the app on port `8080`
- persists SQLite data in the named Docker volume `hookwatch-data`

For local OIDC with Keycloak:

```bash
docker compose -f docker-compose.yml -f docker-compose.oidc.yml up --build
```

For Linux, add the compatibility override:

```bash
docker compose -f docker-compose.yml -f docker-compose.oidc.yml -f docker-compose.oidc.linux.yml up --build
```

For the local OIDC stack, sign into HookWatch with the imported realm user `admin` / `admin` or `hookwatch-user` / `hookwatch-password`.

To stop it:

```bash
docker compose down
```

To stop it and remove the local database volume:

```bash
docker compose down -v
```

## Run Directly On Your Machine

Requirements:

- Go 1.25+
- Node.js 22+
- npm

From the repo root:

```bash
cd hookwatch/frontend
npm ci
npm run build
```

Then, in another shell:

```bash
cd hookwatch
go run ./cmd/hookwatch --port 8080 --data-dir ./data --auth-mode=none
```

Open:

```text
http://localhost:8080
```

Notes:

- The Go binary embeds files from `frontend/build`, so build the frontend before `go run` or `go build`.
- SQLite data is stored in `./data` by default.

## Useful Configuration

The binary accepts either flags or environment variables.

Common flags:

```bash
--port 8080
--data-dir ./data
--auth-mode none
--allow-registration=false
--oidc-issuer=
--oidc-client-id=
--oidc-client-secret=
--token-ttl 24h
--max-requests 500
--token-cleanup-interval 1h
```

Equivalent environment variables:

```bash
HOOKWATCH_PORT=8080
HOOKWATCH_DATA_DIR=./data
HOOKWATCH_AUTH_MODE=none
HOOKWATCH_ALLOW_REGISTRATION=false
HOOKWATCH_OIDC_ISSUER=
HOOKWATCH_OIDC_CLIENT_ID=
HOOKWATCH_OIDC_CLIENT_SECRET=
HOOKWATCH_TOKEN_TTL=24h
HOOKWATCH_MAX_REQUESTS=500
HOOKWATCH_TOKEN_CLEANUP_INTERVAL=1h
```

Defaults:

- port: `8080`
- data dir: `./data`
- auth mode: `none`
- token TTL: `24h` (1 day)
- max captured requests per token: `500`
- expired token cleanup interval: `1h`

## Auth Modes For Local Use

See [`docs/auth.md`](./docs/auth.md) for complete run instructions and examples for:

- `auth-mode=none`
- `auth-mode=local`
- `auth-mode=oidc`

## Quick Smoke Test

1. Start the app.
2. Open `http://localhost:8080`.
3. Create a webhook from the home page.
4. Send a request to the generated URL:

```bash
curl -X POST http://localhost:8080/<token-id> \
  -H 'Content-Type: application/json' \
  -d '{"hello":"world"}'
```

You should see the request appear in the UI for that token.
