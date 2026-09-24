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
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* \
	&& groupadd --system --gid 1000 app && useradd --system --uid 1000 --gid app --home /app --shell /usr/sbin/nologin app
WORKDIR /app
COPY go.mod ./
COPY web/templates ./web/templates
COPY --from=frontend /app/web/static/app ./web/static/app
COPY --from=backend /out/server /out/migrate /out/admin ./
RUN mkdir -p /app/storage && chown -R app:app /app && chmod 0555 /app/server /app/migrate /app/admin
USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/bin/sh", "-c", "./migrate up && exec ./server"]
