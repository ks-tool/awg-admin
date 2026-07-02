# syntax=docker/dockerfile:1
#
# Builds awg-agent-userspace from source for the e2e test (tunnel_test.go). The
# build context is the agent module root (../), so this compiles the agent as it
# is in the working tree — the amneziawg-go library is compiled in, no external
# process. Runtime carries iproute2/iptables (addr/up + lifecycle hooks).
FROM golang:1.26.3-alpine3.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build the specific file (cmd/ holds several `package main` files — awg-agent.go,
# awg-agent-userspace.go, dump.go — so a whole-package build would fail).
RUN GOEXPERIMENT=jsonv2 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/awg-agent-userspace cmd/awg-agent-userspace.go

FROM alpine:3.23
RUN apk add --no-cache ca-certificates iproute2 iptables
COPY --from=build /out/awg-agent-userspace /usr/local/bin/awg-agent-userspace
ENV AWG_AGENT_DB=/data \
    AWG_AGENT_ADDR=0.0.0.0:8080 \
    HOME=/data
ENTRYPOINT ["/usr/local/bin/awg-agent-userspace"]
