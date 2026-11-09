# Pack deployment

Agent and provider installation is packaged with `pack/sdk`: the bundle downloads a
release binary, writes its environment and systemd unit, mounts secrets through
`SecretRef`, creates the state directory and starts the service. Generate the OCI
bundle sources with:

```sh
go run ./pack/vpn-agent/cmd generate -o dist/vpn-agent
go run ./pack/example-provider/cmd generate -o dist/example-provider
```

Publish the generated bundles with the pack OCI tooling, then use the normal
`pack init` / `pack plan` / `pack apply` workflow with the `ansible` target. Do not
copy provider binaries or systemd units through vpn-admin's legacy deployment UI.

`example-provider` also writes `/var/lib/vpn-agent/providers.conf`. The registry
contains the protocol version, the provider address and the definitions consumed by
RJSF. On a real provider, generate `definition` from its `GET /v1/manifest` response
instead of editing schemas in vpn-admin. Restart the agent service after changing
the registry. External providers listen on loopback by default; direct remote
registries must use mTLS.

The agent loads external providers without rebuilding agent-core. Provider crashes
produce `Degraded` resource conditions and retries; they do not terminate the agent
or remove desired state. Go `.so` plugins are not supported.
