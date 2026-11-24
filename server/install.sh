#!/bin/bash
# Обновить bash-версию cvpn на сервере (Linux, systemd), где sing-box держит
# отдельный юнит cvpn-vless, а singbox.json правят руками и чужие скрипты
# (сторож, обновление подписки).
#
# Что делает:
#   - кладёт cvpn и vpnc-wrapper.sh в /usr/local/lib/cvpn (прежние — в *.bak-<время>);
#   - ставит /usr/local/bin/cvpn -> /usr/local/lib/cvpn/cvpn;
#   - обновляет /etc/systemd/system/cvpn-vless.service, если он отличается
#     (прежний — в *.bak-<время>), и делает daemon-reload.
#
# Чего НЕ делает: не трогает /usr/local/etc/cvpn (config, singbox.json,
# пароль), не перезапускает и не включает cvpn-vless — рестарт рвёт
# соединения через tun, его делают руками в удобное окно:
#   systemctl stop cvpn-vless && systemctl start cvpn-vless
#
# Первая установка не поддержана: нужен готовый /usr/local/etc/cvpn/config и
# singbox.json. Go-версия (корневой install.sh) — для рабочих машин, не сюда.
#
# Usage: sudo server/install.sh [--dry-run]

set -euo pipefail

LIB=/usr/local/lib/cvpn
ETC=/usr/local/etc/cvpn
UNIT=/etc/systemd/system/cvpn-vless.service
SRC=$(cd "$(dirname "$0")" && pwd)
STAMP=$(date +%Y%m%d%H%M%S)
DRY=0
[ "${1:-}" = --dry-run ] && DRY=1

say() { printf '==> %s\n' "$*"; }
die() { printf 'server/install.sh: %s\n' "$*" >&2; exit 1; }
run() { if [ "$DRY" = 1 ]; then printf '    [dry-run] %s\n' "$*"; else "$@"; fi; }

[ "$(uname -s)" = Linux ] || die "только Linux"
[ "$(id -u)" -eq 0 ] || die "нужен root (sudo)"
command -v systemctl >/dev/null || die "нужен systemd"
[ -f "$ETC/config" ] || die "нет $ETC/config — первая установка этим скриптом не делается"
[ -f "$ETC/singbox.json" ] || die "нет $ETC/singbox.json — первая установка этим скриптом не делается"
if [ -f "$LIB/cvpn" ] && ! head -c 64 "$LIB/cvpn" | grep -q '^#!/bin/bash'; then
	die "$LIB/cvpn — не bash (стоит Go-версия?). Сначала откат на bash, см. README «Сервер»"
fi
if systemctl cat cvpn.service >/dev/null 2>&1; then
	die "есть служба cvpn (Go-версия) — вместе с cvpn-vless они будут драться за tun0"
fi

# install_file <src> <dst>: заменить, если отличается; прежний — в бэкап.
install_file() {
	local src=$1 dst=$2
	if [ -f "$dst" ] && cmp -s "$src" "$dst"; then
		say "$dst — без изменений"
		return 1
	fi
	[ -f "$dst" ] && run cp -p "$dst" "$dst.bak-$STAMP" && say "бэкап: $dst.bak-$STAMP"
	run install -m "$3" "$src" "$dst"
	say "$dst — обновлён"
}

run mkdir -p "$LIB"
install_file "$SRC/cvpn" "$LIB/cvpn" 755 || true
install_file "$SRC/vpnc-wrapper.sh" "$LIB/vpnc-wrapper.sh" 755 || true
run ln -sfn "$LIB/cvpn" /usr/local/bin/cvpn

if install_file "$SRC/cvpn-vless.service" "$UNIT" 644; then
	run systemctl daemon-reload
	say "юнит обновлён; вступит в силу при следующем stop/start cvpn-vless (сам не перезапускаю)"
fi

say "готово. Проверка: bash -n $LIB/cvpn && ip rule | grep -E '^(55|60):'"
