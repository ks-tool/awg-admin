# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Frontend: build the SPA. Vite's outDir (see frontend/vite.config.ts) is
# "../cmd/dist" relative to the frontend project root, so building from
# /src/frontend lands the output at /src/cmd/dist — exactly where
# cmd/awg-admin.go's `//go:embed all:dist` expects it.
# ---------------------------------------------------------------------------
FROM node:24-alpine AS frontend

WORKDIR /src/frontend

# Install deps in their own layer so they're only rebuilt when package*.json
# actually change, not on every source edit.
COPY frontend/package.json frontend/package-lock.json frontend/package.json.md5 ./
RUN npm ci

COPY frontend/ ./
RUN npm run build

# ---------------------------------------------------------------------------
# Backend: compile the standalone server (cmd/awg-admin.go), embedding the
# frontend build produced above. The root module pulls in the agent module as
# a normal versioned dependency (its agent/v* tag — no go.work/replace), so
# only the root manifests are needed; they are copied before `go mod download`
# so that step is cached independently of unrelated source changes.
# ---------------------------------------------------------------------------
FROM golang:1.26.3-alpine3.23 AS backend

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /src/cmd/dist ./cmd/dist

ARG TARGETOS
ARG TARGETARCH
RUN GOEXPERIMENT=jsonv2 CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags='-s -w' -o /out/awg-admin ./cmd/awg-admin.go \
    && apk add --no-cache ca-certificates


FROM scratch

COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=backend /out/awg-admin /awg-admin

ENV HOME=/data
WORKDIR /data
VOLUME ["/data"]

EXPOSE 8080
ENTRYPOINT ["/awg-admin"]
