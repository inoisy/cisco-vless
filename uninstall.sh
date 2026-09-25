#!/bin/bash
# Удаление cvpn. Пакеты (openconnect, sing-box, wireguard-tools) не трогает.
set -uo pipefail

LIB=/usr/local/lib/cvpn
ETC=/usr/local/etc/cvpn
SUDO=sudo
[ "$(id -u)" -eq 0 ] && SUDO=

[ -x "$LIB/cvpn" ] && $SUDO "$LIB/cvpn" off

if [ "$(uname -s)" = Linux ]; then
	$SUDO systemctl disable --now cvpn-vless >/dev/null 2>&1
	$SUDO rm -f /etc/systemd/system/cvpn-vless.service
	$SUDO systemctl daemon-reload
else
	# shellcheck source=/dev/null
	user=$(. "$ETC/config" 2>/dev/null && echo "${CISCO_USER:-}")
	[ -n "$user" ] && security delete-generic-password -s cvpn -a "$user" >/dev/null 2>&1
fi

$SUDO rm -rf "$LIB" "$ETC"
$SUDO rm -f /usr/local/bin/cvpn /etc/sudoers.d/cvpn /var/log/cvpn-*.log
echo "cvpn удалён"
