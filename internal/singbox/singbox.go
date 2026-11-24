// Package singbox превращает ссылку vless:// в конфиг sing-box: tun, трафик
// через VLESS, по настройке российские сайты — напрямую.
package singbox

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// TunName — имя tun sing-box по ОС. Linux: закрепляем, иначе после рестарта
// sing-box берёт первый свободный tunN, а на имена завязан фаервол хоста
// (tun1 — cisco). Windows: по имени фильтруем свои адаптеры при поиске
// физического default. macOS: utunN выдаёт система.
func TunName(goos string) string {
	switch goos {
	case "linux":
		return "tun0"
	case "windows":
		return "cvpn-vless"
	}
	return ""
}

// Options.RUDirect: российские сайты идут напрямую, с адреса провайдера —
// маркетплейсы, банки и Метрика режут или проверяют зарубежные IP. Домены — по зонам, остальное
// (CDN на .com, vk.com) — по geoip. DNS для зон — Яндекс напрямую, чтобы CDN
// отдавал российские узлы.
var ruSuffixes = []string{"ru", "su", "xn--p1ai", "xn--p1acf", "moscow", "xn--80adxhks"}

// Options — всё, кроме ссылок, из чего собирается конфиг.
type Options struct {
	// Exclude — адреса в обход туннеля (шлюз Cisco, публичные DNS провайдера);
	// без маски считаются /32.
	Exclude []string
	GOOS    string
	// RUDirect — российские сайты напрямую.
	RUDirect bool
	// GeoIP — путь к локальному geoip-ru.srs (см. EnsureGeoIP), нужен при
	// RUDirect; пустой — российские IP без правила, напрямую идут только зоны.
	GeoIP string
	// ProxyDomains — имена хостов, которые идут через VLESS вопреки RUDirect.
	ProxyDomains []string
}

var linkRe = regexp.MustCompile(`^vless://([^@]+)@(\[[^\]]+\]|[^:/?#]+):(\d+)/?(?:\?([^#]*))?(?:#.*)?$`)

// Config строит конфиг sing-box. links — одна или несколько ссылок
// (vless://, hysteria2://) через пробел или перевод строки. Одна — outbound
// proxy; несколько — каждая своим outbound, а proxy — группа urltest: sing-box
// сам держит живой сервер с меньшей задержкой и уходит на другой, когда этот
// перестал отвечать (DE напрямую ↔ RU через посредника). Ссылки, которые
// sing-box не поднимет (xhttp), в группе пропускаются.
func Config(links string, o Options) ([]byte, error) {
	outs, err := Outbounds(links)
	if err != nil {
		return nil, err
	}

	tun := map[string]any{
		"type": "tun",
		"tag":  "tun-in",
		// Вне корп-сетей 10/8 и 172.16/12, чтобы не пересекаться с cisco.
		"address":      []string{"198.18.0.1/30"},
		"auto_route":   true,
		"strict_route": false,
		"stack":        "system",
	}
	if name := TunName(o.GOOS); name != "" {
		tun["interface_name"] = name
	}
	if len(o.Exclude) > 0 {
		ex := make([]string, 0, len(o.Exclude))
		for _, a := range o.Exclude {
			if !strings.Contains(a, "/") {
				a += "/32"
			}
			ex = append(ex, a)
		}
		tun["route_exclude_address"] = ex
	}

	// DNS. На Linux sing-box регистрирует свой tun в systemd-resolved как DNS
	// для всех доменов, поэтому запросы надо перехватить и ответить самому:
	// DoH через VLESS — заодно без подмены ответов провайдером. bootstrap
	// нужен только чтобы резолвить адрес сервера VLESS, если в ссылке домен.
	dns := []any{
		map[string]any{"type": "https", "tag": "remote", "server": "1.1.1.1", "detour": "proxy"},
		map[string]any{"type": "udp", "tag": "bootstrap", "server": "1.1.1.1"},
	}
	dnsRules := []any{}
	rules := []any{
		map[string]any{"action": "sniff"},
		map[string]any{"protocol": "dns", "action": "hijack-dns"},
		map[string]any{"ip_is_private": true, "outbound": "direct"},
	}
	// Раньше зон: иначе правило .ru перехватит хост.
	if len(o.ProxyDomains) > 0 {
		dnsRules = append(dnsRules, map[string]any{"domain": o.ProxyDomains, "server": "remote"})
		rules = append(rules, map[string]any{"domain": o.ProxyDomains, "outbound": "proxy"})
	}
	if o.RUDirect {
		dns = append(dns, map[string]any{"type": "udp", "tag": "ru", "server": "77.88.8.8"})
		dnsRules = append(dnsRules, map[string]any{"domain_suffix": ruSuffixes, "server": "ru"})
		rules = append(rules, map[string]any{"domain_suffix": ruSuffixes, "outbound": "direct"})
	}
	cfg := map[string]any{
		"log": map[string]any{"level": "warn"},
		"dns": map[string]any{
			"servers": dns,
			"rules":   dnsRules,
			"final":   "remote",
		},
		"inbounds":  []any{tun},
		"outbounds": append(outs, map[string]any{"type": "direct", "tag": "direct"}),
		"route": map[string]any{
			"auto_detect_interface":   true,
			"default_domain_resolver": "bootstrap",
			"rules":                   rules,
			"final":                   "proxy",
		},
	}
	// Rule-set локальный: удалённый sing-box качает на каждом старте и, не
	// успев за 10 с, падает ("initialize rule-set take too much time" →
	// "start service: context canceled").
	if o.RUDirect && o.GeoIP != "" {
		route := cfg["route"].(map[string]any)
		route["rules"] = append(route["rules"].([]any),
			map[string]any{"rule_set": "geoip-ru", "outbound": "direct"})
		route["rule_set"] = []any{map[string]any{
			"type": "local", "tag": "geoip-ru", "format": "binary", "path": o.GeoIP,
		}}
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// vlessOutbound — outbound из ссылки vless://.
func vlessOutbound(link, tag string) (map[string]any, error) {
	m := linkRe.FindStringSubmatch(strings.TrimSpace(link))
	if m == nil {
		return nil, fmt.Errorf("не похоже на ссылку vless://uuid@host:port?...")
	}
	uuid := unesc(m[1])
	host := strings.Trim(m[2], "[]")
	port, err := strconv.Atoi(m[3])
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("плохой порт %q", m[3])
	}
	p := map[string]string{}
	for _, kv := range strings.Split(m[4], "&") {
		if kv == "" {
			continue
		}
		k, v, _ := strings.Cut(kv, "=")
		p[k] = unesc(v)
	}

	if t := p["type"]; t != "" && t != "tcp" {
		return nil, fmt.Errorf("транспорт %q пока не поддержан, только tcp", t)
	}

	out := map[string]any{
		"type":        "vless",
		"tag":         tag,
		"server":      host,
		"server_port": port,
		"uuid":        uuid,
	}
	if p["flow"] != "" {
		out["flow"] = p["flow"]
	}

	security := p["security"]
	if security == "" {
		security = "none"
	}
	switch security {
	case "reality", "tls":
		tls := map[string]any{"enabled": true}
		if p["sni"] != "" {
			tls["server_name"] = p["sni"]
		}
		if p["fp"] != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": p["fp"]}
		}
		if security == "reality" {
			if p["pbk"] == "" {
				return nil, fmt.Errorf("в ссылке нет pbk (публичный ключ reality)")
			}
			r := map[string]any{"enabled": true, "public_key": p["pbk"]}
			if p["sid"] != "" {
				r["short_id"] = p["sid"]
			}
			tls["reality"] = r
		}
		out["tls"] = tls
	case "none":
	default:
		return nil, fmt.Errorf("security %q не поддержан", security)
	}
	return out, nil
}

func unesc(s string) string {
	if v, err := url.QueryUnescape(s); err == nil {
		return v
	}
	return s
}
