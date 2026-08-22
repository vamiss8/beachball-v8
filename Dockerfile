# syntax=docker/dockerfile:1

# the client and the server are built in separate stages and only their
# outputs are copied into the final image, so neither node nor the go
# toolchain ships to production.

# --- client ------------------------------------------------------------------
FROM node:22-alpine AS client
WORKDIR /src/client

# the lockfile alone first: this layer is rebuilt only when dependencies
# change, not on every edit to the game
COPY client/package.json client/package-lock.json ./
# retried because a reset connection mid-tarball is a fact of life on flaky
# links and shared CI runners. the npm cache survives between attempts inside
# a single layer, so every retry has less left to download than the last
RUN for attempt in 1 2 3 4 5; do \
      npm ci --no-audit --no-fund && exit 0; \
      echo "npm ci failed (attempt $attempt of 5), retrying"; \
      sleep 5; \
    done; \
    exit 1

COPY client/ ./
RUN npm run build

# --- server ------------------------------------------------------------------
FROM golang:1.26-alpine AS server
WORKDIR /src/server

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
# CGO off because the runtime image below has no libc to link against.
# trimpath and -s -w drop build paths and debug tables from the binary
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/beachball ./cmd/server

# --- runtime -----------------------------------------------------------------
# distroless: no shell, no package manager, nothing to exploit that we did not
# put there ourselves
FROM gcr.io/distroless/static-debian12:nonroot

# standard labels: registries and tools read these to link an image back to
# the code it came from, which is the only way to tell later what is inside a
# tag that has no shell to look around in
LABEL org.opencontainers.image.title="beachball" \
      org.opencontainers.image.description="Two-player arcade volleyball in the browser" \
      org.opencontainers.image.source="https://github.com/vamiss8/beachball-v8" \
      org.opencontainers.image.licenses="MIT"

COPY --from=server /out/beachball /app/beachball
COPY --from=client /src/client/dist /app/client

# the defaults a container needs, so it runs with no arguments at all. a host
# that assigns a port overrides PORT, and ALLOWED_ORIGINS is only needed when
# a proxy rewrites the Host header
ENV STATIC_DIR=/app/client
EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/app/beachball"]
