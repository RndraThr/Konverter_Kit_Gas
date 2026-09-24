# Docker & Staging Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Containerize the Konkit application (Go backend + React frontend, single origin) and produce a working `docker compose` stack plus documented VPS deployment steps, so the team can run a staging environment reachable over HTTPS on a real domain.

**Architecture:** A 3-stage `Dockerfile` (Node build → Go build → slim runtime image carrying `go.mod` + built frontend assets + the `server`/`migrate`/`admin` binaries — `go.mod` is required at runtime because `internal/web/server.go`'s `projectRoot()` walks up from the working directory looking for it to locate `web/static`). A `docker-compose.yml` with 3 services: `postgres` (data on a named volume), `app` (runs `migrate up` then `server` on container start), `caddy` (reverse proxy, automatic HTTPS via a domain pointed at the VPS). Secrets live in a git-ignored `.env.staging` file plus a mounted, git-ignored Google service-account JSON file (`STORAGE_BACKEND=gdrive` was chosen for staging).

**Tech Stack:** Docker, Docker Compose, Caddy (reverse proxy + automatic TLS), Go 1.26 (no CGO — pure-Go `pgx` driver, confirmed via `go.mod`), Node 20 (frontend build only, not present in the runtime image).

**Spec:** None — per this session's established pattern (see the two prior plans), the user asked to skip the formal spec.md step to save time; design was worked out interactively in chat. This plan is the design's only written record.

## Global Constraints

- Never commit secrets: `.env.staging` and the Google service-account JSON file must be `.gitignore`d, never baked into the Docker image.
- The runtime image must NOT include Node, npm, or any frontend source/`node_modules` — only the built `web/static/app` output.
- `STORAGE_PATH` must be an absolute path when `APP_ENV` is not `local` (`internal/config/config.go:66`) — the staging `.env.staging` must set an absolute path even though `STORAGE_BACKEND=gdrive` means it's barely used (still required for `LocalStorage`'s fallback paths and any local temp handling — confirm this by reading `internal/media/storage.go` before assuming it's dead weight to set).
- `SESSION_SECRET` must be at least 32 bytes (`internal/config/config.go:82`) — the deployment steps must show how to generate one, not tell the user to invent one by hand.
- `APP_BASE_URL` must be an absolute `http`/`https` URL with a host (`internal/config/config.go:87-90`) — for staging this must be the real HTTPS domain, not `localhost`.
- `SESSION_COOKIE_SECURE` should be `true` in staging (served over HTTPS via Caddy) — verify this is actually read and enforced by checking `internal/config/config.go`'s handling (already partially read above) and `internal/auth`'s cookie-setting code before writing the compose env block, don't assume.
- Every task's Docker/compose changes must be verified by actually running them locally (`docker build`, `docker compose up`) — Docker is confirmed available on this machine. Do not mark a task done on the strength of "the Dockerfile looks right" alone.
- Do not touch any application source code in `internal/`, `frontend/src/`, or `cmd/` — this plan is infrastructure-only. If a real application bug blocks containerization (unlikely, given the codebase is already fully env-var-configured), stop and flag it rather than silently patching around it.

---

### Task 1: `Dockerfile` — multi-stage build

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

**Interfaces:**
- Produces: a Docker image tagged locally as `konkit:local` for Task 2 to reference in `docker-compose.yml`. The image's `ENTRYPOINT`/`CMD` must run migrations then start the server (exact command decided in this task, consumed by Task 2's compose service definition).

- [ ] **Step 1: Read the two files this Dockerfile depends on understanding**

Read `internal/web/server.go` (the `projectRoot()` function, already located at line 49) and `internal/database/migrations/embed.go` (confirms migrations are `go:embed`-ed into the `migrate` binary, no separate SQL files need copying) — both already investigated during this plan's design; re-confirm they still say what this plan assumes before writing the Dockerfile.

- [ ] **Step 2: Write `.dockerignore`**

```
.git
.env
.env.staging
storage/
frontend/node_modules
frontend/dist
web/static/app
.worktrees/
.claude/
.kilo/
docs/
*.md
```

(`web/static/app` is excluded from the build context because the Dockerfile's own frontend-build stage regenerates it — never copy a stale local build into the image.)

- [ ] **Step 3: Write the Dockerfile**

```dockerfile
# syntax=docker/dockerfile:1

# ---- Stage 1: frontend build ----
FROM node:20-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- Stage 2: Go build ----
FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=frontend /app/web/static/app ./web/static/app
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/admin ./cmd/admin

# ---- Stage 3: runtime ----
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY go.mod ./
COPY --from=frontend /app/web/static/app ./web/static/app
COPY --from=backend /out/server /out/migrate /out/admin ./
EXPOSE 8080
ENTRYPOINT ["/bin/sh", "-c", "./migrate up && exec ./server"]
```

`ca-certificates` is required in the runtime image because `internal/media/gdrive.go` makes outbound HTTPS calls to the Google Drive API — without it, TLS verification fails. `go.mod` is copied into the runtime stage (not just the build stage) because `projectRoot()` needs to find it at runtime, per Step 1's confirmation — this is NOT a build-time artifact being carried over by mistake, it's a genuine runtime dependency of this codebase's static-file-serving logic.

- [ ] **Step 4: Build and smoke-test the image locally**

```bash
cd "d:/KSM/Deployment/konkit"
docker build -t konkit:local .
```

Expected: build succeeds, no errors. This alone doesn't prove the entrypoint works (needs a real Postgres — Task 2 provides one), but it proves the multi-stage build, dependency resolution, and binary compilation are all correct.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "feat(deploy): add multi-stage Dockerfile for the Konkit server"
```

---

### Task 2: `docker-compose.yml` — Postgres, app, Caddy

**Files:**
- Create: `docker-compose.yml`
- Create: `Caddyfile`
- Create: `.env.staging.example`
- Modify: `.gitignore`

**Interfaces:**
- Consumes: the `konkit:local`-equivalent image Task 1's Dockerfile builds (compose builds it inline via `build: .`, not a pre-built tag).
- Produces: a runnable local stack (`docker compose up`) that Task 3's deployment steps document how to run on a real VPS with a real domain.

- [ ] **Step 1: Confirm `SESSION_COOKIE_SECURE` is genuinely read and enforced**

Read `internal/config/config.go` in full (the `SESSION_COOKIE_SECURE` parsing, lines ~92-98 per this plan's earlier read) and grep `internal/auth/` for where `cfg.SessionCookieSecure` (or equivalent) is actually consumed when setting the session cookie (`Secure` attribute on `http.Cookie`). Confirm it is genuinely wired through — if it turns out to be parsed but unused, that's a real defect outside this plan's stated scope (infrastructure-only); flag it in the ledger rather than silently patch application code.

- [ ] **Step 2: Write `.env.staging.example`** (a template — the real `.env.staging` is never committed)

```
APP_ENV=production
APP_ADDR=:8080
APP_BASE_URL=https://staging.example.com
DATABASE_URL=postgres://konkit:CHANGE_ME@postgres:5432/konkit?sslmode=disable
SESSION_SECRET=CHANGE_ME_AT_LEAST_32_BYTES_RANDOM
SESSION_COOKIE_SECURE=true
STORAGE_PATH=/app/storage
STORAGE_BACKEND=gdrive
GDRIVE_SERVICE_ACCOUNT_JSON=/run/secrets/gdrive-service-account.json
GDRIVE_ROOT_FOLDER_ID=CHANGE_ME
POSTGRES_USER=konkit
POSTGRES_PASSWORD=CHANGE_ME
POSTGRES_DB=konkit
```

(`DATABASE_URL`'s host is `postgres` — the compose service name, resolved via Docker's internal DNS, not `localhost`. `GDRIVE_SERVICE_ACCOUNT_JSON` here is a *file path inside the container*, not the JSON content itself — confirm this matches what `internal/media/gdrive.go`'s `NewGoogleDriveStorage`/`option.WithCredentialsFile` actually expects, i.e. `GDriveServiceAccountJSON` config value must be usable as a file path — read `internal/config/config.go`'s `GDriveServiceAccountJSON` field and its one call site to verify before assuming this is right.)

- [ ] **Step 3: Write `Caddyfile`**

```
{$APP_DOMAIN} {
	reverse_proxy app:8080
}
```

- [ ] **Step 4: Write `docker-compose.yml`**

```yaml
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]
      interval: 5s
      timeout: 5s
      retries: 10

  app:
    build: .
    restart: unless-stopped
    env_file: .env.staging
    volumes:
      - storage-data:/app/storage
      - ${GDRIVE_CREDENTIALS_HOST_PATH}:/run/secrets/gdrive-service-account.json:ro
    depends_on:
      postgres:
        condition: service_healthy
    expose:
      - "8080"

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    environment:
      APP_DOMAIN: ${APP_DOMAIN}
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
      - caddy-config:/config
    depends_on:
      - app

volumes:
  postgres-data:
  storage-data:
  caddy-data:
  caddy-config:
```

`GDRIVE_CREDENTIALS_HOST_PATH` and `APP_DOMAIN` are read from the shell environment (or a top-level `.env` file docker-compose auto-loads — note this is a SEPARATE mechanism from the app's own `.env.staging`, which is explicitly passed via `env_file:` to the `app` service only; do not conflate the two). Document this distinction clearly in Task 3's deployment steps so it isn't a source of confusion later.

- [ ] **Step 5: Add `.gitignore` entries**

```
.env.staging
*-service-account.json
```

- [ ] **Step 6: Local smoke test**

Since this machine doesn't have a real domain or Google service account credentials, verify what CAN be verified locally without those:

```bash
cd "d:/KSM/Deployment/konkit"
cp .env.staging.example .env.staging
```
Edit the copy: set `STORAGE_BACKEND=local` (not `gdrive`) and `APP_BASE_URL=http://localhost:8080` for this LOCAL smoke test only (the real staging `.env.staging` on the VPS will use `gdrive` + the real domain, per the user's actual decision — this local substitution exists purely to prove the compose stack itself works without requiring real Google credentials or DNS during development). Generate a real `SESSION_SECRET`: `openssl rand -base64 32`. Remove the `GDRIVE_CREDENTIALS_HOST_PATH`-dependent volume mount and the `caddy` service for this local-only test (or set `GDRIVE_CREDENTIALS_HOST_PATH` to a dummy empty file) — use `docker compose up postgres app` (skip `caddy`) and curl `http://localhost:8080/api/v1/health` directly against the `app` service's exposed port (temporarily add `ports: ["8080:8080"]` to `app` for this test only, or `docker compose exec app` a curl from inside). Confirm the health check reports the database as reachable and the server starts without error, then `docker compose down -v` to clean up the test volumes.

Expected: `server` starts, migrations run cleanly against the fresh `postgres` container, health endpoint responds `200`.

- [ ] **Step 7: Commit**

```bash
git add docker-compose.yml Caddyfile .env.staging.example .gitignore
git commit -m "feat(deploy): add docker-compose stack (postgres, app, caddy reverse proxy)"
```

---

### Task 3: Deployment documentation

**Files:**
- Modify: `README.md` (extend the existing "Alur Git Dan Deployment" section, `README.md:138` onward)

**Interfaces:**
- Consumes: the exact file names, env var names, and commands Tasks 1-2 actually produced — read the live `Dockerfile`/`docker-compose.yml`/`.env.staging.example` before writing these steps, don't restate this plan's draft from memory (Tasks 1-2's implementers may have made small justified adjustments during their own verification steps).

- [ ] **Step 1: Write the VPS setup steps**

Extend `README.md`'s "Alur Git Dan Deployment" section with a new subsection covering, in order, with exact copy-pasteable commands for **Ubuntu/Debian**:

1. Install Docker Engine + Compose plugin (official Docker `apt` repository steps — not the deprecated standalone `docker-compose` binary).
2. Point the staging domain's DNS A record at the VPS's public IP (instruct the user to do this themselves — not something a command can do).
3. Clone the repo onto the VPS.
4. Copy `.env.staging.example` to `.env.staging`, fill in real values: generate `SESSION_SECRET` via `openssl rand -base64 32`, set `APP_BASE_URL`/`APP_DOMAIN` to the real domain, set `POSTGRES_PASSWORD` to a strong random value, place the Google service-account JSON file on the VPS (outside the repo directory) and set `GDRIVE_CREDENTIALS_HOST_PATH` to its absolute path.
5. `docker compose up -d --build`.
6. Bootstrap the first admin account: `docker compose exec app ./admin create`.
7. Verify: `curl https://<domain>/api/v1/health` returns healthy; open the domain in a browser and log in.

- [ ] **Step 2: Write the update/redeploy steps**

A short subsection: `git pull`, `docker compose up -d --build` (rebuilds only what changed, migrations run automatically on the `app` container's next start via the `ENTRYPOINT`). Note that `docker compose up -d --build` on an already-running stack recreates only the `app` service if its image changed, `postgres` data persists via the named volume regardless.

- [ ] **Step 3: Cross-check every command against the actual files**

Read the final `Dockerfile`, `docker-compose.yml`, and `.env.staging.example` as committed by Tasks 1-2 and confirm every env var name, file name, and command in the new README section matches exactly — this is the same discipline this session applied throughout the 3-POS plans (never paraphrase from a stale draft when the live file is one Read call away).

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document Docker build and staging VPS deployment steps"
```

---

## After This Plan

- TLS/HTTPS depends on the staging domain's DNS actually resolving to the VPS before `docker compose up` — Caddy will retry Let's Encrypt issuance but won't succeed until DNS propagates. Not a code concern, an operational one to flag when the user actually deploys.
- No CI/CD pipeline is set up by this plan — deployment is manual (`git pull` + `docker compose up -d --build` on the VPS). If the team wants automatic deploy-on-push later, that's a separate, future plan (e.g. a GitHub Actions workflow that SSHes in and runs the same two commands).
- Backups: the `postgres-data` and `storage-data` Docker volumes are not backed up by anything this plan sets up. Worth a follow-up once staging holds data anyone cares about losing.
