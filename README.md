<p align="center">
  <img src="docs/logo.png" alt="VPN Admin" width="128" height="128">
</p>

# VPN Admin (vpn-admin)

[Русская версия](README_ru.md)

**VPN Admin** is a visual control plane for VPN infrastructure. It combines a geographic server map, a network topology editor,
user management and settings in one workspace. The core is not tied to a particular VPN protocol: extensions handle service
configuration and network links.

> **Installation, operations, plugins and migration:** [docs/GUIDE.md](docs/GUIDE.md)

## Features

- **Map.** Servers are positioned from **Location** using a local Data Center Atlas cache, or from manually entered coordinates.
  Points show agent or VPN service health, while arrows show connections and p95 latency.
- **Network Graph.** Interfaces can be connected by directed arrows into chains of any length, including mixed Xray/AWG/Hysteria
  routes. Same-protocol links work between nodes; local `route` links switch providers on one host. Compatibility, duplicates,
  self-connections and cycles are validated. To reroute an existing connection, select its arrow, press `m`, drag either endpoint
  and confirm with **Create**.
- **Agent and providers.** Under **Manage panel → Agent**, `vpn-agent` can be installed from an OCI release or a local binary.
  VPN providers are installed and removed independently of the agent. Public GHCR and private OCI registries are supported.
- **Interfaces through JSON Schema.** Under **Manage panel → Interfaces**, a wizard first selects an installed provider and then
  builds its `Interface` form through RJSF. Schemas are always read from `~/.vpn-admin/cache/<apiVersion>.<kind>.spec` when
  available and refreshed in the background. Peers are not created here: a user selects the required interface and the peer
  form uses that provider's `Peer` schema. Unmanaged `wg-quick`/`awg-quick` interfaces can be imported from the same tab.
- **VPN service plugins.** A separate plugin API models a service as “application + configuration”, passes topology to the plugin
  and provides independent JSON schemas for configuration and users.
- **AWG, Xray and Hysteria 2.** AWG supports obfuscation, kernel/userspace modes and WireGuard client configs. Xray provides
  VLESS over TCP or WebSocket with optional TLS; Hysteria 2 provides QUIC listeners with multiple username/password peers.
  The proxy providers generate client URIs and manage their own core processes and listener firewall rules.

VPN Admin is available as a Wails desktop application and as a standalone web server. Both modes share the same data model,
business logic and frontend.

## Architecture

### Admin app

The root Go module contains the desktop/standalone application, HTTP API, BoltDB storage, the agent pack and deployment
coordination. The React frontend is in `frontend/`.

The local `~/.vpn-admin/db` database stores servers, SSH and mTLS material, users and peers, the abstract topology, plugin
services, provider resources, links, OCI sources and the standalone account. Servers and interfaces migrated from v1 are
projected into the shared topology with their original UUIDs.

### Agent and providers

`vpn-agent` is a provider-neutral HTTP service on each managed host. It stores desired resources and provider registrations,
runs providers as child processes, restarts them after failures and exposes resource, link, host-info and metrics APIs. Systemd
manages only `vpn-agent`; separate provider units are unnecessary.

By default the agent listens on loopback and is reached through an SSH tunnel; direct mTLS is also supported. With
`discover: true`, the agent obtains resource definitions and JSON schemas through the provider's `GET /v1/manifest` endpoint.

The agent and providers live in a separate Go module under `agent/`, with module path `github.com/ks-tool/vpn-agent`. The root
module uses it through a local `replace`. Releases are published as multi-architecture OCI artifacts:

- `agent/v1.2.3` → `ghcr.io/ks-tool/vpn-agent:v1.2.3`;
- `providers/awg/v1.2.3` → `ghcr.io/ks-tool/vpn-providers/awg:v1.2.3`;
- `providers/xray/v1.2.3` → `ghcr.io/ks-tool/vpn-providers/xray:v1.2.3`;
- `providers/hysteria2/v1.2.3` → `ghcr.io/ks-tool/vpn-providers/hysteria2:v1.2.3`.

### Two extension layers

An **agent provider** runs on the managed server and owns protocol-specific resources. The admin does not import provider code
or know its dependencies. It downloads the selected binary and invokes the standard `deploy --config` command with opaque
options. The provider installs its core and dependencies and writes its own agent registration. Resource APIs use domain
versions such as `awg.ks-tool.dev/v1` and `xray.ks-tool.dev/v1`. The admin coordinates `prepare`/`commit`/`delete` only for
links between compatible resource ports.

A **deployment plugin** is called by the admin process and receives the service, server, interface and related connections. It
implements `apply`, `delete`, `status` and service-user CRUD. External plugins are executables using JSON over stdio, not Go
`.so` files. Manifests are loaded from `~/.vpn-admin/plugins`; `VPN_ADMIN_PLUGINS_DIR` overrides that directory.

These mechanisms complement each other: a provider manages resources through the agent API, while a plugin deploys an abstract
VPN service from the topology model.

### Deployment through pack

The admin embeds `pack apply` in its own process; neither an external `pack` command nor ansible-target is used. The external
`host` target is downloaded automatically and supports Debian and Fedora on `amd64` and `arm64`. The agent and each provider are
applied in independent operations. A provider is optional, not a required part of the agent.

For OCI installation, the target server downloads its platform binary through a temporary loopback OCI proxy. For a local agent
binary, the binary is shipped as pack content. Every provider then performs its own `deploy`; there is no provider-name switch
in the admin. Providers manage listener firewalls through firewalld, ufw or iptables during resource reconciliation. See
[pack/README.md](pack/README.md) for the agent pack.

### Metrics

The agent collects system metrics and ICMP latency; the AWG provider collects peer traffic and handshakes. Their APIs are
separate:

- `GET /metrics` and `GET /metrics/history` — system;
- `GET /metrics/{interface}` and `GET /metrics/{interface}/history` — interface and peers.

Latency is the p95 of successful ICMP probes over a 15-minute sliding window. Map arrows are blue up to 100 ms, yellow between
100 and 250 ms, and red above 250 ms; arrows remain blue when no measurement is available. Topology metric requests run in
parallel per source server and do not block graph loading.

## Building from source

Requirements: Go **1.27.1** or later and Node.js **24** or later.

```sh
make server       # standalone web server
make run-server   # local development server through go run
make desktop      # Wails desktop application
make agent        # provider-neutral Linux agent
make provider-awg # Linux AWG provider
make -C agent build-provider-xray
make -C agent build-provider-hysteria2
make migrate      # awg-migrate utility
make test         # unit tests for both Go modules
```

Build artifacts are written to `build/bin/`. Targets that include the UI build the frontend automatically. Wails CLI is invoked
through `go tool wails`.

The Docker-based AWG userspace tunnel test is separate and requires Docker with `/dev/net/tun` and `NET_ADMIN`:

```sh
make -C agent test-e2e
make -C agent test-provider-e2e # Xray/Hysteria chains and mixed 8-node/13-link chain
```

## Migration and backups

v1-to-v2 migration is manual through `awg-migrate v2`. It creates a separate database and leaves the source unchanged. See
[docs/migrate_v2.md](docs/migrate_v2.md) for resource adoption and rollback.

**Settings → Backup** exports a complete JSON dump of the admin database. It contains secrets and must be protected like the
database itself.

## License

This project is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for the full license text.
