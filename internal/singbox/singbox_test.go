package singbox

import (
	"encoding/json"
	"strings"
	"testing"
)

const reality = "vless://11111111-2222-3333-4444-555555555555@ex.com:443?security=reality&sni=www.ms.com&fp=chrome&pbk=PUBKEY&sid=ab12&flow=xtls-rprx-vision&type=tcp#name"

func parse(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestReality(t *testing.T) {
	b, err := Config(reality, Options{Exclude: []string{"1.2.3.4", "10.0.0.0/8"}, GOOS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	m := parse(t, b)
	out := m["outbounds"].([]any)[0].(map[string]any)
	if out["server"] != "ex.com" || out["server_port"].(float64) != 443 || out["flow"] != "xtls-rprx-vision" {
		t.Fatalf("outbound: %v", out)
	}
	r := out["tls"].(map[string]any)["reality"].(map[string]any)
	if r["public_key"] != "PUBKEY" || r["short_id"] != "ab12" {
		t.Fatalf("reality: %v", r)
	}
	tun := m["inbounds"].([]any)[0].(map[string]any)
	if tun["interface_name"] != "tun0" {
		t.Fatalf("tun name: %v", tun["interface_name"])
	}
	ex := tun["route_exclude_address"].([]any)
	if ex[0] != "1.2.3.4/32" || ex[1] != "10.0.0.0/8" {
		t.Fatalf("exclude: %v", ex)
	}
}

func TestTunNamePerOS(t *testing.T) {
	for goos, want := range map[string]any{"darwin": nil, "windows": "cvpn-vless"} {
		b, err := Config(reality, Options{GOOS: goos})
		if err != nil {
			t.Fatal(err)
		}
		tun := parse(t, b)["inbounds"].([]any)[0].(map[string]any)
		if tun["interface_name"] != want {
			t.Errorf("%s: interface_name=%v", goos, tun["interface_name"])
		}
		if _, ok := tun["route_exclude_address"]; ok {
			t.Errorf("%s: лишний route_exclude_address", goos)
		}
	}
}

func TestIPv6AndEscapes(t *testing.T) {
	b, err := Config("vless://a%2Bb@[2001:db8::1]:8443?security=tls&sni=x.com", Options{GOOS: "darwin"})
	if err != nil {
		t.Fatal(err)
	}
	out := parse(t, b)["outbounds"].([]any)[0].(map[string]any)
	if out["server"] != "2001:db8::1" || out["uuid"] != "a+b" {
		t.Fatalf("outbound: %v", out)
	}
}

func TestErrors(t *testing.T) {
	for link, want := range map[string]string{
		"vmess://x":                        "не похоже",
		"vless://u@h:443?type=ws":          "транспорт",
		"vless://u@h:443?security=reality": "pbk",
		"vless://u@h:443?security=xtls":    "security",
	} {
		if _, err := Config(link, Options{GOOS: "linux"}); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err=%v, want %q", link, err, want)
		}
	}
}

func TestRussiaDirect(t *testing.T) {
	b, err := Config(reality, Options{GOOS: "darwin", RUDirect: true, GeoIP: "/etc/cvpn/geoip-ru.srs",
		ProxyDomains: []string{"api.example.ru"}})
	if err != nil {
		t.Fatal(err)
	}
	m := parse(t, b)
	rules := m["route"].(map[string]any)["rules"].([]any)
	var got []string
	for _, r := range rules[2:] {
		r := r.(map[string]any)
		got = append(got, r["outbound"].(string))
	}
	// api.example.ru — в proxy раньше зоны .ru, иначе правило зоны его перехватит.
	if strings.Join(got, ",") != "direct,proxy,direct,direct" {
		t.Fatalf("порядок правил: %v", got)
	}
	if rules[3].(map[string]any)["domain"].([]any)[0] != "api.example.ru" {
		t.Fatalf("исключение: %v", rules[3])
	}
	dns := m["dns"].(map[string]any)["rules"].([]any)
	if dns[1].(map[string]any)["server"] != "ru" {
		t.Fatalf("dns: %v", dns)
	}
}

func TestGeoIPLocal(t *testing.T) {
	b, err := Config(reality, Options{GOOS: "darwin", RUDirect: true, GeoIP: "/etc/cvpn/geoip-ru.srs"})
	if err != nil {
		t.Fatal(err)
	}
	rs := parse(t, b)["route"].(map[string]any)["rule_set"].([]any)[0].(map[string]any)
	if rs["type"] != "local" || rs["path"] != "/etc/cvpn/geoip-ru.srs" || rs["url"] != nil {
		t.Fatalf("rule_set: %v", rs)
	}
}

func TestNoGeoIP(t *testing.T) {
	b, err := Config(reality, Options{GOOS: "darwin", RUDirect: true})
	if err != nil {
		t.Fatal(err)
	}
	route := parse(t, b)["route"].(map[string]any)
	if _, ok := route["rule_set"]; ok {
		t.Fatalf("rule_set без файла: %v", route["rule_set"])
	}
	for _, r := range route["rules"].([]any) {
		if _, ok := r.(map[string]any)["rule_set"]; ok {
			t.Fatalf("правило geoip без файла: %v", r)
		}
	}
}

// По умолчанию всё идёт через VLESS: ни российских правил, ни DNS Яндекса.
func TestNoRussiaDirect(t *testing.T) {
	b, err := Config(reality, Options{GOOS: "darwin", GeoIP: "/etc/cvpn/geoip-ru.srs"})
	if err != nil {
		t.Fatal(err)
	}
	m := parse(t, b)
	if strings.Contains(string(b), "77.88.8.8") || strings.Contains(string(b), "geoip-ru") {
		t.Fatalf("российские правила без RUDirect: %s", b)
	}
	if r := m["route"].(map[string]any)["rules"].([]any); len(r) != 3 {
		t.Fatalf("правила: %v", r)
	}
	if r := m["dns"].(map[string]any)["rules"].([]any); len(r) != 0 {
		t.Fatalf("правила DNS: %v", r)
	}
}
