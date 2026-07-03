# Amnezia Admin — User Guide

[Русская версия](GUIDE_ru.md) · [← README](../README.md)

This document is a complete guide to installing, running, and operating Amnezia Admin. The
[README](../README.md) describes the project itself — its purpose, architecture, and how to build it from source; this
guide covers everything required to run the application and use its features.

## Contents

- [Installation and running](#installation-and-running)
    - [Desktop application](#desktop-application)
    - [Standalone server](#standalone-server)
    - [Standalone configuration](#standalone-configuration)
    - [Data directories](#data-directories)
    - [The agent](#the-agent)
    - [Agent configuration](#agent-configuration)
- [How it fits together](#how-it-fits-together)
- [Servers](#servers)
    - [Adding a server](#adding-a-server)
    - [SSH authentication](#ssh-authentication)
    - [Reaching the agent: SSH tunnel vs mTLS](#reaching-the-agent-ssh-tunnel-vs-mtls)
    - [The agent dialog](#the-agent-dialog)
    - [Deploying the agent](#deploying-the-agent)
    - [Agent sources (deploy presets)](#agent-sources-deploy-presets)
    - [Sync](#sync)
    - [Reconcile (agent ↔ database)](#reconcile-agent--database)
    - [Monitoring toggle](#monitoring-toggle)
    - [Profiling](#profiling)
- [Interfaces](#interfaces)
- [Users and peers](#users-and-peers)
- [Multi-hop tunnels (exit nodes)](#multi-hop-tunnels-exit-nodes)
- [Dashboard and metrics](#dashboard-and-metrics)
- [Settings](#settings)
    - [Backup](#backup)
    - [Logs and debug mode (desktop)](#logs-and-debug-mode-desktop)
    - [Login, credentials and Basic Auth (standalone)](#login-credentials-and-basic-auth-standalone)
- [Backup, restore and migration](#backup-restore-and-migration)
- [Data and secret locations](#data-and-secret-locations)

## Installation and running

Amnezia Admin can be deployed either as a desktop application or as a standalone web server. Both deployment modes use
the same data model and business logic and differ only in how the application is accessed.

### Desktop application

The desktop application is intended for local administration and does not require any additional services. All
configuration is stored locally, and communication with managed servers is performed directly from the application.

Prebuilt binaries are available from the project's [Releases](../../../releases) page. The following packages are
provided for each supported operating system:

- **Windows** — `amnezia-admin-amd64-installer_<version>.exe` (NSIS installer) or the portable executable
  `amnezia-admin_<version>.exe`.
- **macOS** — `amnezia-admin_<version>.dmg`, containing a universal build for both Intel and Apple Silicon systems. The
  application is not signed or notarized with an Apple Developer ID certificate. On first launch, either open it through
  Finder's **Open** context menu or remove the quarantine attribute manually:

```sh
xattr -cr "/Applications/Amnezia Admin.app"
```

- **Linux** — `amnezia-admin_<version>`. The application requires the `libgtk-3` and `libwebkit2gtk-4.1` system
  libraries.

The desktop application does not expose a network API and therefore does not require authentication.

### Standalone server

The standalone deployment mode exposes the administrative interface as a web application and is intended for server
installations or headless environments. By default, the server listens on `:8080`.

Run the binary directly:

```sh
./awg-admin
```

Run using Docker:

```sh
docker run -d --name awg-admin \
  -p 8080:8080 \
  -v awg-admin-data:/data \
  ghcr.io/<owner>/<repo>:latest
```

The default credentials are:

- **Username:** `admin`
- **Password:** `admin`

These credentials should be changed immediately after the initial login, either through **Settings** or via the
authentication API:

```http
PUT /auth/change-credentials
```

Authentication is only required when running the standalone server. The desktop application operates entirely on the
local machine and is not accessible over the network.

### Standalone configuration

The standalone server is configured through environment variables.

| Variable                                   | Description                                                                                                                                                                         |
|--------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `AWG_ADMIN_ADDR`                           | HTTP listen address. Defaults to `:8080`.                                                                                                                                           |
| `AWG_ADMIN_TLS_CERT` / `AWG_ADMIN_TLS_KEY` | TLS certificate and private key used for HTTPS.                                                                                                                                     |
| `AWG_ADMIN_AUTOCERT_DOMAINS`               | Comma-separated list of domains for automatic Let's Encrypt certificate provisioning. Cannot be used together with a static certificate. Requires port 80 to be publicly reachable. |
| `AWG_ADMIN_AUTOCERT_CACHE_DIR`             | Directory used to store automatically issued certificates. Defaults to `$HOME/.awg-admin-autocert`.                                                                                 |

### Data directories

The standalone server stores all runtime data under the user's home directory. By default, the following locations are
used:

| Path                        | Purpose                                                     |
|-----------------------------|-------------------------------------------------------------|
| `$HOME/.awg-admin`          | BoltDB database containing the infrastructure configuration |
| `$HOME/.awg-admin-autocert` | Cache of automatically issued TLS certificates              |
| `$HOME/.awg-admin-cache`    | Cache of downloaded agent binaries                          |

Inside the official Docker image, `$HOME` is mapped to `/data`. This directory is intended to be mounted as a persistent
Docker volume.

### The agent

The agent is a separate Linux executable responsible for managing WireGuard and AmneziaWG interfaces on the target host.
In most deployments, the administrative application installs and configures the agent automatically over SSH (see
[Deploying the agent](#deploying-the-agent)). Manual startup is also supported:

```sh
AWG_AGENT_ADDR=127.0.0.1:8080 AWG_AGENT_DB=/var/lib/awg-agent ./awg-agent
```

After startup, the agent exposes its management API and waits for connections from the administrative application. By
default, the API is bound to the loopback interface (`127.0.0.1`) and is expected to be accessed through an SSH tunnel.
Direct mTLS connections are supported when SSH tunneling is not available or not desired.

### Agent configuration

| Variable                                                               | Description                                                                  |
|------------------------------------------------------------------------|------------------------------------------------------------------------------|
| `AWG_AGENT_ADDR`                                                       | HTTP listen address. Defaults to `127.0.0.1:8080`.                           |
| `AWG_AGENT_DB`                                                         | Directory used to store the local interface configuration.                   |
| `AWG_AGENT_TLS_CERT` / `AWG_AGENT_TLS_KEY` / `AWG_AGENT_TLS_CLIENT_CA` | Enables mutual TLS authentication when all three variables are provided.     |
| `AWG_AGENT_METRICS_INTERVAL`                                           | Interval between system and WireGuard metrics collection. Defaults to `45s`. |
| `AWG_AGENT_LOG_LEVEL`                                                  | Logging level. Defaults to `info`.                                           |

The agent is available for Linux only and is distributed for both `amd64` and `arm64` architectures.

## How it fits together

Amnezia Admin manages four types of object, nested within one another:

- A **server** is a remote machine accessible over SSH. Amnezia Admin stores its connection details and, once the agent
  has been deployed, communicates with the agent running on it.
- An **interface** is a WireGuard/AmneziaWG network interface on a server (`wg0`, `awg0`, and so on). A server may have
  several.
- A **user** is a person or device owner. Users exist independently of servers.
- A **peer** is a single VPN client belonging to a user and attached to one interface on one server. Peers are
  distributed to clients as configuration files or QR codes.

The administrative application stores all of this — together with SSH credentials, keys, and assignments — in a local
embedded database (BoltDB) and treats it as the single source of truth. The agent only applies the interface
configuration pushed to it and never maintains desired state of its own. Every change is saved locally first and then
pushed to the relevant agent on a best-effort basis: a temporarily unreachable agent never blocks an edit — the
interface is marked *out of sync* until the next successful push or a manual **Sync**.

## Servers

### Adding a server

A server is added from the **Servers** page. At minimum it requires an SSH host and an agent address. The optional
metadata — **name**, **description**, **location**, and **tags** — is used only for organization and search.

Creating a server does not connect to it: the form only records the connection details, so servers may be added before
they become reachable. Connection errors (an invalid key, an incorrect passphrase, an unreachable host) are surfaced
later — during **Deploy**, **Sync**, or **Update** — rather than at creation time.

### SSH authentication

Amnezia Admin connects to a server over SSH to deploy the agent and, by default, to tunnel to its API. The following
credential types are supported:

- **Private key file** — a path to a key on the machine running Amnezia Admin.
- **Uploaded key** — the key contents supplied directly and stored in the database. This is the portable option: it
  behaves identically in the desktop application and in a browser tab and does not depend on a file path that exists on
  only one machine. When both a path and uploaded contents are provided, the uploaded contents take precedence.
- **Password** — plain SSH password authentication.

**Passphrase-protected keys.** When a key requires a passphrase, the first operation that connects to the server prompts
for it. The passphrase is cached in memory for the lifetime of the process and is never written to disk, so it is
requested only once per run. The *Use for all connections* option reuses it as a fallback for other servers whose keys
share the same passphrase.

Host keys are not verified (there is no pre-shared `known_hosts`), consistent with how lightweight server-management
tools behave on first connection.

### Reaching the agent: SSH tunnel vs mTLS

Once the agent is running, the administrative application communicates with its HTTP API in one of two ways:

- **SSH tunnel (default).** The agent listens on loopback (`127.0.0.1:8080`) and is never exposed to the network; the
  application reaches it through a long-lived SSH connection kept open for the whole session. No additional
  configuration is required — providing the server's SSH details is sufficient.
- **Direct mTLS.** Intended for an agent that is reached directly on a public IP without SSH. The **Generate TLS**
  action
  on the server page issues a CA, server, and client certificate set; the agent is then run with those certificates and
  verifies the application's client certificate on every request. This is appropriate when there is no convenient SSH
  path or when the application and agent should be fully decoupled from SSH.

The connection is resilient: if a tunnel drops silently (a host reboot or a network interruption), the application
detects it and reconnects automatically on the next call.

### The agent dialog

All operations performed on the agent itself — as opposed to the interfaces and peers it carries — are collected in a
single **agent** dialog rather than spread across the page. It is opened from the **wrench** button on the server page
or
the **gear** button on each dashboard row. The top of the dialog shows the agent's reported **version** and capabilities
(backend, Docker, interface kinds); below that are all agent actions: [deployment](#deploying-the-agent),
[synchronization](#sync), [reconciliation](#reconcile-agent--database), the [monitoring toggle](#monitoring-toggle), and
[profiling](#profiling). Each is described in the sections below.

### Deploying the agent

The **Deploy agent** action in the agent dialog installs and starts the agent over SSH in a single step: a
cross-compiled Linux binary is uploaded and launched automatically. After a deployment, the application re-pushes every
interface for that server so the host comes up in the desired state. A live status area reports progress and any
failure.

In the desktop application, a local agent binary may also be selected from disk through a native file picker instead of
relying on a downloaded one.

#### Server prerequisites

Two choices are independent: the agent **backend** (kernel or userspace — how the interface link is created) and the
**deployment method** (systemd or Docker, determined by the [agent source](#agent-sources-deploy-presets) kind — a
**URL** or **local file** source installs a binary as a **systemd** service, while an **image** source runs a
**container**). The server requirements depend on the combination used:

- **`awg-agent` — kernel backend (systemd).** Drives the AmneziaWG kernel module directly and therefore requires it to
  be installed on the server:
    - the **AmneziaWG kernel module** for AmneziaWG interfaces — install `amneziawg-dkms` as described in the
      [amneziawg-linux-kernel-module README](https://github.com/amnezia-vpn/amneziawg-linux-kernel-module/blob/master/README.md)
      and load it (`modprobe amneziawg`). The deployment checks for this in advance and fails with a clear message if
      the
      module is missing;
    - **WireGuard** itself, if plain (non-Amnezia) interfaces are also created — see
      [wireguard.com/install](https://www.wireguard.com/install/).
- **`awg-agent-userspace` — userspace backend (systemd or Docker).** An in-process userspace WireGuard implementation
  (the amneziawg-go library is compiled in) that requires no kernel module, intended for hosts where one cannot or
  should
  not be installed. It can be deployed in either of two ways:
    - **as a binary through systemd** — using a **URL** or **local file** agent source that points to the
      `awg-agent-userspace` binary with its **Userspace agent (no kernel module)** option enabled. It is installed as a
      systemd service exactly like the kernel agent and requires only `/dev/net/tun` (standard on Linux); Docker is not
      involved. The option instructs the deployment to skip the kernel-module pre-check that otherwise applies to the
      kernel agent.
    - **as a container** — using an **image** agent source. The deployment runs the container with `/dev/net/tun` and
      `NET_ADMIN` and checks `docker info` in advance. Only **Docker** is required — see
      [docs.docker.com/engine/install](https://docs.docker.com/engine/install/).

Both builds expose an identical API; only the method of creating the interface link differs. Versions must match: an
AmneziaWG 1.0 kernel module rejects 2.0-style obfuscation parameters (`H1–H4` ranges, `I1–I5`). In that case, upgrade
the
module to 2.0, or deactivate the interface (see [Interfaces](#interfaces)) to keep its configuration without applying
it.

Once running, the agent detects what its host actually supports — Docker, the AmneziaWG kernel module, the interface
kinds it can create, and whether it is running in a container — and reports this back; the
[Dashboard](#dashboard-and-metrics) **agent type** column surfaces it.

### Agent sources (deploy presets)

An **agent source** is a reusable, named preset that describes where to obtain the agent binary (for example, a release
URL), optionally cached locally so that repeated deployments do not download it again. Agent sources are managed from
the
source dropdown in the deployment section of the agent dialog: **Add new…** opens a small form to create one, and each
saved entry provides an **edit** (pencil) action to view and change its URL, path, image, and options, along with
cache-refresh and remove actions. This is useful when deploying to many servers or when pinning a specific agent
version.

A source is one of three kinds — a **URL**, a **local file path**, or a **Docker image** (the last deploys the userspace
agent as a container). If the agent has already reported that the server has no usable Docker, image sources are not
offered for it: they are hidden from the dropdown and the **Docker image** option is removed from the creation form,
because the container deployment cannot run there. Before the first deployment the server capabilities are unknown, so
all options are offered and the deployment's own Docker pre-check serves as the backstop.

### Sync

**Sync** re-pushes every interface recorded for a server to its agent, overwriting the agent's current state with the
desired state from the database. It runs automatically after a deployment and after a tunnel reconnects; the manual
action is provided for forcing the agent back into line — for example, after the agent has been reinstalled or when
drift
is suspected.

### Reconcile (agent ↔ database)

**Reconcile** compares what the agent actually has configured against what the database records, matching by interface
name, and reports the differences so that each can be resolved manually. Two kinds of mismatch are possible:

- **Present on the agent but not in the database** (for example, after the database was restored from an older backup):
  the interface can be **imported** into the database or **deleted from the agent**.
- **Present in the database but not on the agent** (for example, after the agent lost its storage or was reinstalled):
  the interface can be **re-pushed** to the agent or **deleted from the database**.

When both sides agree, reconciliation reports that no action is required. Note that importing recovers only the
interface
itself (address, keys, AmneziaWG parameters, and the peers the agent reports); the association between a peer and the
user it belongs to exists only in the administrative database and cannot be reconstructed from the agent.

### Monitoring toggle

Each server has a monitoring switch that enables or disables the agent's metrics collection at runtime. The desired
state is stored and re-applied on redeployment, so a server whose monitoring has been disabled remains so after the
agent
restarts.

### Profiling

For diagnosing a misbehaving agent, **profiling** exposes the agent's Go runtime profiling (`net/http/pprof`). It is
disabled by default, because sampling adds overhead and the endpoints expose internal state, and is toggled per server
from the agent dialog; as with monitoring, the desired state is re-applied on redeployment. While profiling is enabled,
a
small activity icon appears next to the server name (on the dashboard and the server page) as a reminder.

With profiling enabled, the **Download dump** action retrieves a profile from the agent for inspection with
`go tool pprof` or `go tool trace`. The available kinds are `goroutine` (stuck or leaking goroutines), `heap` (memory),
the CPU `profile` and an execution `trace` (both sampled over a chosen number of seconds), and the standard `allocs`,
`block`, `mutex`, and `threadcreate` profiles. The desktop application saves the dump through a native dialog; the web
application downloads it in the browser. Profiling should be disabled again once diagnosis is complete.

## Interfaces

The server page allows WireGuard/AmneziaWG interfaces to be created, edited, and deleted. The form has two tabs.

The **General** tab:

- **Name** — the interface name (`wg0`, `awg0`, and so on). Required.
- **Address** — the interface's own address in CIDR form (for example, `10.0.0.1/24`). Optional: when left blank,
  Amnezia
  Admin assigns the first host of the next free `/24` from the `172.23.0.0/16` pool, chosen so that it does not overlap
  any existing interface. On edit, leaving it blank keeps the current address, so the interface is not moved to a new
  subnet and its peers are not orphaned.
- **Listen port** — the UDP port the interface listens on (default `51820`).
- **Private key** — left blank to generate one automatically.
- **Activate interface** — a checkbox, enabled by default. While enabled, the agent brings the interface up and raises
  it
  again on every agent restart. Disabling it keeps the stored configuration but instructs the agent to take the
  interface
  down; the interface then shows a **Disabled** badge in the list. This allows an interface to be parked without
  deleting
  it.

The **Amnezia** tab contains the `Jc/Jmin/Jmax`, `S1–S4`, `H1–H4`, and `I1–I5` junk-packet and header-obfuscation values
that make AmneziaWG traffic harder to fingerprint:

- **Amnezia Interface** — a checkbox at the top of the tab, enabled by default. While enabled, all obfuscation
  parameters
  below are pre-filled with freshly generated values (the same set the application would otherwise apply), and every
  field remains editable for manual tuning.
- Disabling it creates a plain **WireGuard** interface instead: the obfuscation parameters are then ignored and the
  interface behaves as standard WireGuard.

On edit, the tab reflects the interface's current configuration — the checkbox is enabled (showing the stored values)
for
an AmneziaWG interface and disabled for a plain WireGuard one.

A new or edited interface is validated against the server's existing interfaces: its **name**, **listen port**, and
**subnet** (derived from the address) must each be free. A duplicate name or port, or a subnet that overlaps another
interface, is rejected with the reason shown. On edit, a field that was not changed is not re-checked.

Interfaces that are part of a [tunnel](#multi-hop-tunnels-exit-nodes) are locked against editing and deletion in the
user
interface; the tunnel must be removed first.

Deleting an interface also deletes all of its peers, since a peer cannot exist without its interface. When the interface
has peers, the user interface requires confirmation twice, and the first dialog states how many peers will be removed.
The removed peers also stop appearing in the [peer metrics](#dashboard-and-metrics).

Every interface tracks an **in-sync** status: whether the last push to the agent succeeded, the error if it did not, and
the time of the last successful synchronization.

The agent raises only active interfaces on startup and takes every active interface down when it stops, so a stopped
agent leaves no tunnels running. A deactivated interface is never brought up and its configuration is stored but not
applied — which also makes deactivation a way to park an interface that the agent would otherwise fail to configure.

Unless it is addressed with IPv6, each interface is brought up with **IPv6 disabled** (the tunnels here are IPv4-only),
so it never receives an automatically assigned link-local IPv6 address and cannot carry or leak IPv6 traffic. An
interface deliberately given an IPv6 address is left unchanged. This is best-effort: where the agent cannot apply the
setting — notably the userspace agent's Docker container, which runs with a read-only `/proc/sys` — the interface still
comes up and a warning is logged.

### Hook commands

Each interface supports **PreUp**, **PostUp**, **PreDown**, and **PostDown** hook commands, which run on the agent when
the interface comes up or goes down — the equivalent of wg-quick's hooks. Because the agent does not run wg-quick,
anything such as custom routing or `iptables` rules is expressed as hook commands. In a command, `%i` expands to the
interface name. These hooks are what the tunnel feature uses internally to set up policy routing and NAT.

## Users and peers

**Users** (the *Users* page) are the people to whom peers belong. A user has a **name**, an optional **description**,
and
can be **disabled**. Users are server-independent: one user may have peers on several servers.

A **peer** is a single VPN client belonging to a user. When adding a peer:

- **Name** — a label for the peer (a device name, for example).
- **Server** and **interface** — where the peer is attached.
- **Allowed IPs** — the peer's addresses; left blank to auto-assign a free host address on the interface's subnet. A
  supplied host address (a `/32`) must lie inside the interface's subnet and must not already be taken by another peer
  or
  the interface itself; out-of-subnet and duplicate addresses are rejected. Broader routed CIDRs — a LAN behind a
  site-to-site peer, such as `192.168.1.0/24` — are permitted outside the subnet.
- **Endpoint** — optional, for peers that should be dialed at a fixed address.
- **DNS** — optional client-side DNS for this peer's generated configuration (one or more servers separated by commas,
  for example `1.1.1.1, 8.8.8.8`). It becomes the `DNS = …` line in the client configuration; left blank, it falls back
  to the interface's DNS. It is a client-side setting and is never pushed to the agent.
- **Private key** — optional; left blank, a fresh key is generated for the peer. It is filled in only to import an
  existing key.
- **Pre-shared key** — an optional additional symmetric key for extra hardening. The *Generate preshared key* option is
  enabled by default, creating a random key. Disabling it reveals a field for pasting a custom key, or that field may be
  left empty to create the peer without a pre-shared key.

For each peer, the application provides:

- **Config** — the ready-to-use `wg-quick`/AmneziaWG client configuration text, with the peer's keys, the server
  endpoint, and the AmneziaWG obfuscation parameters filled in.
- **QR code** — the same configuration as a scannable QR code for the mobile applications, with an action to save it as
  a
  PNG file (a native save dialog in the desktop application).

A peer can be moved to another interface using the move action (⇄) on the peer, followed by selecting a target on any
server: its keys, name, DNS, and pre-shared key are preserved. Its address is kept when it still fits the target
interface's subnet and is free there — for example, when moving between the two members of a tunnel, which share a
subnet — otherwise a free address on the target is assigned. Both interfaces are re-pushed.

A peer can be deactivated using the toggle (⏻) on the peer, without being deleted. A deactivated peer keeps its stored
configuration — keys, address, and pre-shared key — but is removed from the live interface on the agent, so that client
can no longer connect until it is reactivated. This mirrors the interface-level *Activate* switch, applied to a single
client, and is useful for temporarily revoking access without discarding the peer or its address. Its configuration and
QR code remain available, but a deactivated peer is not present on the device and therefore reports no traffic in the
metrics.

Deleting a peer revokes it on the agent (removing it from the interface) and drops it from the peer metrics, so it no
longer appears in the metrics history.

## Multi-hop tunnels (exit nodes)

A **tunnel** chains two servers so that clients connected to the first reach the internet through the second. The
typical
use is a **relay** (server 1) that clients connect to and an **exit node** (server 2) whose public IP the internet sees.

A tunnel is built from the **Tunnels** page using a wizard:

1. **Entry** — select the server and interface that clients already connect to (the relay). It must have a listen port
   and a CIDR address.
2. **Exit** — select a second server and interface to become the dedicated exit. Its configuration is replaced to serve
   this role.
3. **Review** — the tunnel's shared subnet is shown for confirmation. It is always the entry interface's own subnet (the
   exit is placed on it), not a value chosen manually.

Amnezia Admin reconfigures both interfaces and pushes them: the relay receives policy-routing rules and a gateway peer
pointing at the exit; the exit receives NAT (masquerade) and dials back to the relay. Clients require no configuration
change — their existing tunnel to the relay now exits through the second server.

Because the exit sits on the entry's subnet, the two members share a single address pool: a peer added at either end
receives an address that is unique across both. Client peers may be added to the exit interface as well as the relay.

Removing a tunnel reverts both interfaces to an empty state (clearing the added peers, hooks, and routing) rather than
deleting them, so the interfaces remain available for reuse. While a tunnel exists, its two interfaces cannot be edited
or deleted individually, nor can the servers that hold them; the tunnel must be removed first.

## Dashboard and metrics

The **Dashboard** shows totals (servers, peers, users, tunnels) and a per-server table with an **agent status** badge,
an
**agent type** column, a **peers** count, the **load average (1/5/15)**, and **RAM %**. Selecting a server opens the
**metrics modal** with two tabs:

- **System** — host CPU, RAM, load, and network over time.
- **Peers** — a per-peer activity table with a compact sparkline of each peer's traffic, labelled `<user>/<peer>`.

The **agent status** badge is a tri-state health check that probes the agent and combines the result with the state of
the connection to it:

- **Green** — the agent is reachable and answering (for an SSH-tunnelled server, this also confirms that the tunnel is
  up).
- **Red** — the connection is down: the SSH tunnel could not be established, or an mTLS agent reached directly is
  unreachable.
- **Amber** — an intermediate state, for example when the SSH tunnel is up but the agent behind it does not respond.

A grey *unknown* state is shown until the first check completes. Any non-green state is logged at error level, so
problems are visible in the agent and admin logs.

The **agent type** column reflects what the agent detected about its host at startup: its backend build (`kernel` or
`userspace`), a **docker** indicator when the agent itself runs in a container, and the interface kinds it can create
there (`awg` for AmneziaWG, `wg` for plain WireGuard). Hovering over it shows the full detail — whether Docker is
available on the host, whether the AmneziaWG kernel module is present, and the agent version. This provides an
at-a-glance view of whether a server can obfuscate traffic and how its agent is running. As with metrics, it is
best-effort: the last known values are shown while the agent is briefly unreachable, and `—` until the first successful
read.

Metrics are produced by the agent, which samples the host and every peer's rx/tx/handshake on an interval (default 45s,
configurable via `AWG_AGENT_METRICS_INTERVAL`) into in-memory ring buffers, retaining up to 48 hours of history. The
history is checkpointed to disk on the agent, so charts survive an agent restart. Metrics are best-effort: a server
whose
agent is briefly unreachable shows the last known values, and reachability is signalled separately by the agent-status
badge.

The agent can also expose metrics in **Prometheus** text format (its `/metrics?fmt=prom` endpoint) for scraping into an
external monitoring system.

## Settings

The **Settings** page adapts to the deployment mode:

- **Language** — English or Russian. Available in both modes.
- **Backup** — described below. Available in both modes.
- **Logs** and **Debug mode** — desktop only.
- **Basic Auth** and **Change credentials** — standalone only.
- **Version** — the application build version.

### Backup

**Save backup** downloads a full snapshot of the administrative database — servers, users, peers, interfaces, and
credentials — as a JSON file. In the desktop application it opens a native save dialog; in the standalone server it
downloads through the browser. The file uses the same portable format as the `awg-migrate` tool and can be restored with
`awg-migrate import` (there is no in-application restore, since that would replace the database beneath the running
process — see [Backup, restore and migration](#backup-restore-and-migration)).

> The backup contains secrets — SSH private keys, agent mTLS keys, peer pre-shared keys, and the admin password hash.
> Store it securely.

### Logs and debug mode (desktop)

The desktop application captures its own log output in memory. **View logs** shows the recent log lines and allows them
to be saved to a JSON file, which is useful for bug reports. The **Debug mode** checkbox enables debug-level logging at
runtime; it is disabled by default to avoid flooding the log. The intended workflow is to enable it, reproduce the
issue,
and then refresh to view the detailed lines. There is no log buffer in standalone mode, so this section is desktop-only.

### Login, credentials and Basic Auth (standalone)

The standalone web server is the only mode with authentication; the desktop application runs locally with no network
surface to protect.

- **Login** — a single admin account, seeded to **admin / admin** on first run. It should be changed immediately.
- **Change credentials** — sets a new username and password.
- **HTTP Basic Auth** — an optional additional gate: when enabled, every request (including the login page itself) also
  requires an HTTP Basic authentication prompt using the same admin account. This is useful when exposing the server
  directly without a reverse proxy. Disabled by default.

## Backup, restore and migration

The entire administrative state resides in a single BoltDB file (`~/.awg-admin`). Two tools operate on it, sharing the
same portable JSON dump format:

- **In-application Backup** (Settings → Backup) — a one-step export of the current state to a JSON file.
- **`awg-migrate`** — the command-line export/import tool, used to move data between machines or between the desktop and
  standalone modes (both read the same file format):

  ```sh
  awg-migrate export -db ~/.awg-admin -out dump.json   # on the source installation
  awg-migrate import -db ~/.awg-admin -in dump.json    # into the target installation
  ```

The exported document contains the complete administrative configuration — managed servers, SSH credentials, users, VPN
peers, and their relationships. Because both deployment modes use the same data model, the same export can migrate
between desktop and standalone installations or move the application to another machine.

A file saved by the in-application **Backup** is a valid `awg-migrate` dump. To restore a backup, import it with
`awg-migrate` into a fresh or target database while the application is stopped, and then start the application.

## Data and secret locations

- **Administrative database** — `~/.awg-admin` (BoltDB). It contains everything: servers, SSH credentials and keys,
  users, peers, interface configurations, and the standalone admin account. This is the one item that must be backed up.
- **Autocert cache** (standalone HTTPS) — `~/.awg-admin-autocert`.
- **Agent-binary cache** (deploy presets) — `~/.awg-admin-cache`.
- **Agent, per server** — stores only the interface configurations pushed to it (`AWG_AGENT_DB`, default
  `/var/lib/awg-agent`) together with its metrics-history checkpoint. It holds no administrative secrets beyond the
  interface and peer keys required to run WireGuard.

In the standalone Docker image, `$HOME` is mapped to `/data`, so mounting that volume persists all of the above.

Interface hook commands run on the agent host with the agent's privileges (typically root, since it manages network
interfaces), by design and exactly like wg-quick hooks. They originate from the administrator over the authenticated
channel.
