<p align="center">
  <img src="docs/logo.png" alt="Amnezia-WG Admin" width="128" height="128">
</p>

# Amnezia-WG Admin (awg-admin)

[Русская версия](README_ru.md)

**Amnezia-WG Admin** is a centralized management application for AmneziaWG and WireGuard infrastructures. It stores the
configuration of managed servers, users, and VPN peers, and applies that configuration through an agent running on each
server.

A single instance can manage multiple servers simultaneously. The local database serves as the source of truth, while
agents are responsible for reconciling the actual server state with the stored configuration.

> 📖 **Installation, configuration, and full documentation:** [docs/GUIDE.md](docs/GUIDE.md)

## Overview

Amnezia-WG Admin provides centralized administration of AmneziaWG and WireGuard deployments without manually editing
configuration files or connecting to every server over SSH.

After a server has been added, the application can automatically deploy an agent over SSH. All subsequent management
operations are performed through the agent API, either over an SSH tunnel or directly via mTLS. If the actual server
state diverges from the stored configuration, the application reconciles the differences by reapplying the required
changes.

Users and VPN peers are managed independently. Each user may own one or more peers. When a peer is created, the
application automatically generates cryptographic keys, assigns addresses, applies AmneziaWG parameters, and produces
client configuration files together with QR codes.

Multiple servers can be organized into multi-hop VPN topologies, allowing selected nodes to operate as intermediate
routers or Internet exit nodes.

The application collects runtime information from managed servers, including CPU load, memory usage, and WireGuard peer
statistics. The entire infrastructure configuration can be exported to a portable backup and restored on another
installation.

Amnezia-WG Admin is available as both a desktop application and a standalone web server. Both deployment modes share the
same data model, business logic, and user interface.

## Architecture

Amnezia-WG Admin consists of three components: the administrative application, the agent, and the frontend.

### Admin app

The administrative application stores the infrastructure configuration, manages servers, users, and VPN peers, and
coordinates communication with remote agents.

It is available in two deployment modes: a Wails-based desktop application and a standalone web server. Both variants
share the same business logic and user interface, differing only in their runtime environment and frontend transport.

The source code is located in the root Go module and the `frontend/` directory.

### Agent

The agent is a lightweight HTTP service running on every managed server.

It manages AmneziaWG and WireGuard interfaces, stores their local configuration, and exposes the API used by the
administrative application. WireGuard operations are performed through the `wgctrl-go` library.

By default, the agent listens only on the loopback interface and is intended to be accessed through an SSH tunnel.
Direct mTLS connections are also supported when SSH tunneling is not desirable.

The source code is located in the `agent/` directory.

### Frontend

The user interface is implemented as a React 19 and TypeScript single-page application.

At startup, the frontend detects its runtime environment and automatically selects the appropriate transport: the Wails
API for the desktop application or the standalone server's HTTP API. The user experience remains identical in both
deployment modes.

The source code is located in the `frontend/` directory.

### Data storage

All infrastructure metadata is stored locally in a BoltDB database (`~/.awg-admin`).

The database contains information about managed servers, SSH credentials, users, VPN peers, and their relationships. It
serves as the single source of truth for the administrative application.

The agent stores only its local interface configuration and has no knowledge of the infrastructure as a whole.

## Building from source

Building the project requires Go **1.26.2** or later and Node.js **24** or later.

The following `Makefile` targets are available:

```sh
make server      # standalone web server
make run-server  # local development server (go run)
make desktop     # Wails desktop application
make agent       # Linux agent
make migrate     # awg-migrate utility
```

Build artifacts are written to the `build/bin/` directory. The `make run-server` target is intended for local
development and starts the application without producing a standalone executable.

Building the desktop application requires the Wails CLI:

```sh
go tool wails
```

Targets that include the user interface automatically build the frontend, so a separate Node.js build step is usually
unnecessary.

## License

This project is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for the full license text.
