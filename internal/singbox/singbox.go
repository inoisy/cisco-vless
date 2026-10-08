// Package singbox превращает ссылку vless:// в конфиг sing-box: tun, весь
// трафик через VLESS.
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

var linkRe = regexp.MustCompile(`^vless://([^@]+)@(\[[^\]]+\]|[^:/?#]+):(\d+)/?(?:\?([^#]*))?(?:#.*)?$`)

// Config строит конфиг sing-box. exclude — адреса в обход туннеля (шлюз
// Cisco, публичные DNS провайдера); без маски считаются /32.
func Config(link string, exclude []string, goos string) ([]byte, error) {
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
		"tag":         "proxy",
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

	tun := map[string]any{
		"type": "tun",
		"tag":  "tun-in",
		// Вне корп-сетей 10/8 и 172.16/12, чтобы не пересекаться с cisco.
		"address":      []string{"198.18.0.1/30"},
		"auto_route":   true,
		"strict_route": false,
		"stack":        "system",
	}
	if name := TunName(goos); name != "" {
		tun["interface_name"] = name
	}
	if len(exclude) > 0 {
		ex := make([]string, 0, len(exclude))
		for _, a := range exclude {
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
	cfg := map[string]any{
		"log": map[string]any{"level": "warn"},
		"dns": map[string]any{
			"servers": []any{
				map[string]any{"type": "https", "tag": "remote", "server": "1.1.1.1", "detour": "proxy"},
				map[string]any{"type": "udp", "tag": "bootstrap", "server": "1.1.1.1"},
			},
			"final": "remote",
		},
		"inbounds":  []any{tun},
		"outbounds": []any{out, map[string]any{"type": "direct", "tag": "direct"}},
		"route": map[string]any{
			"auto_detect_interface":   true,
			"default_domain_resolver": "bootstrap",
			"rules": []any{
				map[string]any{"action": "sniff"},
				map[string]any{"protocol": "dns", "action": "hijack-dns"},
				map[string]any{"ip_is_private": true, "outbound": "direct"},
			},
			"final": "proxy",
		},
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func unesc(s string) string {
	if v, err := url.QueryUnescape(s); err == nil {
		return v
	}
	return s
}
