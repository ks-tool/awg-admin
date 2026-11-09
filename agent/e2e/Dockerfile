# syntax=docker/dockerfile:1
FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/vpn-provider-awg ./cmd/vpn-provider-awg

FROM alpine:3.23
RUN apk add --no-cache ca-certificates iproute2 iptables
COPY --from=build /out/vpn-provider-awg /usr/local/bin/vpn-provider-awg
ENV VPN_PROVIDER_AWG_DB=/data \
    VPN_PROVIDER_AWG_ADDR=0.0.0.0:8080 \
    HOME=/data
ENTRYPOINT ["/usr/local/bin/vpn-provider-awg", "--userspace"]
