#!/bin/bash
# Удаление cvpn. Пакеты (openconnect, sing-box, wireguard-tools, AmneziaWG)
# не трогает.
set -uo pipefail

LIB=/usr/local/lib/cvpn
ETC=/usr/local/etc/cvpn
SUDO=sudo
[ "$(id -u)" -eq 0 ] && SUDO=

user=$($SUDO sed -n 's/.*"cisco_user": *"\([^"]*\)".*/\1/p' "$ETC/config.json" 2>/dev/null)

if [ -x "$LIB/cvpn" ]; then
	"$LIB/cvpn" off 2>/dev/null || $SUDO "$LIB/cvpn" off
	$SUDO "$LIB/cvpn" service uninstall
fi

if [ "$(uname -s)" = Linux ]; then
	# Хвосты bash-версии.
	$SUDO systemctl disable --now cvpn-vless >/dev/null 2>&1
	$SUDO rm -f /etc/systemd/system/cvpn-vless.service
	$SUDO systemctl daemon-reload
	$SUDO rm -f /usr/share/applications/cvpn.desktop "$HOME/.config/autostart/cvpn.desktop" /usr/local/bin/cvpn-gui
else
	pkill -x cvpn-gui 2>/dev/null
	rm -f "$HOME/Library/LaunchAgents/com.github.inoisy.cvpn-gui.plist"
	$SUDO rm -rf /Applications/cvpn.app
	[ -n "$user" ] && security delete-generic-password -s cvpn -a "$user" >/dev/null 2>&1
fi

$SUDO rm -rf "$LIB" "$ETC"
$SUDO rm -f /usr/local/bin/cvpn /etc/sudoers.d/cvpn /var/log/cvpn-*.log /var/run/cvpn.sock
echo "cvpn удалён"
