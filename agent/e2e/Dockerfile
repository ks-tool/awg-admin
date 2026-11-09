# syntax=docker/dockerfile:1
#
# Builds vpn-agent-userspace from source for the e2e test (tunnel_test.go). The
# build context is the agent module root (../), so this compiles the agent as it
# is in the working tree — the amneziawg-go library is compiled in, no external
# process. Runtime carries iproute2/iptables (addr/up + lifecycle hooks).
FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build the specific file (cmd/ holds several `package main` files — vpn-agent.go,
# vpn-agent-userspace.go, dump.go — so a whole-package build would fail).
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/vpn-agent-userspace cmd/vpn-agent-userspace.go

FROM alpine:3.23
RUN apk add --no-cache ca-certificates iproute2 iptables
COPY --from=build /out/vpn-agent-userspace /usr/local/bin/vpn-agent-userspace
ENV VPN_AGENT_DB=/data \
    VPN_AGENT_ADDR=0.0.0.0:8080 \
    HOME=/data
ENTRYPOINT ["/usr/local/bin/vpn-agent-userspace"]
