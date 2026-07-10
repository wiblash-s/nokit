# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1 — web-builder: build the React/TypeScript frontend with bun.
# Vite is configured to output to ../assets/dist (see web/vite.config.ts),
# so the built assets land in /build/assets/dist.
# ---------------------------------------------------------------------------
FROM oven/bun:latest AS web-builder
WORKDIR /build

# Install dependencies first for better layer caching.
COPY web/package.json web/bun.lock web/
RUN cd web && bun install --frozen-lockfile

# Copy the rest of the frontend sources.
COPY web/ web/

# Vite writes to ../assets/dist relative to the web/ dir; ensure it exists.
RUN mkdir -p assets/dist && cd web && bun run build

# ---------------------------------------------------------------------------
# Stage 2 — go-builder: compile the Go binary, embedding the built frontend.
# ---------------------------------------------------------------------------
FROM golang:1.26-alpine AS go-builder
WORKDIR /build

# Download modules first for better layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source tree (node_modules is excluded via .dockerignore).
COPY . .

# Bring in the freshly built frontend so //go:embed all:dist can pick it up.
COPY --from=web-builder /build/assets/dist/ ./assets/dist/

# Build a static, stripped linux/amd64 binary.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /bin/defuse ./cmd/defuse

# ---------------------------------------------------------------------------
# Stage 3 — runtime: minimal alpine image.
# Alpine (unlike distroless) accepts arbitrary UID/GID at runtime, which
# allows TrueNAS Scale / other NAS platforms to run the container as a
# specific user (e.g. 568) that owns the volume mount.
# ---------------------------------------------------------------------------
FROM alpine:3.21 AS runtime

# ca-certificates is needed for any outbound TLS (e.g. future API calls).
# docker-cli lets the panel tail the CS2 container's logs via `docker logs -f`
# (Live Logs feature). Requires the host's Docker socket to be mounted — see
# docker-compose.yml.
RUN apk add --no-cache ca-certificates docker-cli

COPY --from=go-builder /bin/defuse /defuse

EXPOSE 8080
# No USER directive — let the orchestrator set the UID/GID via `user:` in
# compose or the TrueNAS app config (e.g. user: "568:568").
ENTRYPOINT ["/defuse"]
