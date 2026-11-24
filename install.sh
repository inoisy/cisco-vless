#!/bin/bash
# Установка cvpn: Cisco AnyConnect (openconnect) + VLESS / WireGuard / AmneziaWG.
# macOS (Homebrew) и Linux (apt, systemd). Windows — install.ps1.
#
# Ставит зависимости, собирает cvpn (есть Go) или качает релиз, ставит
# службу и спрашивает настройки (cvpn setup). Повторный запуск — обновление;
# старая bash-версия выключается, её настройки подхватываются.
#
# Переменные окружения:
#   CVPN_GUI=0|1       ставить значок в трее (по умолчанию 1 на macOS, спросить на Linux)
#   CVPN_WG=1          поставить wireguard-tools
#   CVPN_AWG=1         поставить AmneziaWG (awg-quick)
#   CVPN_NO_SETUP=1    не запускать cvpn setup
#   CVPN_VERSION=vX.Y  версия релиза, если собирать нечем (по умолчанию latest)

set -euo pipefail
cd "$(dirname "$0")"

REPO=inoisy/cisco-vless
SINGBOX_VERSION=1.14.1
LIB=/usr/local/lib/cvpn
OS=$(uname -s)

SUDO=sudo
[ "$(id -u)" -eq 0 ] && SUDO=
if [ "$OS" = Darwin ] && [ "$(id -u)" -eq 0 ]; then
	echo "на macOS запускай без sudo: пароль Cisco кладётся в твой Keychain"
	exit 1
fi
OWNER=${SUDO_USER:-$USER}

# as_owner — команда от того, кто ставит (под sudo — от SUDO_USER).
as_owner() {
	if [ "$(id -u)" -eq 0 ] && [ "$OWNER" != root ]; then sudo -u "$OWNER" "$@"; else "$@"; fi
}

say() { printf '\n== %s\n' "$*"; }
die() { echo "Ошибка: $*" >&2; exit 1; }
yes_no() {
	local ans
	read -rp "$1 [y/N]: " ans
	[[ "$ans" =~ ^[YyДд] ]]
}

case "$OS" in Darwin|Linux) ;; *) die "ОС $OS не поддержана (Windows — install.ps1)" ;; esac
case "$(uname -m)" in x86_64|amd64) ARCH=amd64 ;; arm64|aarch64) ARCH=arm64 ;; *) die "архитектура $(uname -m)" ;; esac
GOOS=$(echo "$OS" | tr '[:upper:]' '[:lower:]')

# Сервер с bash-версией (VPS агентов): sing-box там — отдельный юнит
# cvpn-vless, на нём держатся сторож и подписка, singbox.json правлен руками.
# Go-версия этот юнит удаляет и пересобирает конфиг — не ставим.
# Обновлять такой сервер — server/install.sh. Обход — CVPN_FORCE_GO=1.
if [ "$OS" = Linux ] && [ "${CVPN_FORCE_GO:-}" != 1 ]; then
	for u in cvpn-guard.timer cvpn-sub.timer; do
		if systemctl cat "$u" >/dev/null 2>&1; then
			die "здесь $u — сервер на bash-версии cvpn (юнит cvpn-vless, ручной singbox.json).
Go-версия снесёт cvpn-vless и перепишет конфиг. Обновление — server/install.sh, см. README «Сервер».
Ставить Go всё равно — CVPN_FORCE_GO=1 (сначала бэкап, см. README)."
		fi
	done
fi

GUI=${CVPN_GUI:-}
if [ -z "$GUI" ]; then
	if [ "$OS" = Darwin ]; then GUI=1
	elif [ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ] && yes_no "Поставить значок в трее (GUI)?"; then GUI=1
	else GUI=0
	fi
fi

# --- зависимости ---------------------------------------------------------

deps_mac() {
	command -v brew >/dev/null || die "нужен Homebrew: https://brew.sh"
	local pkgs=(openconnect sing-box)
	[ "${CVPN_WG:-}" = 1 ] && pkgs+=(wireguard-tools)
	say "brew install ${pkgs[*]}"
	brew install "${pkgs[@]}"
	if [ "${CVPN_AWG:-}" = 1 ]; then awg_mac; fi
}

# AmneziaWG на macOS: формулы в Homebrew нет — собираем awg/awg-quick и
# userspace-реализацию amneziawg-go из исходников.
awg_mac() {
	command -v awg-quick >/dev/null && return
	command -v go >/dev/null || brew install go
	brew install bash # awg-quick требует bash 4+
	local tmp
	tmp=$(mktemp -d)
	say "AmneziaWG: сборка amneziawg-tools и amneziawg-go"
	git clone -q --depth 1 https://github.com/amnezia-vpn/amneziawg-tools "$tmp/tools"
	make -C "$tmp/tools/src" -s
	$SUDO make -C "$tmp/tools/src" -s install WITH_WGQUICK=yes PREFIX=/usr/local
	git clone -q --depth 1 https://github.com/amnezia-vpn/amneziawg-go "$tmp/go"
	(cd "$tmp/go" && go build -o amneziawg-go .)
	$SUDO install -m 755 "$tmp/go/amneziawg-go" /usr/local/bin/amneziawg-go
	rm -rf "$tmp"
}

deps_linux() {
	command -v apt-get >/dev/null || die "поддержан только Linux с apt (Debian/Ubuntu)"
	command -v systemctl >/dev/null || die "нужен systemd"
	local pkgs=(openconnect vpnc-scripts curl iproute2 ca-certificates)
	[ "${CVPN_WG:-}" = 1 ] && pkgs+=(wireguard-tools)
	[ "$GUI" = 1 ] && pkgs+=(libgtk-4-1 libwebkitgtk-6.0-4)
	say "apt-get install ${pkgs[*]}"
	$SUDO apt-get update -qq
	$SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "${pkgs[@]}" >/dev/null

	if ! command -v sing-box >/dev/null; then
		local deb
		deb=$(mktemp --suffix=.deb)
		say "sing-box $SINGBOX_VERSION ($ARCH)"
		curl -fsSL -o "$deb" \
			"https://github.com/SagerNet/sing-box/releases/download/v$SINGBOX_VERSION/sing-box_${SINGBOX_VERSION}_linux_${ARCH}.deb"
		$SUDO dpkg -i "$deb" >/dev/null
		rm -f "$deb"
	fi

	if [ "${CVPN_AWG:-}" = 1 ] && ! command -v awg-quick >/dev/null; then
		say "AmneziaWG (ppa:amnezia/ppa)"
		$SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq software-properties-common >/dev/null
		$SUDO add-apt-repository -y ppa:amnezia/ppa >/dev/null
		$SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq amneziawg amneziawg-tools >/dev/null ||
			echo "ВНИМАНИЕ: AmneziaWG не встал — поставь amneziawg-tools вручную"
	fi
}

if [ "$OS" = Darwin ]; then deps_mac; else deps_linux; fi

# --- cvpn: собрать или скачать --------------------------------------------

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if command -v go >/dev/null && [ -f go.mod ]; then
	say "Сборка cvpn ($(go version | awk '{print $3}'))"
	ver=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$ver" -o "$tmp/cvpn" ./cmd/cvpn
	if [ "$GUI" = 1 ]; then
		if [ "$OS" = Linux ]; then
			$SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev >/dev/null
		fi
		go build -trimpath -ldflags "-s -w" -o "$tmp/cvpn-gui" ./cmd/cvpn-gui 2>&1 | grep -v 'warning' || true
		[ -x "$tmp/cvpn-gui" ] || die "GUI не собрался (CVPN_GUI=0 — без него)"
	fi
else
	rel="latest/download"
	[ -n "${CVPN_VERSION:-}" ] && rel="download/$CVPN_VERSION"
	say "Загрузка cvpn ($rel, ${GOOS}_$ARCH)"
	curl -fsSL -o "$tmp/cvpn" "https://github.com/$REPO/releases/$rel/cvpn_${GOOS}_$ARCH"
	if [ "$GUI" = 1 ]; then
		curl -fsSL -o "$tmp/cvpn-gui" "https://github.com/$REPO/releases/$rel/cvpn-gui_${GOOS}_$ARCH"
	fi
	chmod +x "$tmp"/cvpn*
fi

# --- старая bash-версия ----------------------------------------------------

if [ -f "$LIB/cvpn" ] && head -1 "$LIB/cvpn" | grep -q bash; then
	say "Выключаю старую bash-версию cvpn (настройки подхватятся)"
	$SUDO "$LIB/cvpn" off || true
	if [ "$OS" = Linux ]; then
		$SUDO systemctl disable --now cvpn-vless >/dev/null 2>&1 || true
		$SUDO rm -f /etc/systemd/system/cvpn-vless.service
		$SUDO systemctl daemon-reload
	fi
	# Служба отвечает владельцу по сокету — sudo для cvpn больше не нужен.
	$SUDO rm -f /etc/sudoers.d/cvpn "$LIB/vpnc-wrapper.sh"
fi

# --- установка -------------------------------------------------------------

say "Файлы: $LIB, /usr/local/bin/cvpn"
$SUDO mkdir -p "$LIB" /usr/local/bin
$SUDO install -m 755 "$tmp/cvpn" "$LIB/cvpn"
$SUDO ln -sf "$LIB/cvpn" /usr/local/bin/cvpn

say "Служба cvpn"
$SUDO env CVPN_OWNER="$OWNER" "$LIB/cvpn" service install

for _ in 1 2 3 4 5 6 7 8 9 10; do
	[ -S /var/run/cvpn.sock ] && break
	sleep 0.5
done

if [ "$GUI" = 1 ]; then
	say "Значок в трее"
	if [ "$OS" = Darwin ]; then
		app=/Applications/cvpn.app
		$SUDO mkdir -p "$app/Contents/MacOS"
		$SUDO install -m 755 "$tmp/cvpn-gui" "$app/Contents/MacOS/cvpn-gui"
		$SUDO tee "$app/Contents/Info.plist" >/dev/null <<-EOF
			<?xml version="1.0" encoding="UTF-8"?>
			<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
			<plist version="1.0"><dict>
			  <key>CFBundleName</key><string>cvpn</string>
			  <key>CFBundleIdentifier</key><string>com.github.inoisy.cvpn</string>
			  <key>CFBundleExecutable</key><string>cvpn-gui</string>
			  <key>CFBundlePackageType</key><string>APPL</string>
			  <key>LSUIElement</key><true/>
			  <key>LSMinimumSystemVersion</key><string>11.0</string>
			</dict></plist>
		EOF
		# Автозапуск при входе.
		agent="$HOME/Library/LaunchAgents/com.github.inoisy.cvpn-gui.plist"
		mkdir -p "$(dirname "$agent")"
		cat > "$agent" <<-EOF
			<?xml version="1.0" encoding="UTF-8"?>
			<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
			<plist version="1.0"><dict>
			  <key>Label</key><string>com.github.inoisy.cvpn-gui</string>
			  <key>ProgramArguments</key><array><string>/usr/bin/open</string><string>-a</string><string>$app</string></array>
			  <key>RunAtLoad</key><true/>
			</dict></plist>
		EOF
		open -a "$app" || true
	else
		$SUDO install -m 755 "$tmp/cvpn-gui" "$LIB/cvpn-gui"
		$SUDO ln -sf "$LIB/cvpn-gui" /usr/local/bin/cvpn-gui
		desktop="[Desktop Entry]
Type=Application
Name=cvpn
Comment=Cisco + VLESS / WireGuard / AmneziaWG
Exec=/usr/local/bin/cvpn-gui
Icon=network-vpn
Categories=Network;
X-GNOME-Autostart-enabled=true"
		echo "$desktop" | $SUDO tee /usr/share/applications/cvpn.desktop >/dev/null
		home=$(getent passwd "$OWNER" | cut -d: -f6)
		as_owner mkdir -p "$home/.config/autostart"
		echo "$desktop" | as_owner tee "$home/.config/autostart/cvpn.desktop" >/dev/null
	fi
fi

# --- настройки -------------------------------------------------------------

if [ "${CVPN_NO_SETUP:-}" != 1 ]; then
	say "Настройки"
	as_owner /usr/local/bin/cvpn setup
fi

if [ "$OS" = Linux ] && ! readlink /etc/resolv.conf | grep -q 'systemd/resolve/stub-resolv.conf'; then
	say "Внимание: /etc/resolv.conf не смотрит в systemd-resolved"
	echo "Split-DNS для корп-доменов работает только через resolved. Включить:"
	echo "  sudo ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf"
fi

say "Готово"
cat <<-EOF
	  cvpn on [otp]          vless + cisco
	  cvpn off               выключить всё
	  cvpn                   статус
	  cvpn cisco|vless|wg|awg on|off
EOF
