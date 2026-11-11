#!/bin/sh
set -eu

case "${AWG_E2E_FIREWALL:-iptables}" in
	firewalld)
		mkdir -p /run/dbus
		dbus-daemon --system --fork
		firewalld --nofork >/tmp/firewalld.log 2>&1 &
		attempt=0
		until firewall-cmd --state >/dev/null 2>&1; do
			attempt=$((attempt + 1))
			if [ "$attempt" -ge 30 ]; then
				cat /tmp/firewalld.log
				exit 1
			fi
			sleep 1
		done
		firewall-cmd --add-port=8080/tcp
		firewall-cmd --permanent --add-port=8080/tcp
		;;
	ufw)
		ufw --force enable
		ufw allow 8080/tcp
		;;
esac

mkdir -p /data/providers.d /data/awg /data/xray /data/hysteria
cat > /data/providers.d/awg.conf <<'EOF'
{"protocolVersion":"v1","providers":[{"address":"http://127.0.0.1:9091","discover":true,"command":"/usr/local/bin/vpn-provider-awg","args":["--userspace"],"env":{"VPN_PROVIDER_AWG_DB":"/data/awg","VPN_PROVIDER_AWG_ADDR":"127.0.0.1:9091"},"provider":{"name":"awg"}}]}
EOF
cat > /data/providers.d/xray.conf <<'EOF'
{"protocolVersion":"v1","providers":[{"address":"http://127.0.0.1:9092","discover":true,"command":"/usr/local/bin/vpn-provider-xray","env":{"VPN_PROVIDER_XRAY_DB":"/data/xray","VPN_PROVIDER_XRAY_ADDR":"127.0.0.1:9092","VPN_PROVIDER_XRAY_CORE":"/usr/local/bin/xray"},"provider":{"name":"xray"}}]}
EOF
cat > /data/providers.d/hysteria2.conf <<'EOF'
{"protocolVersion":"v1","providers":[{"address":"http://127.0.0.1:9093","discover":true,"command":"/usr/local/bin/vpn-provider-hysteria2","env":{"VPN_PROVIDER_HYSTERIA2_DB":"/data/hysteria","VPN_PROVIDER_HYSTERIA2_ADDR":"127.0.0.1:9093","VPN_PROVIDER_HYSTERIA2_CORE":"/usr/local/bin/hysteria"},"provider":{"name":"hysteria2"}}]}
EOF

exec /usr/local/bin/vpn-agent
