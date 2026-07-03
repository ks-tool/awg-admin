# Amnezia Admin — User Guide

[Русская версия](GUIDE_ru.md) · [← README](../README.md)

A complete walkthrough of everything Amnezia Admin can do. For installation and
running instructions see the [README](../README.md); this document covers the
features once it's up.

## Contents

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
- [Where your data and secrets live](#where-your-data-and-secrets-live)

## How it fits together

Four object types, nested:

- A **server** is a remote box you have SSH access to. Amnezia Admin stores its
  connection details and, once deployed, talks to the **agent** running on it.
- An **interface** is a WireGuard/AmneziaWG network interface on a server
  (`wg0`, `awg0`, …). A server can have several.
- A **user** is a person (or device owner). Users exist independently of
  servers.
- A **peer** is one VPN client belonging to a user, attached to one interface
  on one server. Peers are what you hand out as client configs / QR codes.

The admin app keeps all of this — plus SSH credentials, keys and assignments —
in a local embedded database (boltdb). It is the **source of truth**: the agent
only ever applies the interface config it's pushed, and never stores desired
state of its own. Every change you make is saved locally first, then pushed to
the relevant agent on a best-effort basis (a temporarily unreachable agent
never blocks the edit — the interface is just marked "out of sync" until the
next successful push or **Sync**).

## Servers

### Adding a server

*Servers → add*. A server needs, at minimum, an SSH host and an agent address.
Optional metadata — **name**, **description**, **location**, **tags** — is
purely for your own organization and search.

Creating a server never dials it: the form just records the details, so you
can add servers before they're reachable. Connection errors (bad key,
wrong passphrase, host down) surface later, when you **Deploy**, **Sync** or
**Update** — not on create.

### SSH authentication

Amnezia Admin reaches a server over SSH to deploy the agent and (by default) to
tunnel to its API. Supported credentials:

- **Private key file** — a path to a key on the machine running Amnezia Admin.
- **Uploaded key** — paste or upload the key's contents directly; stored in the
  database and used verbatim. This is the portable option: it works identically
  in the desktop app and a browser tab, and doesn't depend on a file path that
  only exists on one machine. When both a path and uploaded contents are set,
  the uploaded contents win.
- **Password** — plain SSH password auth.

**Passphrase-protected keys.** If a key needs a passphrase, the first operation
that dials the server prompts for it. The passphrase is cached **in memory for
the lifetime of the process** (never written to disk), so you're only asked
once per run. "Use for all connections" reuses it as a fallback for other
servers whose keys share the same passphrase.

Host keys are **not** verified (there's no pre-shared `known_hosts`), matching
how lightweight server-management tools behave on first connect.

### Reaching the agent: SSH tunnel vs mTLS

Once the agent is running, the admin app talks to its HTTP API in one of two
ways:

- **SSH tunnel (default).** The agent listens on loopback (`127.0.0.1:8080`)
  and is never exposed to the network; the admin app reaches it through a
  long-lived SSH connection it keeps open for the whole session. Nothing extra
  to configure — if you gave the server SSH details, this just works.
- **Direct mTLS.** For an agent you'd rather reach directly on a public IP
  without SSH. On the server page, **Generate TLS** issues a CA + server +
  client certificate set; the agent is then run with those certs and verifies
  the admin app's client certificate on every request. Use this when there's no
  convenient SSH path, or you want the admin app and agent fully decoupled from
  SSH.

The connection is resilient: if a tunnel silently drops (host reboot, network
blip), the admin app detects it and reconnects automatically on the next call.

### The agent dialog

Everything you do *to the agent* — as opposed to the interfaces/peers it carries
— lives in one **agent** dialog, so it's in a single place rather than scattered
across the page. Open it from the **wrench** button on the server page, or the
**gear** on each dashboard row. At the top it shows the agent's reported
**version** and capabilities (backend, Docker, interface kinds); below that are
all the agent actions: [deploy](#deploying-the-agent), [sync](#sync),
[check/reconcile](#reconcile-agent--database), the [monitoring](#monitoring-toggle)
toggle, and [profiling](#profiling). The sections below describe each one.

### Deploying the agent

From the agent dialog, **Deploy agent** installs and starts the agent over SSH in
one click — cross-compiled Linux binary, uploaded and launched for you. After a
deploy the admin app automatically re-pushes every interface for that server so
the box comes up in the desired state. A live status area reports progress and
any failure.

In the **desktop** app you can also pick a local agent binary from disk with a
native file picker, instead of relying on a downloaded one.

#### Server prerequisites

The agent ships in two builds, each needing different things on the server —
pick the matching [agent source](#agent-sources-deploy-presets):

- **`awg-agent` (systemd, kernel).** Drives the kernel module directly, so the
  server needs it installed:
  - the **AmneziaWG kernel module** for AmneziaWG interfaces — install
    `amneziawg-dkms` per the
    [amneziawg-linux-kernel-module README](https://github.com/amnezia-vpn/amneziawg-linux-kernel-module/blob/master/README.md)
    and load it (`modprobe amneziawg`). The deploy pre-checks this and fails
    fast with a clear message if it's missing;
  - **WireGuard** itself if you also create plain (non-Amnezia) interfaces —
    see [wireguard.com/install](https://www.wireguard.com/install/).
- **`awg-agent-userspace` (Docker).** Runs the agent as a container with an
  in-process userspace WireGuard (the amneziawg-go library is compiled in) — no
  kernel module required, for hosts where you can't install one. It only needs
  **Docker** — see [docs.docker.com/engine/install](https://docs.docker.com/engine/install/)
  (the deploy runs the container with `/dev/net/tun` + `NET_ADMIN` for you). The
  deploy pre-checks `docker info` and fails fast if Docker isn't available.

Both builds speak the identical API; only how the interface link is created
differs. Versions must match: an AmneziaWG 1.0 kernel module rejects 2.0-style
obfuscation params (`H1–H4` ranges, `I1–I5`) — upgrade the module to 2.0, or
deactivate the interface (see [Interfaces](#interfaces)) to keep its config
without applying it.

Once running, the agent detects what its host actually supports — Docker, the
AmneziaWG kernel module, which interface kinds it can create, and whether it's
running in a container — and reports it back; the
[Dashboard](#dashboard-and-metrics)'s **agent type** column surfaces it.

### Agent sources (deploy presets)

An **agent source** is a reusable, named preset describing where to get the
agent binary from (e.g. a release URL), optionally cached locally so repeated
deploys don't re-download. Manage them from the source dropdown in the agent
dialog's deploy section: **Add new…** opens a small form to create one, and each
saved entry has a refresh-cache and a remove button. Handy when you deploy to
many servers or pin a specific agent version.

### Sync

**Sync** re-pushes every interface Amnezia Admin has recorded for a server to
its agent, overwriting whatever the agent currently has with the database's
desired state. It runs automatically after a deploy and after a tunnel
reconnects; the manual button is there for when you want to force the agent back
into line — e.g. after the agent was reinstalled, or you suspect drift.

### Reconcile (agent ↔ database)

**Reconcile** compares what the agent *actually* has configured against what the
database says it should have, by interface name, and shows the mismatches so you
can resolve each one by hand. Two kinds of mismatch:

- **On the agent but not in the database** (e.g. the DB was restored from an
  older backup): **Import** it into the database, or **Delete from agent**.
- **In the database but not on the agent** (e.g. the agent lost its storage /
  was reinstalled): **Re-push** it to the agent, or **Delete from database**.

When both sides agree, reconcile simply reports there's nothing to do. Note that
importing recovers only the interface shell (address, keys, AmneziaWG params,
peers the agent reports) — the association between a peer and the *user* it
belongs to lives only in the admin database and can't be reconstructed from the
agent.

### Monitoring toggle

Each server has a monitoring switch that enables/disables the agent's metrics
collection at runtime. The desired state is stored and re-applied on redeploy,
so a server you've muted stays muted after the agent restarts.

### Profiling

For diagnosing a misbehaving agent, you can turn on **profiling** — the agent's
Go runtime profiling (`net/http/pprof`). It's **off by default** (its sampling
has overhead, and the endpoints expose internals), toggled per server from the
agent dialog, and like monitoring the desired state is re-applied on redeploy.
While it's on, a small **activity icon** appears next to the server's name (on
the dashboard and the server page) as a reminder it's enabled.

With profiling on, use **Download dump** to pull a profile off the agent for
inspection with `go tool pprof`/`go tool trace`: pick the kind — `goroutine`
(stuck/leaking goroutines), `heap` (memory), the CPU `profile` or an execution
`trace` (both sampled over a chosen number of seconds), and the usual
`allocs`/`block`/`mutex`/`threadcreate`. Desktop saves it via a native dialog;
the web app downloads it in the browser. Turn profiling back off when you're
done.

## Interfaces

On the server page you can create, edit and delete WireGuard/AmneziaWG
interfaces. The form has two tabs.

**General** tab:

- **Name** — the interface name (`wg0`, `awg0`, …). Required.
- **Address** — the interface's own address in CIDR form (e.g. `10.0.0.1/24`).
  **Optional**: leave it blank and Amnezia Admin auto-assigns the first host of
  the next free `/24` from the `172.23.0.0/16` pool (picked so it doesn't
  overlap any existing interface). On edit, leaving it blank keeps the current
  address (it won't move the interface to a new subnet and orphan its peers).
- **Listen port** — the UDP port the interface listens on (default `51820`).
- **Private key** — leave blank to have one generated automatically.
- **Activate interface** — a checkbox, **ticked by default**. While ticked, the
  agent brings the interface up (and raises it again on every agent restart).
  Untick it to keep the stored config but have the agent take the interface
  down; it then shows a **Disabled** badge in the list. Handy to park an
  interface without deleting it.

**Amnezia** tab — the `Jc/Jmin/Jmax`, `S1–S4`, `H1–H4` and `I1–I5` junk-packet
and header-obfuscation values that make AmneziaWG traffic harder to fingerprint:

- **Amnezia Interface** — a checkbox at the top, **ticked by default**. While
  ticked, all the obfuscation parameters below are pre-filled with freshly
  generated values (the same set the app would otherwise apply on its own), and
  every field stays editable if you want to tune them by hand.
- **Untick it** to create a plain **WireGuard** interface instead: the
  obfuscation parameters are then ignored and the interface behaves as vanilla
  WireGuard.

On edit the tab reflects what the interface already has — the box is ticked
(showing the stored values) for an AmneziaWG interface, unticked for a plain
WireGuard one.

A new or edited interface is validated against the server's existing ones: its
**name**, **listen port** and **subnet** (from Address) must each be free — a
duplicate name or port, or a subnet that overlaps another interface, is rejected
with the reason shown. (On edit, a field you didn't change isn't re-checked.)

Interfaces that are part of a [tunnel](#multi-hop-tunnels-exit-nodes) are locked
against editing/deletion in the UI; remove the tunnel first.

Deleting an interface also **deletes all of its peers** — a peer can't exist
without its interface. When the interface has any peers, the UI asks you to
confirm **twice** (the first dialog spells out how many peers will be removed).
The removed peers also stop appearing in the
[peer metrics](#dashboard-and-metrics).

Every interface tracks an **in-sync** status: whether the last push to the agent
succeeded, the error if it didn't, and when it last synced.

The agent raises only **active** interfaces on startup and takes every active
interface down when it stops, so a stopped agent leaves no tunnels running. A
deactivated interface is never brought up and its config is not applied — only
stored — which also makes deactivation a way to park an interface the agent
otherwise fails to configure.

Each interface — unless you address it with IPv6 — is brought up with **IPv6
disabled** (the tunnels here are IPv4-only), so it never gets an auto-assigned
link-local IPv6 address and can't carry or leak IPv6. An interface you
deliberately give an IPv6 address is left alone. This is best-effort: where the
agent can't set it — notably the userspace agent's Docker container, which runs
with a read-only `/proc/sys` — the interface still comes up (a warning is logged).

### Hook commands

Each interface supports **PreUp / PostUp / PreDown / PostDown** hook commands,
run on the agent when the interface comes up or goes down — the equivalent of
wg-quick's hooks (the agent doesn't run wg-quick, so anything like custom
routing or `iptables` rules is expressed as hooks). `%i` in a command expands to
the interface name. These are what the tunnel feature uses under the hood to set
up policy routing and NAT.

## Users and peers

**Users** (*Users* page) are the people your peers belong to. A user has a
**name**, an optional **description**, and can be **disabled**. Users are
server-independent — one user can have peers on several servers.

A **peer** is a single VPN client belonging to a user. *Add peer*:

- **Name** — a label for the peer (device name, etc.).
- **Server** and **interface** — where the peer attaches.
- **Allowed IPs** — the peer's addresses; leave blank to auto-assign a free
  host address on the interface's subnet. A supplied host address (a `/32`) must
  lie **inside** the interface's subnet and not already be taken by another peer
  or the interface itself — out-of-subnet and duplicate addresses are rejected.
  Broader routed CIDRs (a LAN behind a site-to-site peer, e.g. `192.168.1.0/24`)
  are allowed outside the subnet.
- **Endpoint** — optional, for peers that should be dialed at a fixed address.
- **DNS** — optional client-side DNS for this peer's generated config (one or
  more comma-separated servers, e.g. `1.1.1.1, 8.8.8.8`). It becomes the
  `DNS = …` line in the client's config; leave it blank to fall back to the
  interface's DNS. It's a client setting, never pushed to the agent.
- **Private key** — optional; leave it blank to have a fresh key generated for
  the peer. Fill it in only to import an existing key.
- **Pre-shared key** — optional extra symmetric key for post-quantum-ish
  hardening. The *Generate preshared key* box is ticked by default (a random key
  is created for you). Untick it to reveal a field where you can paste your own
  key — or leave that field empty to create the peer without a pre-shared key.

For each peer you get:

- **Config** — the ready-to-use `wg-quick`/AmneziaWG client configuration text,
  with the peer's keys, the server endpoint and the AmneziaWG obfuscation
  parameters filled in. Copy it into a client.
- **QR code** — the same config as a scannable QR for the mobile apps, with a
  button to **save it as a PNG** file (native save dialog in the desktop app).

You can **move a peer to another interface** (the ⇄ action on a peer, then pick a
target from any server): its keys, name, DNS and pre-shared key are preserved.
Its address is kept when it still fits the target interface's subnet and is free
there — e.g. moving between a tunnel's two members, which share a subnet —
otherwise a free address on the target is assigned; both interfaces are re-pushed.

You can **deactivate a peer** (the ⏻ toggle on a peer) without deleting it. A
deactivated peer keeps its stored config — keys, address, pre-shared key — but is
removed from the live interface on the agent, so that client can no longer
connect until you reactivate it. Mirrors the interface-level *Activate* switch
above, one client at a time: handy for temporarily revoking access without
throwing away the peer or its address. Its config/QR code stay available, but a
deactivated peer isn't on the device, so it reports no traffic in the metrics.

Deleting a peer revokes it on the agent (removes it from the interface) and drops
it from the peer metrics, so it no longer shows in the metrics history.

## Multi-hop tunnels (exit nodes)

A **tunnel** chains two of your servers so that clients connected to the first
egress the internet through the second. The classic use: server 1 is a **relay**
your clients reach, server 2 is the **exit node** whose public IP the internet
sees.

Build one from the **Tunnels** page with the wizard:

1. **Entry** — pick the server + interface your clients already connect to (the
   relay). It must have a listen port and a CIDR address.
2. **Exit** — pick a second server + interface to become the dedicated exit.
   Note its config is replaced to serve this role.
3. **Review** — the tunnel's shared subnet is shown for confirmation. It is
   **always the entry interface's own subnet** (the exit is placed on it), not a
   value you pick, then build.

Amnezia Admin reconfigures both interfaces and pushes them: the relay gets
policy-routing rules and a gateway peer pointing at the exit; the exit gets NAT
(masquerade) and dials back to the relay. Clients need no config change — their
existing tunnel to the relay now exits via the second server.

Because the exit sits on the entry's subnet, the two members share **one address
pool**: a peer you add to either end gets an address that's unique across both.
You can add client peers to the exit interface too, not just the relay.

**Removing** a tunnel reverts both interfaces to empty (clears the added peers,
hooks and routing) rather than deleting them, so the interfaces remain for
reuse. While a tunnel exists, its two interfaces can't be edited or deleted
individually, and neither can the servers that hold them — remove the tunnel
first.

## Dashboard and metrics

The **Dashboard** shows totals (servers, peers, users, tunnels) and a per-server
table with an **agent status** badge, an **agent type** column, a **peers** count,
**load average (1/5/15)** and **RAM %**. Clicking a server opens the **metrics
modal** with two tabs:

- **System** — host CPU/RAM/load/network over time.
- **Peers** — a per-peer activity table with a compact sparkline of each peer's
  traffic, labelled `<user>/<peer>`.

The **agent status** badge is a tri-state health check — it actually probes the
agent and combines that with the state of the connection to it:

- 🟢 **green** — the agent is reachable and answering (which, for an
  SSH-tunnelled server, also means its tunnel is up);
- 🔴 **red** — the connection is down: the SSH tunnel could not be brought up,
  or an mTLS agent (reached directly) is unreachable;
- 🟡 **amber** — in between, e.g. the SSH tunnel is up but the agent behind it
  isn't responding.

(A grey "unknown" shows until the first check completes.) Any non-green state is
logged at error level, so problems are visible in the agent/admin logs.

The **agent type** column reflects what the agent detected about its host when it
started: its backend build (`kernel` or `userspace`), a **docker** pill when the
agent itself runs in a container, and the interface kinds it can create there
(`awg` for AmneziaWG, `wg` for plain WireGuard). Hover it for the full detail —
whether Docker is available on the host, whether the AmneziaWG kernel module is
present, and the agent's version. It's a quick way to see at a glance whether a
server can obfuscate traffic and how its agent is running. (Like metrics it's
best-effort — it shows the last known values while the agent is briefly
unreachable, and `—` until the first successful read.)

Metrics come from the agent, which samples the host and every peer's
rx/tx/handshake on an interval (default 45s, `AWG_AGENT_METRICS_INTERVAL`) into
in-memory ring buffers, retaining up to 48h of history. History is checkpointed
to disk on the agent, so charts survive an agent restart. Metrics are
best-effort: a server whose agent is briefly unreachable shows the last known
values, and reachability is signalled separately by the agent-status badge.

The agent can also expose metrics in **Prometheus** text format (its `/metrics?fmt=prom`
endpoint) if you want to scrape it into your own monitoring.

## Settings

The **Settings** page adapts to how you're running:

- **Language** — English / Russian. Available in both modes.
- **Backup** — see below. Both modes.
- **Logs** and **Debug mode** — desktop only.
- **Basic Auth** and **Change credentials** — standalone only.

### Backup

**Save backup** downloads a full snapshot of the admin database — servers,
users, peers, interfaces and credentials — as a JSON file. In the desktop app it
opens a native save dialog; in the standalone server it downloads through the
browser. The file is the same portable format as the `awg-migrate` tool, so it
can be **restored with `awg-migrate import`** (there is no in-app restore, since
that would replace the database under the running process — see
[Backup, restore and migration](#backup-restore-and-migration)).

> The backup contains secrets — SSH private keys, agent mTLS keys, peer
> pre-shared keys and the admin password hash. Store it somewhere safe.

### Logs and debug mode (desktop)

The desktop app captures its own log output in memory. **View logs** shows the
recent log lines and lets you **save them to a JSON file** (useful for bug
reports). The **Debug mode** checkbox turns on debug-level logging at runtime —
off by default so the log isn't flooded; enable it, reproduce the issue, then
Refresh to see the detailed lines. (There's no log buffer in standalone mode, so
this section is desktop-only.)

### Login, credentials and Basic Auth (standalone)

The standalone web server is the only mode with authentication (the desktop app
runs locally with no network surface to protect).

- **Login** — a single admin account, seeded to **admin / admin** on first run.
  Change it immediately.
- **Change credentials** — set a new username/password.
- **HTTP Basic Auth** — an optional extra gate: when enabled, every request
  (including the login page itself) also requires an HTTP Basic auth prompt,
  using the same admin account. Useful when exposing the server directly without
  a reverse proxy in front. Off by default.

## Backup, restore and migration

Your entire admin state lives in one boltdb file (`~/.awg-admin`). Two tools
work with it, sharing the same portable JSON dump format:

- **In-app Backup** (Settings → Backup) — a one-click export of the current
  state to a JSON file.
- **`awg-migrate`** — the command-line export/import tool, for moving between
  machines or between the desktop and standalone modes (both read the same file
  format):

  ```sh
  awg-migrate export -db ~/.awg-admin -out dump.json   # from the old install
  awg-migrate import -db ~/.awg-admin -in dump.json    # into the new one
  ```

A file saved by the in-app **Backup** is a valid `awg-migrate` dump, so **to
restore a backup**, import it with `awg-migrate` into a fresh (or target)
database while the app is stopped, then start the app.

## Where your data and secrets live

- **Admin database** — `~/.awg-admin` (boltdb). Everything: servers, SSH
  credentials/keys, users, peers, interface configs, the standalone admin
  account. This is the one thing to back up.
- **Autocert cache** (standalone HTTPS) — `~/.awg-admin-autocert`.
- **Agent-binary cache** (deploy presets) — `~/.awg-admin-cache`.
- **Agent, per server** — stores only the interface configs it's been pushed
  (`AWG_AGENT_DB`, default `/var/lib/awg-agent`) plus its metrics-history
  checkpoint. It holds no admin-side secrets beyond the interface/peer keys it
  needs to run WireGuard.

In the standalone Docker image `$HOME` is `/data`, so mounting that volume
persists all of the above.

Interface hook commands run on the agent host with the agent's privileges
(typically root, since it manages network interfaces) — by design, exactly like
wg-quick hooks. They come from you, the admin, over the authenticated channel.
