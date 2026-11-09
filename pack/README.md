# Pack deployment

Agent and provider installation is packaged with `pack/sdk`. Binary releases live in GHCR as multi-architecture OCI artifacts:

- `oci://ghcr.io/ks-tool/vpn-agent:v1.2.3`, tagged `agent/v1.2.3`;
- `oci://ghcr.io/ks-tool/vpn-providers/awg:v1.2.3`, tagged `providers/awg/v1.2.3`.

The pack's `artifact` value points to that OCI reference. Use an OCI-capable pack target; ansible-target is not used. Generate the pack bundle sources with:

```sh
go run ./pack/vpn-agent/cmd generate -o dist/vpn-agent
go run ./pack/providers/awg/cmd generate -o dist/vpn-provider-awg
go run ./pack/example-provider/cmd generate -o dist/example-provider
```

Publish the generated bundles with `oci-packer`, then use the normal `pack init` / `pack plan` / `pack apply` workflow. Install `vpn-agent` before its providers. Do not copy provider binaries or systemd units through vpn-admin's legacy deployment UI.

Systemd runs only `vpn-agent`. Each provider writes a registry file under `/var/lib/vpn-agent/providers.d/` containing its command, arguments, environment and loopback address. The agent starts the provider, restarts it after an unexpected exit and terminates it during agent shutdown. The provider pack restarts `vpn-agent` after installing or changing that registration. There is no separate provider systemd unit.

With `discover: true`, the agent reads the provider definition and RJSF schemas from `GET /v1/manifest` after starting the process. Static registrations may embed `provider` definitions instead. The agent also reads the legacy `providers.conf`.

AWG automatically selects the kernel backend when `modinfo amneziawg` or `modinfo wireguard` succeeds, and otherwise uses `amneziawg-go`. Add `--userspace` to the registration's `args` (or set `VPN_PROVIDER_AWG_USERSPACE=true`) to force userspace mode.

Provider crashes produce `Degraded` resource conditions and retries; they do not remove desired state. External providers listen on loopback by default; direct remote registrations must use mTLS. Go `.so` plugins are not supported.
