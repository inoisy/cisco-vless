#!/bin/sh
# Обёртка над штатным vpnc-script для openconnect (--script).
#
# Штатный скрипт, получив от сервера DNS, перехватывает DNS всей системы
# (на macOS — networksetup -setdnsservers, OverridePrimary, резолвер на tun
# без привязки к домену): публичные домены уходят в корп-DNS и не
# резолвятся, а tun cisco спорит с vless/wg за primary-интерфейс. Поэтому
# DNS у него отбираем и делаем split-DNS: в корп-DNS идут только корп-домены
# (CORP_DOMAINS из конфига плюс домены, которые прислал сервер).
# Маршруты штатный скрипт настраивает как обычно.

. /usr/local/etc/cvpn/config

STATE=/var/run/cvpn-dns-domains

corp_dns="$INTERNAL_IP4_DNS"
domains=$(echo "$CORP_DOMAINS $CISCO_DEF_DOMAIN $CISCO_SPLIT_DNS" | tr ',' ' ')
unset INTERNAL_IP4_DNS INTERNAL_IP6_DNS CISCO_DEF_DOMAIN CISCO_SPLIT_DNS

"$VPNC_SCRIPT"
rc=$?

# Домен идёт в имя файла под root — пропускаем всё, кроме букв, цифр, точек
# и дефисов.
safe_domains() {
	for d in $domains; do
		case "$d" in
			''|.*|*[!A-Za-z0-9.-]*) ;;
			*) echo "$d" ;;
		esac
	done
}

# reason и TUNDEV выставляет openconnect.
# shellcheck disable=SC2154
case "$(uname -s)/$reason" in
	Darwin/connect)
		[ -n "$corp_dns" ] || exit "$rc"
		mkdir -p /etc/resolver
		: > "$STATE"
		for d in $(safe_domains); do
			: > "/etc/resolver/$d"
			for ns in $corp_dns; do
				echo "nameserver $ns" >> "/etc/resolver/$d"
			done
			echo "$d" >> "$STATE"
		done
		;;
	Darwin/disconnect)
		if [ -f "$STATE" ]; then
			while read -r d; do rm -f "/etc/resolver/$d"; done < "$STATE"
			rm -f "$STATE"
		fi
		;;
	Linux/connect)
		list=$(safe_domains)
		[ -n "$corp_dns" ] && [ -n "$list" ] || exit "$rc"
		if command -v resolvectl >/dev/null 2>&1; then
			# shellcheck disable=SC2086
			resolvectl dns "$TUNDEV" $corp_dns
			# shellcheck disable=SC2046
			resolvectl domain "$TUNDEV" $(for d in $list; do printf '~%s ' "$d"; done)
			resolvectl default-route "$TUNDEV" false 2>/dev/null
		else
			echo "cvpn: нет systemd-resolved, split-DNS для $list не настроен" >&2
		fi
		;;
esac

exit "$rc"
