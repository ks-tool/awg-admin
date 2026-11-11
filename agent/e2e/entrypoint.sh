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

exec /usr/local/bin/vpn-provider-awg --userspace
