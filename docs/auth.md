# Authentication Modes

This document explains how to run HookWatch in each supported authentication mode.

## Prerequisites

Build the frontend before running the Go server directly:

```bash
cd hookwatch/frontend
npm ci
npm run build
```

Then run the server from `hookwatch/`.

## Mode: none

Use `none` for local development or single-user testing with no sign-in flow.

```bash
cd hookwatch
go run ./cmd/hookwatch \
  --port 8080 \
  --data-dir ./data \
  --auth-mode=none
```

Equivalent environment variables:

```bash
export HOOKWATCH_PORT=8080
export HOOKWATCH_DATA_DIR=./data
export HOOKWATCH_AUTH_MODE=none
go run ./cmd/hookwatch
```

Behavior:

- no login or registration UI
- hooks can still use private receive secrets
- private view mode is forced public because there are no users

## Mode: local

Use `local` for built-in email/password authentication backed by SQLite.

```bash
cd hookwatch
go run ./cmd/hookwatch \
  --port 8080 \
  --data-dir ./data \
  --auth-mode=local
```

To allow open self-service registration after the first account:

```bash
cd hookwatch
go run ./cmd/hookwatch \
  --port 8080 \
  --data-dir ./data \
  --auth-mode=local \
  --allow-registration=true
```

Equivalent environment variables:

```bash
export HOOKWATCH_PORT=8080
export HOOKWATCH_DATA_DIR=./data
export HOOKWATCH_AUTH_MODE=local
export HOOKWATCH_ALLOW_REGISTRATION=true
go run ./cmd/hookwatch
```

Behavior:

- first registered user becomes `admin`
- first user can register even if `--allow-registration=false`
- later registrations require `--allow-registration=true`
- login uses the built-in `/login` form

## Mode: oidc

Use `oidc` for external single sign-on through an OpenID Connect provider such as Keycloak, Auth0, Okta, or Google.

Required settings:

- `--oidc-issuer`
- `--oidc-client-id`
- `--oidc-client-secret`

Example:

```bash
cd hookwatch
go run ./cmd/hookwatch \
  --port 8080 \
  --data-dir ./data \
  --auth-mode=oidc \
  --oidc-issuer=https://issuer.example.com/realms/main \
  --oidc-client-id=hookwatch \
  --oidc-client-secret=super-secret
```

Equivalent environment variables:

```bash
export HOOKWATCH_PORT=8080
export HOOKWATCH_DATA_DIR=./data
export HOOKWATCH_AUTH_MODE=oidc
export HOOKWATCH_OIDC_ISSUER=https://issuer.example.com/realms/main
export HOOKWATCH_OIDC_CLIENT_ID=hookwatch
export HOOKWATCH_OIDC_CLIENT_SECRET=super-secret
go run ./cmd/hookwatch
```

Behavior:

- visiting `/login` redirects to the identity provider
- local password login and registration are disabled
- first successful OIDC user becomes `admin`
- existing local accounts are not auto-linked by matching email

## Docker Example

The same auth settings can be passed through the environment when using Docker Compose or another container runtime.

Example for local auth:

```bash
HOOKWATCH_AUTH_MODE=local HOOKWATCH_ALLOW_REGISTRATION=true docker compose up --build
```

Example for OIDC:

```bash
HOOKWATCH_AUTH_MODE=oidc \
HOOKWATCH_OIDC_ISSUER=https://issuer.example.com/realms/main \
HOOKWATCH_OIDC_CLIENT_ID=hookwatch \
HOOKWATCH_OIDC_CLIENT_SECRET=super-secret \
docker compose up --build
```

## Local OIDC With Keycloak

Docker Desktop:

```bash
docker compose -f docker-compose.yml -f docker-compose.oidc.yml up --build
```

Linux:

```bash
docker compose -f docker-compose.yml -f docker-compose.oidc.yml -f docker-compose.oidc.linux.yml up --build
```

Local URLs and credentials:

- HookWatch: `http://localhost:8080`
- Keycloak realm: `hookwatch`
- Keycloak admin console: `http://localhost:8090/admin/`
- Keycloak admin console user: `admin`
- Keycloak admin console password: `admin`
- HookWatch realm login user: `admin`
- HookWatch realm login password: `admin`
- Test OIDC user: `hookwatch-user`
- Test OIDC password: `hookwatch-password`

Important:

- the Keycloak admin console account lives in the `master` realm
- HookWatch signs into the imported `hookwatch` realm
- the local bootstrap now includes a matching `admin` user inside the `hookwatch` realm so `admin/admin` works on the HookWatch login screen too

The compose overlay preconfigures HookWatch with:

- issuer: `http://keycloak.localhost:8090/realms/hookwatch`
- client ID: `hookwatch`
- client secret: `hookwatch-local-secret`
