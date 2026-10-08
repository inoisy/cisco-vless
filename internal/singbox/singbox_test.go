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
	b, err := Config(reality, []string{"1.2.3.4", "10.0.0.0/8"}, "linux")
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
		b, err := Config(reality, nil, goos)
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
	b, err := Config("vless://a%2Bb@[2001:db8::1]:8443?security=tls&sni=x.com", nil, "darwin")
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
		if _, err := Config(link, nil, "linux"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err=%v, want %q", link, err, want)
		}
	}
}
