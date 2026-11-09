#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "usage: $0 <agent/v1.2.3|providers/awg/v1.2.3>" >&2
    exit 2
fi

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tag=$1
case "$tag" in
    agent/v*)
        binary=vpn-agent
        package=./cmd/vpn-agent
        repository=vpn-agent
        version=${tag#agent/}
        artifact_type=application/vnd.ks-tool.vpn-agent.v1
        ;;
    providers/awg/v*)
        binary=vpn-provider-awg
        package=./cmd/vpn-provider-awg
        repository=vpn-providers/awg
        version=${tag#providers/awg/}
        artifact_type=application/vnd.ks-tool.vpn-provider-awg.v1
        ;;
    providers/example/v*)
        binary=example-provider
        package=./cmd/example-provider
        repository=vpn-providers/example
        version=${tag#providers/example/}
        artifact_type=application/vnd.ks-tool.vpn-provider-example.v1
        ;;
    *)
        echo "unsupported release tag: $tag" >&2
        exit 2
        ;;
esac

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
    echo "invalid semantic version in tag: $tag" >&2
    exit 2
fi

out="$root/dist/${binary}-${version}"
rm -rf "$out"
mkdir -p "$out"
for arch in amd64 arm64; do
    mkdir -p "$out/linux/$arch"
    (
        cd "$root"
        CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
            go build -trimpath -ldflags="-s -w -X main.version=$version" \
            -o "$out/linux/$arch/$binary" "$package"
    )
done

cat > "$out/pack.yaml" <<EOF
repository: $repository:$version
type: $artifact_type
annotations:
  org.opencontainers.image.source: https://github.com/ks-tool/vpn-agent
  org.opencontainers.image.version: ${version#v}
items:
  - from: file://linux/amd64/$binary
    platform: linux/amd64
    annotations:
      org.opencontainers.image.title: $binary
  - from: file://linux/arm64/$binary
    platform: linux/arm64
    annotations:
      org.opencontainers.image.title: $binary
EOF

registry=${OCI_REGISTRY:-oci://ghcr.io/ks-tool}
options=()
if [[ ${OCI_PLAIN_HTTP:-} == 1 ]]; then
    options+=(--plain-http)
fi
if [[ -n ${OCI_LOGIN:-} ]]; then
    options+=(--login "$OCI_LOGIN" --password "${OCI_PASSWORD:-}")
fi
(
    cd "$out"
    "${OCI_PACKER:-oci-packer}" "$registry" -f pack.yaml "${options[@]}"
)
