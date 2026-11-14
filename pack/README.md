# Pack deployment

Agent and provider installation is packaged with `pack/sdk`. Binary releases live in GHCR as multi-architecture OCI artifacts:

- `oci://ghcr.io/ks-tool/vpn-agent:v2.0.0`, tagged `v2.0.0`;
- `oci://ghcr.io/ks-tool/vpn-providers/awg:v1.2.3`, tagged `providers/awg/v1.2.3`.

The pack's `artifact` value points to that OCI reference. The Manage panel's **Agent** tab deploys the agent through `pack apply`. For an optional provider, the admin's bootstrap pack only delivers its binary and deployment values, then invokes `vpn-provider-<name> deploy --config …`. The provider's own deploy command applies its pack locally: dependencies, values and registration live beside its implementation (`agent/providers/awg/deploy` for AWG), and its persistent pack state lets unchanged packages and configuration skip subsequent deploys. The agent itself does not install packages or run pack. The admin's external `host` target is refreshed automatically; no external `pack` command is used. The target supports Debian and Fedora on `amd64` and `arm64`. It opens its own SSH connection for the duration of the deployment and starts a temporary host agent and OCI proxy. The managed server downloads the binary for its platform from GHCR through that loopback proxy. The AWG pack persists `net.ipv4.ip_forward=1`; during provider deployment, the host target accepts input on every stored VPN interface through firewalld, ufw or iptables (for example, `iptables -A INPUT -i awg0 -j ACCEPT`). Successful AWG interface changes also trigger a lightweight firewall-only pack, without downloading the provider again or restarting the agent. SSH key authentication is required, and a non-root SSH user needs passwordless sudo. Pack state, the downloaded target and cached bundles are stored under `~/.vpn-admin/cache`. Generate the standalone bundle sources with:

```sh
go run ./pack/vpn-agent/cmd generate -o dist/vpn-agent
go run ./pack/example-provider/cmd generate -o dist/example-provider
```

Publish the generated bundles with `oci-packer`, then use the normal `pack init` / `pack plan` / `pack apply` workflow. Install `vpn-agent` before its providers. Do not copy provider binaries or systemd units through vpn-admin's legacy deployment UI.

Systemd runs only `vpn-agent`. Each provider writes a registry file under `/var/lib/vpn-agent/providers.d/` containing its command, arguments, environment and loopback address. The agent watches this directory, starts or reloads changed providers without an agent restart, restarts a provider after an unexpected exit and terminates it during agent shutdown. There is no separate provider systemd unit.

With `discover: true`, the agent reads the provider definition and RJSF schemas from `GET /v1/manifest` after starting the process. Static registrations may embed `provider` definitions instead. The agent also reads the legacy `providers.conf`.

AWG automatically selects the kernel backend when `modinfo amneziawg` or `modinfo wireguard` succeeds, and otherwise uses `amneziawg-go`. Add `--userspace` to the registration's `args` (or set `VPN_PROVIDER_AWG_USERSPACE=true`) to force userspace mode.

Provider crashes produce `Degraded` resource conditions and retries; they do not remove desired state. External providers listen on loopback by default; direct remote registrations must use mTLS. Go `.so` plugins are not supported.
