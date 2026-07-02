module github.com/ks-tool/awg-admin/agent

go 1.26.2

require (
	github.com/Jipok/wgctrl-go v1.2.0
	// AmneziaWG 2.0 line (h1–h4 ranges, s1–s4, i1–i5) — matches what
	// GenerateAmneziaParams emits. Do NOT "upgrade" to v1.0.x: despite the
	// higher number it's the older 1.x semantics (single-uint32 h, no s3/s4)
	// and rejects the app's params at the UAPI. Verified by agent/e2e.
	github.com/amnezia-vpn/amneziawg-go v0.2.19
	github.com/caarlos0/env/v11 v11.4.1
	github.com/fsnotify/fsnotify v1.10.1
	github.com/natefinch/atomic v1.0.1
	github.com/rs/zerolog v1.35.1
	github.com/vishvananda/netlink v1.3.1
)

require (
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/mdlayher/genetlink v1.4.0 // indirect
	github.com/mdlayher/netlink v1.11.2 // indirect
	github.com/mdlayher/socket v0.6.1 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
	golang.zx2c4.com/wireguard v0.0.0-20260522210424-ecfc5a8d5446 // indirect
)
