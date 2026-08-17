# syntax=docker/dockerfile:1

# ---------------------------------------------------------------- frontend --
FROM node:22-bookworm-slim AS web

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
# vite writes to ../internal/webui/dist, i.e. /src/internal/webui/dist
RUN npm run build

# ------------------------------------------------------------------ server --
FROM golang:1.26-bookworm AS server

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist

# cgo is required: zstd level 19 comes from the real C library, which the pure
# Go compressors do not reach.
RUN CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o /out/haste ./cmd/haste

# The runtime image has no shell, so the data directory has to be created and
# handed to the unprivileged user here.
RUN mkdir -p /data && chown 65532:65532 /data

# ----------------------------------------------------------------- runtime --
FROM gcr.io/distroless/base-debian12

COPY --from=server /out/haste /haste
COPY --from=server --chown=65532:65532 /data /data

ENV HASTE_ADDR=:8080 \
    HASTE_DB_PATH=/data/haste.db

USER 65532:65532
EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/haste"]
