# syntax=docker/dockerfile:1
FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/vpn-provider-awg ./providers/awg/cmd

FROM debian:13-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates dbus firewalld iproute2 iptables iputils-ping nftables procps ufw \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/vpn-provider-awg /usr/local/bin/vpn-provider-awg
COPY e2e/entrypoint.sh /usr/local/bin/vpn-awg-e2e-entrypoint
RUN chmod 0755 /usr/local/bin/vpn-awg-e2e-entrypoint
ENV VPN_PROVIDER_AWG_DB=/data \
    VPN_PROVIDER_AWG_ADDR=0.0.0.0:8080 \
    HOME=/data
ENTRYPOINT ["/usr/local/bin/vpn-awg-e2e-entrypoint"]
