# VPN Admin — User Guide

[Русская версия](GUIDE_ru.md) · [← README](../README.md)

This document describes the current **vpn-admin v2**: one workspace with a map, Network Graph, agents, resource providers and
VPN service plugins. The old Servers, Dashboard and two-node tunnel wizard pages from v1 are intentionally not presented as the
current UI.

## Contents

- [Installation and running](#installation-and-running)
  - [Desktop application](#desktop-application)
  - [Standalone server](#standalone-server)
  - [Configuration and directories](#configuration-and-directories)
- [v2 architecture](#v2-architecture)
  - [Agent and providers](#agent-and-providers)
  - [VPN service plugins](#vpn-service-plugins)
  - [Desired state and synchronization](#desired-state-and-synchronization)
- [The workspace](#the-workspace)
- [Servers and the map](#servers-and-the-map)
  - [Adding a server](#adding-a-server)
  - [SSH and mTLS](#ssh-and-mtls)
  - [Location and coordinates](#location-and-coordinates)
  - [Server status and management](#server-status-and-management)
- [Network Graph and tunnels](#network-graph-and-tunnels)
  - [Creating and deleting a connection](#creating-and-deleting-a-connection)
  - [Chains with multiple nodes](#chains-with-multiple-nodes)
  - [Moving a tunnel](#moving-a-tunnel)
- [Installing the agent and providers](#installing-the-agent-and-providers)
  - [OCI registries and local binaries](#oci-registries-and-local-binaries)
  - [Independent provider installation](#independent-provider-installation)
  - [Managed host requirements](#managed-host-requirements)
  - [Firewall and upgrading from awg-agent](#firewall-and-upgrading-from-awg-agent)
- [Provider resources](#provider-resources)
- [AWG interfaces, users and peers](#awg-interfaces-users-and-peers)
  - [The specialized interface editor](#the-specialized-interface-editor)
  - [Users and client peers](#users-and-client-peers)
  - [Importing wg-quick / awg-quick](#importing-wg-quick--awg-quick)
- [The VPN service model and external plugins](#the-vpn-service-model-and-external-plugins)
- [Metrics and tunnel latency](#metrics-and-tunnel-latency)
- [Settings, security and backups](#settings-security-and-backups)
- [Migrating from v1](#migrating-from-v1)
- [HTTP API](#http-api)
- [Troubleshooting](#troubleshooting)

## Installation and running

VPN Admin has two execution modes with the same database, business logic and frontend.

### Desktop application

Prebuilt packages are published on the [Releases](../../../releases) page:

- **Windows** — `vpn-admin-amd64-installer_<version>.exe` (NSIS) or the portable `vpn-admin_<version>.exe`;
- **macOS** — a universal `vpn-admin_<version>.dmg` for Intel and Apple Silicon;
- **Linux** — `vpn-admin_<version>`; requires `libgtk-3` and `libwebkit2gtk-4.1`.

The desktop application exposes no network HTTP API and requires no login. The macOS build is ad-hoc signed but not notarized.
On first launch, choose **Open** from Finder's context menu, or remove the quarantine attribute:

```sh
xattr -cr "/Applications/VPN Admin.app"
```

### Standalone server

Run a local binary:

```sh
./vpn-admin
```

Run the official container:

```sh
docker run -d --name vpn-admin \
  -p 8080:8080 \
  -v vpn-admin-data:/data \
  ghcr.io/ks-tool/vpn-admin:latest
```

The HTTP server listens on `:8080` by default. Initializing a new database creates one administrator:

- username: `admin`;
- password: `admin`.

Change these credentials immediately in **Settings**. The corresponding API is `POST /auth/change-credentials`, not the PUT
method shown in older instructions. Authentication applies only to standalone mode.

### Configuration and directories

| Variable | Purpose |
|---|---|
| `VPN_ADMIN_ADDR` | Standalone HTTP listen address; defaults to `:8080`. |
| `VPN_ADMIN_TLS_CERT` / `VPN_ADMIN_TLS_KEY` | Static TLS certificate and private key for HTTPS. |
| `VPN_ADMIN_AUTOCERT_DOMAINS` | Comma-separated Let's Encrypt domains; mutually exclusive with a static certificate and requires reachable port 80. |
| `VPN_ADMIN_AUTOCERT_CACHE_DIR` | ACME cache; defaults to `~/.vpn-admin/acme`. |
| `VPN_ADMIN_PLUGINS_DIR` | Directory containing external deployment plugin manifests; defaults to `~/.vpn-admin/plugins`. |

Legacy variables with the `AWG_ADMIN_` prefix are accepted as fallbacks. Main paths:

| Path | Contents |
|---|---|
| `~/.vpn-admin/db` | The complete administrative BoltDB database. |
| `~/.vpn-admin/acme` | Autocert certificates for the standalone interface. |
| `~/.vpn-admin/plugins` | `plugin.json` files and deployment plugin executables. |
| `~/.vpn-admin/cache` | Pack state, target, release caches and `datacenter-atlas.json`. |

Docker sets `HOME=/data`, so the `/data` volume preserves the database and working directories. Do not keep the only database
copy inside a disposable container.

## v2 architecture

The current model contains servers, network interfaces, VPN services, provider resources and directed connections. It is
important to distinguish the two independent extension mechanisms.

### Agent and providers

`vpn-agent` runs on a managed host and has no specific knowledge of Xray, Hysteria, AWG or another protocol. It:

- stores desired `Resource` objects and provider registrations under `VPN_AGENT_DB`;
- starts each provider as a child process, restarts it after failure and stops it during shutdown;
- obtains manifests and JSON schemas through the provider API;
- reconciles resources and participates in link `prepare`/`commit`/`delete`;
- exposes host info, system metrics and ICMP latency.

A provider is an independent optional component. Its own pack defines OS packages, environment, files, registration and
dependencies. Systemd runs only `vpn-agent`; there are no separate provider unit files.

With `discover: true`, the agent reads definitions through the provider's `GET /v1/manifest`. A static registration can embed
definitions directly. Providers normally listen on loopback; remote registrations must use mTLS.

The AWG provider selects its kernel backend when `modinfo amneziawg` or `modinfo wireguard` succeeds, and otherwise uses
`amneziawg-go`. The `userspace` installation option, `--userspace` flag or `VPN_PROVIDER_AWG_USERSPACE=true` forces userspace.

Agent settings come from environment variables and equivalent flags; flags take precedence:

| Variable | Purpose |
|---|---|
| `VPN_AGENT_ADDR` | HTTP listen address; defaults to `127.0.0.1:8080`. |
| `VPN_AGENT_DB` | Directory for resources, providers.d and history; defaults to `~/.vpn-agent`, while pack uses `/var/lib/vpn-agent`. |
| `VPN_AGENT_TLS_CERT` / `VPN_AGENT_TLS_KEY` / `VPN_AGENT_TLS_CLIENT_CA` | All three files together enable mTLS. |
| `VPN_AGENT_LEGACY_PROXY` | Loopback AWG provider URL for the legacy API and interface metrics; pack uses `http://127.0.0.1:9091`. |
| `VPN_AGENT_METRICS_INTERVAL` | System metrics and ICMP probe interval; defaults to `45s`. |
| `VPN_AGENT_LOG_LEVEL` | Logging level; defaults to `info`. |

Run `vpn-agent --help` for the complete flag list. The AWG provider uses the `VPN_PROVIDER_AWG_` prefix, including `DB`, `ADDR`
and `METRICS_INTERVAL`.

### VPN service plugins

A deployment plugin models a VPN service as **application + configuration**. The admin passes the service, server, attached
interface and that interface's connections to it. The plugin implements `apply`, `delete`, `status` and service users. The core
never interprets `service.config`.

A plugin supplies independent JSON Schemas for application configuration and one user; RJSF uses them to build forms. This is
not the same contract as an agent provider manifest: a plugin is called by the admin and manages a complete service, while a
provider runs beside the agent and owns concrete resources.

### Desired state and synchronization

`~/.vpn-admin/db` is the source of truth for inventory, users, topology, OCI sources and links. The agent additionally stores
its own copy of desired resources, so “the agent stores nothing” describes only the old v1 model.

For migrated AWG inventory, a change is stored locally and then pushed best-effort. A failure appears in
`inSync`/`lastSyncError` without rolling back the record. Another push occurs after deployment and after an SSH tunnel
reconnects; the standalone API also exposes `POST /servers/:id/sync`.

For a generic provider resource, `PUT` stores desired state and starts reconciliation. The UI reads resources again and displays
actual `generation`, `observedGeneration`, phase and conditions. A provider error does not remove desired configuration.

## The workspace

Buttons in the upper-right corner switch between four views:

- **Map** — geography, health and tunnels;
- **Network Graph** — interfaces and directed connections;
- **Users** — global AWG users and their peers;
- **Settings** — language, backup, desktop logs and standalone security.

The **+** button adds a server on Map/Graph or a user on Users. The separate Servers, Dashboard and Tunnels pages from v1 no
longer exist in the current workspace.

## Servers and the map

### Adding a server

The modal accepts a name, SSH host/port/user, authentication, agent address, description, Location and zero or more interfaces.
Creating a record does not prove connectivity: SSH errors surface during later operations.

A server created by this form belongs to the current agent-based inventory and is automatically projected into Topology with
the same UUID. Its interfaces appear in Network Graph. The API also supports abstract topology-only servers without agent
inventory, but their Agent and Providers tabs are disabled.

### SSH and mTLS

Supported authentication methods are:

- a private key path on the machine running vpn-admin;
- an uploaded PEM key stored in the database, which takes precedence over a path;
- an SSH password for normal connections, but **not for the pack host target**.

A protected key's passphrase is requested on first use and kept only in process memory. “Use for all connections” applies it as
a fallback to other servers. SSH host keys are not currently checked against `known_hosts`.

By default, the agent listens on `127.0.0.1:8080` and the admin connects through a long-lived SSH tunnel. **Generate TLS**
creates a CA, server certificate and client certificate; subsequent communication uses direct mTLS to the agent address. TLS
material is stored in the admin database and sent to the agent during pack deployment.

### Location and coordinates

When **Location** is set, vpn-admin looks up the city in Data Center Atlas. `City, Country` narrows the search; case and Cyrillic
transliteration are normalized. If several records match without a country qualifier, the first matching dataset entry is used.
The `sites.json` dataset is downloaded from `https://kiankyars.github.io/datacenter-atlas/api/v1/sites.json` and cached in
`~/.vpn-admin/cache/datacenter-atlas.json` for 24 hours. A stale cache is used if refresh fails.

Manual latitude and longitude are available **only when Location is empty**. Setting Location disables these fields; changing
Location clears previous coordinates and triggers another lookup. A server without a result remains in **Unlocated servers**,
which also shows the geocoding error.

The map displays all continents except Antarctica and fits them into the window. It supports 100–600% zoom, reset and panning
while zoomed. Coincident points form clusters; clicking a cluster expands its servers.

### Server status and management

For an agent-based server, point color comes from the agent check:

- **green** — the agent is reachable and migrated resources have been adopted;
- **yellow** — transitional, degraded or unmanaged state;
- **red** — the connection or agent is unavailable.

For a topology-only server, the worst VPN service status is used: running is green, error is red, everything else is yellow.
The tooltip contains the address, coordinates, agent status and services. No native SVG alt-tooltip is layered over it.

Clicking a point opens **Manage panel** with these tabs:

- **Interfaces** — the specialized AWG editor;
- **Providers** — generic resources and actions;
- **Agent** — agent and provider installation;
- **Metrics** — system and peers;
- **Information** — SSH, TLS and metadata.

Agent and Providers are available only for agent-based inventory.

## Network Graph and tunnels

Graph displays interface nodes, server name/address, CIDR and any attached service. Nodes can be dragged, and positions are
saved. Minimap, zoom and pan are available.

### Creating and deleting a connection

Drag an arrow from one interface's source handle to another interface's target handle and confirm the route with **Create**.
Self-connections, duplicates and cycles are rejected.

For two agent-based AWG interfaces, a **provider link** is created:

1. The admin checks `linkKinds` and `remotePorts` in both manifests.
2. Both agents execute `prepare`, and the admin exchanges their offers.
3. Both sides execute `commit`; only then does the link become `Ready`.
4. On failure, the link remains `Degraded` with a condition instead of silently disappearing.

If either endpoint is topology-only, an abstract `NetworkConnection` is created. It is stored in Topology and passed to the
plugin on the next service **Apply**; by itself it does not call the agent provider API.

A selected arrow can be deleted with **Delete** or **Backspace** after confirmation. Deleting a provider link removes only its
generated tunnel peers and link-specific hooks/routes. The interface and user peers are preserved.

### Chains with multiple nodes

Provider links are not limited to the old “exactly two interfaces” wizard. You can build `A → B → C → D → E`; one intermediate
interface can be the previous link's target and the next link's source at the same time.

For AWG, a source receives a peer to the next node, `AllowedIPs = 0.0.0.0/0` and a separate policy-routing table. A target
receives the return peer and marking/NAT rules. On an intermediate node, both roles apply independently to their respective
links. Tables and marks derive from link IDs rather than one shared global value.

Host firewall configuration is handled separately through pack/host target, with firewalld, ufw and iptables support. VPN Admin
does not require you to repeat the manual v1 command examples.

### Moving a tunnel

An existing route can be changed without manually deleting and recreating it:

1. Select the arrow and press `m`.
2. Drag its start to replace **source**, or its end to replace **target**.
3. Review the old and new routes and click **Create**.

`Esc` exits move mode; **Cancel** in the modal leaves configuration unchanged. Duplicate, compatibility and cycle checks account
for all remaining links.

For a provider link, the admin first creates a replacement through the new nodes' agents, then removes the previous link. If
creation fails, it attempts to clean up the replacement while preserving the old tunnel. If old-link deletion fails, it attempts
to roll back the replacement. Cleanup and rollback errors are reported because some state may remain on the nodes. Traffic may
be briefly interrupted.

For an abstract `NetworkConnection`, the change is atomically written to Topology. Its actual application depends on the relevant
service plugin's **Apply**.

## Installing the agent and providers

### OCI registries and local binaries

**Manage panel → Agent** supports:

- **OCI release** — source, repository and tag; tags are loaded from the registry and sorted by semver;
- **Local binary** — a local `vpn-agent` path; desktop provides a file dialog, while in standalone the path refers to the admin
  server, not the computer running the browser.

The first database initialization adds `GitHub Container Registry — oci://ghcr.io`. This is a one-time operation controlled by a
`meta` marker, including during v1→v2 migration: a deleted source is not recreated.

**Add registry** saves a private registry name, host:port without a repository path, username/password and plain HTTP option.
The password is stored in BoltDB, omitted from list/get API responses and never shown again in the form; an empty password field
during editing preserves the old password. HTTPS is the default; enable plain HTTP only on a trusted network.

### Independent provider installation

Install the agent first. Then click **+** under **Installed providers** and select a catalog, provider and version. For official
GHCR, providers are discovered from `providers/<name>/v*` release tags in `ks-tool/vpn-agent`; a private registry must support
OCI `_catalog` for automatic listing. Repository and name can also be entered manually.

Each provider is installed by its own `pack apply`, independently of the agent. AWG exposes the provider-specific option
`{"userspace": true}`; the core does not impose this option on other providers. The **−** button removes a provider registration
through its pack. Installed names and versions come from `/v1/manifest`.

Operations are asynchronous: the UI polls deployment status and shows the current step or error. After success, the admin syncs
the server's inventory. The agent does not install providers or invoke pack itself.

### Managed host requirements

The host target supports **Debian and Fedora**, on **amd64 and arm64**. Deployment requires:

- SSH key authentication; password-only SSH is not supported by pack;
- root, or a non-root user with **passwordless sudo**;
- network access from the admin to SSH and from the managed host to the OCI registry through the temporary proxy;
- `/dev/net/tun` and `CAP_NET_ADMIN` for AWG userspace;
- a suitable `amneziawg` or `wireguard` module for AWG kernel mode; AmneziaWG 2.0 obfuscation requires a compatible module.

The admin calls `pack apply` as a Go library, without an external `pack` CLI. The `host` target is downloaded automatically.
During an operation, the target opens a separate SSH connection and starts a temporary host agent and OCI proxy on loopback. A
local self-deploy binary is shipped as pack content without an OCI download.

### Firewall and upgrading from awg-agent

The AWG pack enables `net.ipv4.ip_forward=1`. During provider installation and after a successful AWG interface change, the host
target adds input acceptance **by interface name**, such as `i:awg0`, through the available firewall backend. A separate
lightweight firewall pack does not download the provider again or restart the agent. There is no need to install iptables just
for these rules.

When installing the new `vpn-agent`, the pack first stops the legacy `awg-agent.service`. After the new agent starts and passes
readiness, the old unit and `/usr/local/bin/awg-agent` are removed. The upgrade is therefore performed from Manage panel, with
no manual server commands.

## Provider resources

The **Providers** tab combines resources from every provider in the agent manifest. Each resource shows provider/kind, phase,
observed generation and condition. Indicator colors are green for Ready, red for Degraded and yellow for Pending/Deleting.

The **+** button opens provider and kind selection. The `spec` form is built by RJSF from `specSchema`/`uiSchema`; an existing
resource's name and type are immutable. A boolean `disabled` property enables activate/deactivate without deletion.

The manifest can declare actions with their own argument schemas. VPN Admin supports:

- JSON results;
- downloadable files with names and media types;
- asynchronous results with `taskId`;
- `mutating` and `requiresConfirmation`.

AWG provides `Interface` and `Peer` resources, including client config rendering, key rotation/revocation and secret import for
migration. The generic editor does not replace provider validation: semantic errors are returned through status/conditions.

## AWG interfaces, users and peers

### The specialized interface editor

Agent-based AWG inventory retains a specialized editor under **Interfaces**, complementing the generic Providers UI. Main fields
are name, CIDR, client DNS, listen port, private key, activation and AmneziaWG parameters.

- An empty address on creation allocates the first host in the next free `/24` from `172.23.0.0/16`.
- An empty private key is generated automatically.
- Name, port and subnet overlap are checked against the server's other interfaces.
- Deactivation preserves configuration but takes the interface down.
- Clearing the Amnezia checkbox creates plain WireGuard; otherwise `Jc/Jmin/Jmax`, `S1–S4`, `H1–H4` and `I1–I5` control
  obfuscation.
- **Force replace peers** applies `ReplacePeers=true` and resets active sessions on that interface.

An interface with tunnel metadata cannot be edited or deleted through the specialized editor until its connection is removed.
Deleting a free interface cascades to its peers and asks for additional confirmation when peers exist.

`PreUp`, `PostUp`, `PreDown` and `PostDown` hooks run with the agent's privileges; `%i` expands to the interface name. An AWG
link uses generated hooks for policy routing and NAT and removes only its own commands.

### Users and client peers

The **Users** view manages global owners of AWG peers. A user has a name, description and enabled/disabled state; their peers
can be on different servers. This is a separate model from users of a particular plugin VPN service.

Creating a peer selects a server and interface. Device name, Allowed IPs, endpoint, DNS, private key and preshared key are
supported. An empty address is allocated from the interface pool, and an empty key is generated. A `/32` host must belong to the
subnet and be unique; a wider CIDR is allowed for site-to-site routes.

Peers offer a client `.conf` and QR code. Desktop saves files through native dialogs; standalone downloads them in the browser.
A peer can be deactivated without losing keys, or moved to another interface: keys, name, DNS and PSK are preserved. Its address
is kept if it fits the new subnet and is free; otherwise a new address is allocated. Both interfaces are synchronized.

### Importing wg-quick / awg-quick

A live interface created outside the agent appears under Interfaces as **Not managed** and read-only. **Import…** reads
`/etc/amnezia/amneziawg/<name>.conf`, then `/etc/wireguard/<name>.conf`.

Run **Preview** before importing: it shows the interface, users/peers, original and generated hooks and warnings without changing
anything. The peer map can associate existing client private keys with users; without it, peers survive but the admin cannot
recover their private keys for new client configs.

Import stores the inventory in BoltDB and pushes it to the agent. A live kernel link is adopted in place; userspace cannot adopt
somebody else's kernel link, so bring it down first and retry Sync. Afterwards, disable `wg-quick@<name>` or `awg-quick@<name>`
so the interface has only one owner. Disable `SaveConfig` beforehand, or stopping wg-quick will rewrite the file.

Preview checks `DNS`, `Table`, `AllowedIPs`, hooks and configuration constraints. Non-idempotent hooks require a clean migration
through `wg-quick down`; do not blindly apply them over already configured rules.

## The VPN service model and external plugins

In Topology, a service stores `serverId`, optional `interfaceId`, `pluginId`, name, opaque `config`, status and message. Saving
configuration moves it to `pending`; **Apply** calls the plugin and stores the returned status. Service users are created, listed,
updated and deleted through the same plugin.

The built-in `vpn-agent` plugin manages an AWG interface through an existing agent HTTP URL. It does not install the agent;
installation is a separate operation on the Agent tab.

An external plugin is a trusted executable. `~/.vpn-admin/plugins/<id>.json` contains:

```json
{
  "id": "example",
  "executable": "example-plugin"
}
```

A relative executable is resolved from the plugins directory. For every operation, the admin starts a new process, writes one
JSON request to stdin and expects one JSON response on stdout. Process protocol version is `1`; supported actions are:

```text
describe
apply
delete
status
users.list
users.create
users.update
users.delete
```

`describe` returns a descriptor with `configSchema`, `userSchema`, optional `uiSchema`, name and version. Other calls receive a
`deployment` containing the service, server, interface and connections. Errors are returned in `error`, or through stderr and a
nonzero exit code. The call timeout is 30 seconds. Go `.so` plugins are unsupported, so any implementation language can be used.

> **Current frontend limitation.** The model, HTTP API, RJSF forms and service inspector exist, but the add-plugin-service button
> and Network Graph node selection are not yet connected to the main workspace. Create services through
> `POST /topology/services` or the corresponding Wails binding. Do not confuse this model with the fully available **Providers**
> tab for agent resources.

A plugin runs with the admin's privileges and is not sandboxed. Install only trusted executables and restrict access to the
plugins directory.

## Metrics and tunnel latency

**Manage panel → Metrics** provides 1h/3h/6h/24h/all ranges and two tabs:

- **System** — host CPU, memory and network;
- **Peers** — online state, rx+tx activity and a sparkline for each peer.

The agent collects system metrics every 45 seconds by default (`VPN_AGENT_METRICS_INTERVAL`) and retains up to 48 hours with a
disk checkpoint. The AWG provider separately collects peer rx/tx/handshake (`VPN_PROVIDER_AWG_METRICS_INTERVAL`). Their APIs are
not merged:

```text
GET /metrics                         system, also supports ?fmt=prom
GET /metrics/history                 system history
GET /metrics/{interface}             interface and peers
GET /metrics/{interface}/history     interface history
```

The agent sends ICMP probes to tunnel peers' `probeAddress`, retains successful RTTs for 15 minutes and computes **p95**. Up to
eight probes run concurrently, each with a two-second timeout. The admin requests metrics from different source servers in
parallel, and the frontend refreshes them independently of the main graph.

Map arrow colors:

- **blue** — no p95 or p95 ≤100 ms;
- **yellow** — p95 >100 and ≤250 ms;
- **red** — p95 >250 ms.

The arrow tooltip shows the route and numeric p95. Missing ICMP replies do not create an artificially high latency: failed probes
simply do not enter the percentile.

Monitoring and profiling APIs remain available (`PATCH /servers/:id/monitoring`, `PATCH /servers/:id/profiling`,
`GET /servers/:id/profile`), but the old v1 switches are not part of the current Manage panel.

## Settings, security and backups

**Settings** contains:

- English/Русский;
- **Backup** in both modes;
- log viewing, export and debug logging only in desktop;
- Basic Auth, credential changes and logout only in standalone;
- application version.

Basic Auth is an additional gate over the login session using the same account; it is disabled by default. For a public
standalone installation, use HTTPS, replace admin/admin and optionally restrict access with a reverse proxy or firewall.

**Backup** exports the complete BoltDB database: servers, SSH/mTLS secrets, users and peers, topology/services/connections,
resources/links, OCI registry credentials and the admin password hash. Treat the JSON as secret.

Restore while the application is stopped:

```sh
./build/bin/awg-migrate export --db ~/.vpn-admin/db --out dump.json
./build/bin/awg-migrate import --db ~/.vpn-admin/db --in dump.json
```

Make a separate copy of the target database before importing. Never restore underneath a running process.

## Migrating from v1

Schema migration never runs automatically at startup. Stop v1 and create a new database:

```sh
make migrate
./build/bin/awg-migrate v2 --db ~/.awg-admin --out ~/.vpn-admin/db
```

The source is opened read-only and the result is written atomically. An existing destination is replaced only with `--force`.

After starting v2, migrated servers and interfaces appear in Map/Graph as **Unmanaged**. On each server, install the v2 agent and
then the AWG provider. During the status check, the admin submits migrated resources and links to the provider; unmanaged clears
only after successful adoption. The original `~/.awg-admin` database is unchanged and serves as the rollback copy.

For complete guarantees, conversion details, an already-v2 source and rollback instructions, see
[docs/migrate_v2.md](migrate_v2.md).

## HTTP API

Standalone routes are registered in `internal/api/handler.go`; desktop calls the same service layer through Wails bindings. Main
groups:

| Group | Purpose |
|---|---|
| `/auth/*`, `/backup`, `/version` | Session, credentials, Basic Auth, export and version. |
| `/topology/*` | Servers/interfaces/services/plugin users/connections/positions/plugins/tunnel metrics. |
| `/servers/*` | Agent inventory, SSH/TLS, deployment, sync, metrics and interfaces. |
| `/servers/:id/providers/*` | Manifest, schemas, resources, status and actions. |
| `/links/*` | Provider links: list/create/apply/move/delete. |
| `/artifacts/*`, `/registry-sources/*` | OCI tags, provider catalog and saved registries. |
| `/users/*` | Global users and AWG peers. |
| `/tunnels/*` | Compatibility with the old two-node model; use Graph/links for new routes. |

All operational routes except login/logout require a standalone session. Moving uses `PUT /links/:linkID` and
`PUT /topology/connections/:connectionID`; plugin service creation uses `POST /topology/services`.

## Troubleshooting

- **The server is yellow on the map, but SSH works.** Check that the agent is running, the AWG provider appears in
  `/v1/manifest`, and migrated resources have left Pending/Unmanaged.
- **No coordinates.** Check Location and `locationError`. If the city is absent from Atlas, clear Location and enter both
  coordinates manually. If the network is unavailable, a previously saved `datacenter-atlas.json` is used.
- **No providers from a private registry.** Check `_catalog`, credentials and prefix. Alternatively, enter provider/repository
  manually; tags still require OCI `/tags/list`.
- **Pack rejects SSH.** A key is required, and non-root users need passwordless sudo. Password SSH alone is insufficient.
- **Resource Degraded.** Open the condition message and check provider generation, `vpn-agent` logs and AWG kernel module
  compatibility. Desired state is retained for another reconciliation.
- **Link Degraded after moving.** Check the error on both nodes. Cleanup and rollback are best-effort; if rollback reports an
  error, inspect old and replacement peers/hooks before retrying.
- **A blue arrow has no p95.** This means there are no successful ICMP probes, not guaranteed zero latency. Check `probeAddress`,
  ICMP and monitoring state.
- **Detailed desktop logs are needed.** Select Settings → Logs → Debug, reproduce the operation and save the JSON. In standalone,
  inspect process stdout/stderr and agent logs on the host.
