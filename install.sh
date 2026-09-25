#!/bin/bash
# Установка cvpn: Cisco AnyConnect (openconnect) + VLESS/WireGuard.
# macOS (Homebrew) и Linux (apt, systemd).
#
# Интерактивно:  ./install.sh
# Без вопросов — те же ответы через переменные окружения:
#   CVPN_CISCO_HOST       адрес сервера Cisco (vpn.example.com)
#   CVPN_CISCO_USER       логин
#   CVPN_CISCO_GROUP      группа (authgroup), можно пусто
#   CVPN_CISCO_PIN        pin-sha256:... сертификата; пусто — снять с сервера
#   CVPN_CORP_DOMAINS     корп-домены для split-DNS через пробел, можно пусто
#   CVPN_CORP_CHECK_HOST  хост для проверки в cvpn status, можно пусто
#   CVPN_VLESS_LINK       ссылка vless://...
#   CVPN_WG_CONF          путь к конфигу WireGuard, можно пусто
#   CVPN_CISCO_PASSWORD   пароль Cisco (только Linux; на macOS — Keychain)
#   CVPN_VLESS_BOOT=1     Linux: поднимать vless при загрузке
#   CVPN_SUDOERS=1        разрешить cvpn через sudo без пароля
#   CVPN_YES=1            не спрашивать подтверждений

set -euo pipefail
cd "$(dirname "$0")"

SINGBOX_VERSION=1.14.1
LIB=/usr/local/lib/cvpn
ETC=/usr/local/etc/cvpn
OS=$(uname -s)

SUDO=sudo
[ "$(id -u)" -eq 0 ] && SUDO=

say()  { printf '\n== %s\n' "$*"; }
die()  { echo "Ошибка: $*" >&2; exit 1; }

# ask VAR "вопрос" [по-умолчанию] [secret]: берёт CVPN_VAR, иначе спрашивает.
ask() {
	local var=$1 q=$2 def=${3:-} secret=${4:-} env="CVPN_$1" ans
	if [ -n "${!env+x}" ]; then
		printf -v "$var" '%s' "${!env}"
		return
	fi
	if [ -n "$secret" ]; then
		read -rsp "$q: " ans
		echo
	else
		read -rp "$q${def:+ [$def]}: " ans
	fi
	printf -v "$var" '%s' "${ans:-$def}"
}

yes_no() {
	[ "${CVPN_YES:-}" = 1 ] && return 0
	local ans
	read -rp "$1 [y/N]: " ans
	[[ "$ans" =~ ^[YyДд] ]]
}

# --- зависимости ---------------------------------------------------------

install_deps_mac() {
	command -v brew >/dev/null || die "нужен Homebrew: https://brew.sh"
	[ "$(id -u)" -ne 0 ] || die "на macOS запускай без sudo: пароль Cisco кладётся в твой Keychain"
	local pkgs=(openconnect sing-box)
	if [ -n "$WG_CONF" ]; then pkgs+=(wireguard-tools); fi
	say "brew install ${pkgs[*]}"
	brew install "${pkgs[@]}"
	local prefix
	prefix=$(brew --prefix)
	EXTRA_PATH="$prefix/bin:$prefix/sbin"
	VPNC_SCRIPT="$prefix/etc/vpnc/vpnc-script"
}

install_deps_linux() {
	command -v apt-get >/dev/null || die "поддержан только Linux с apt (Debian/Ubuntu)"
	command -v systemctl >/dev/null || die "нужен systemd"
	local pkgs=(openconnect vpnc-scripts curl perl bind9-dnsutils iproute2)
	if [ -n "$WG_CONF" ]; then pkgs+=(wireguard-tools); fi
	say "apt-get install ${pkgs[*]}"
	$SUDO apt-get update -qq
	$SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "${pkgs[@]}" >/dev/null

	if ! command -v sing-box >/dev/null; then
		local arch deb
		arch=$(dpkg --print-architecture)
		deb=$(mktemp --suffix=.deb)
		say "sing-box $SINGBOX_VERSION ($arch)"
		curl -fsSL -o "$deb" \
			"https://github.com/SagerNet/sing-box/releases/download/v$SINGBOX_VERSION/sing-box_${SINGBOX_VERSION}_linux_${arch}.deb"
		$SUDO dpkg -i "$deb" >/dev/null
		rm -f "$deb"
	fi
	EXTRA_PATH=
	VPNC_SCRIPT=/usr/share/vpnc-scripts/vpnc-script
	[ -x "$VPNC_SCRIPT" ] || VPNC_SCRIPT=/etc/vpnc/vpnc-script
	[ -x "$VPNC_SCRIPT" ] || die "не найден vpnc-script"
}

# Публичные DNS-серверы системы: их пускаем мимо tun — многие провайдерские
# резолверы не отвечают запросам с чужого IP (выхода VLESS).
linux_public_dns() {
	{
		resolvectl dns 2>/dev/null | tr ' ' '\n'
		awk '/^nameserver/ { print $2 }' /etc/resolv.conf
	} | grep -E '^[0-9]+(\.[0-9]+){3}$' |
		grep -vE '^(127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|198\.1[89]\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.)' | sort -u
}

cert_pin() {
	openssl s_client -connect "$1:443" -servername "$1" </dev/null 2>/dev/null |
		openssl x509 -pubkey -noout |
		openssl pkey -pubin -outform der 2>/dev/null |
		openssl dgst -sha256 -binary | base64
}

# --- вопросы -------------------------------------------------------------

say "cvpn: Cisco AnyConnect + VLESS/WireGuard ($OS)"
case "$OS" in Darwin|Linux) ;; *) die "ОС $OS не поддержана" ;; esac

ask CISCO_HOST "Сервер Cisco (например vpn.example.com)"
[ -n "$CISCO_HOST" ] || die "сервер Cisco обязателен"
ask CISCO_USER "Логин Cisco"
[ -n "$CISCO_USER" ] || die "логин обязателен"
ask CISCO_GROUP "Группа Cisco (authgroup), Enter — без группы"
ask CORP_DOMAINS "Корп-домены для DNS через пробел (example.ru corp.local), Enter — пропустить"
ask CORP_CHECK_HOST "Корп-хост для проверки в cvpn status, Enter — пропустить"
ask VLESS_LINK "Ссылка vless://... (не отображается)" "" secret
[[ "$VLESS_LINK" == vless://* ]] || die "нужна ссылка vless://"
ask WG_CONF "Путь к конфигу WireGuard, Enter — без wg"
[ -z "$WG_CONF" ] || [ -f "$WG_CONF" ] || $SUDO test -f "$WG_CONF" || die "нет файла $WG_CONF"

if [ "$OS" = Darwin ]; then install_deps_mac; else install_deps_linux; fi
perl -MJSON::PP -e1 2>/dev/null || die "нужен perl с модулем JSON::PP"

# --- Cisco: адрес и сертификат -------------------------------------------

CISCO_IP=$(dig +short +time=3 +tries=1 "$CISCO_HOST" | grep -E '^[0-9.]+$' | head -1 || true)
[ -n "$CISCO_IP" ] || die "не резолвится $CISCO_HOST"

CISCO_PIN="${CVPN_CISCO_PIN:-}"
if [ -z "$CISCO_PIN" ]; then
	pin=$(cert_pin "$CISCO_HOST")
	[ -n "$pin" ] || die "не удалось получить сертификат $CISCO_HOST"
	CISCO_PIN="pin-sha256:$pin"
	say "Сертификат $CISCO_HOST ($CISCO_IP): $CISCO_PIN"
	yes_no "Доверять этому сертификату?" || die "отменено"
fi

# --- конфиг sing-box ------------------------------------------------------

exclude=("$CISCO_IP")
if [ "$OS" = Linux ]; then
	while read -r ip; do [ -n "$ip" ] && exclude+=("$ip"); done < <(linux_public_dns)
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
perl lib/vless2singbox.pl "$VLESS_LINK" "${exclude[@]}" > "$tmp/singbox.json"
PATH="${EXTRA_PATH:+$EXTRA_PATH:}$PATH" sing-box check -c "$tmp/singbox.json" || die "sing-box не принял конфиг"

{
	echo "# cvpn: создан install.sh $(date '+%Y-%m-%d')"
	printf 'CISCO_HOST=%q\n' "$CISCO_HOST"
	printf 'CISCO_IP=%q\n' "$CISCO_IP"
	printf 'CISCO_USER=%q\n' "$CISCO_USER"
	printf 'CISCO_GROUP=%q\n' "$CISCO_GROUP"
	printf 'CISCO_PIN=%q\n' "$CISCO_PIN"
	printf 'CORP_DOMAINS=%q\n' "$CORP_DOMAINS"
	printf 'CORP_CHECK_HOST=%q\n' "$CORP_CHECK_HOST"
	printf 'VPNC_SCRIPT=%q\n' "$VPNC_SCRIPT"
	printf 'EXTRA_PATH=%q\n' "$EXTRA_PATH"
} > "$tmp/config"

# --- установка ------------------------------------------------------------

say "Файлы: $LIB, $ETC, /usr/local/bin/cvpn"
$SUDO mkdir -p "$LIB" "$ETC" /usr/local/bin
$SUDO install -m 755 bin/cvpn lib/vpnc-wrapper.sh "$LIB/"
$SUDO install -m 644 "$tmp/config" "$ETC/config"
$SUDO install -m 600 "$tmp/singbox.json" "$ETC/singbox.json"
if [ -n "$WG_CONF" ]; then $SUDO install -m 600 "$WG_CONF" "$ETC/cvpn-wg.conf"; fi
$SUDO ln -sf "$LIB/cvpn" /usr/local/bin/cvpn

if [ "$OS" = Linux ]; then
	singbox_bin=$(command -v sing-box)
	$SUDO tee /etc/systemd/system/cvpn-vless.service >/dev/null <<-EOF
		[Unit]
		Description=cvpn: VLESS (sing-box tun)
		After=network-online.target
		Wants=network-online.target

		[Service]
		ExecStartPre=$LIB/cvpn _rules up
		ExecStart=$singbox_bin run -c $ETC/singbox.json
		ExecStopPost=$LIB/cvpn _rules down
		Restart=on-failure
		RestartSec=3

		[Install]
		WantedBy=multi-user.target
	EOF
	$SUDO systemctl daemon-reload
	if [ "${CVPN_VLESS_BOOT:-}" = 1 ]; then $SUDO systemctl enable cvpn-vless >/dev/null 2>&1; fi
fi

# --- пароль Cisco ---------------------------------------------------------

say "Пароль Cisco"
if [ "$OS" = Darwin ]; then
	/usr/local/bin/cvpn password
elif [ -n "${CVPN_CISCO_PASSWORD:-}" ]; then
	(umask 077; printf '%s' "$CVPN_CISCO_PASSWORD" | $SUDO tee "$ETC/cisco-password" >/dev/null)
	$SUDO chmod 600 "$ETC/cisco-password"
	echo "сохранён в $ETC/cisco-password (читает только root)"
else
	$SUDO /usr/local/bin/cvpn password
fi

# --- sudo без пароля -------------------------------------------------------

sudo_user="${SUDO_USER:-$USER}"
if [ "$sudo_user" != root ]; then
	if [ "${CVPN_SUDOERS:-}" = 1 ] || { [ -z "${CVPN_SUDOERS+x}" ] && yes_no "Запускать cvpn без пароля sudo?"; }; then
		rule="$sudo_user ALL=(root) NOPASSWD: $LIB/cvpn"
		echo "$rule" > "$tmp/sudoers"
		$SUDO visudo -cf "$tmp/sudoers" >/dev/null || die "sudoers не прошёл проверку"
		$SUDO install -m 440 "$tmp/sudoers" /etc/sudoers.d/cvpn
		echo "добавлено: $rule"
	fi
fi

# --- предупреждения --------------------------------------------------------

if [ "$OS" = Linux ] && [ -n "$CORP_DOMAINS" ] &&
	! readlink /etc/resolv.conf | grep -q 'systemd/resolve/stub-resolv.conf'; then
	say "Внимание: /etc/resolv.conf не смотрит в systemd-resolved"
	echo "Split-DNS для корп-доменов работает только через resolved. Включить:"
	echo "  sudo ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf"
fi

say "Готово"
cat <<-EOF
	  cvpn on [otp]      vless + cisco
	  cvpn off           выключить всё
	  cvpn               статус
	  cvpn cisco|vless|wg on|off
EOF
