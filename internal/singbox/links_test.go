package singbox

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	de    = "vless://11111111-2222-3333-4444-555555555555@92.51.37.88:443?security=reality&sni=www.google.com&fp=chrome&pbk=PUBKEY&sid=ab12&flow=xtls-rprx-vision&type=tcp#DE-TCP-443"
	ru    = "vless://11111111-2222-3333-4444-555555555555@135.106.225.250:9443?security=reality&sni=www.google.com&fp=chrome&pbk=PUBKEY&sid=ab12&flow=xtls-rprx-vision&type=tcp#RU-TCP-9443"
	xhttp = "vless://11111111-2222-3333-4444-555555555555@135.106.225.250:2053?security=reality&sni=dl.google.com&pbk=PUBKEY&type=xhttp&path=%2Fxh#RU-XHTTP-2053"
	hy2   = "hysteria2://pa%3Ass@135.106.225.250:4443?sni=www.bing.com&insecure=1&obfs=salamander&obfs-password=ob#RU-HY2-4443"
)

func TestGroup(t *testing.T) {
	b, err := Config(strings.Join([]string{de, ru, xhttp, hy2, de}, "\n"), nil, "darwin", "")
	if err != nil {
		t.Fatal(err)
	}
	outs := parse(t, b)["outbounds"].([]any)
	g := outs[0].(map[string]any)
	if g["type"] != "urltest" || g["tag"] != "proxy" {
		t.Fatalf("группа: %v", g)
	}
	// xhttp sing-box не умеет — пропущен; повтор имени — с суффиксом.
	var tags []string
	for _, x := range g["outbounds"].([]any) {
		tags = append(tags, x.(string))
	}
	if strings.Join(tags, ",") != "DE-TCP-443,RU-TCP-9443,RU-HY2-4443,DE-TCP-443-2" {
		t.Fatalf("теги: %v", tags)
	}
	if outs[len(outs)-1].(map[string]any)["tag"] != "direct" || len(outs) != 6 {
		t.Fatalf("outbounds: %v", outs)
	}
	h := outs[3].(map[string]any)
	tls := h["tls"].(map[string]any)
	if h["type"] != "hysteria2" || h["password"] != "pa:ss" || h["server_port"].(float64) != 4443 ||
		tls["server_name"] != "www.bing.com" || tls["insecure"] != true ||
		h["obfs"].(map[string]any)["password"] != "ob" {
		t.Fatalf("hysteria2: %v", h)
	}
}

func TestGroupAllBad(t *testing.T) {
	if _, err := Config(xhttp+" "+xhttp, nil, "linux", ""); err == nil || !strings.Contains(err.Error(), "ни одна") {
		t.Fatalf("err=%v", err)
	}
}

func TestSubscription(t *testing.T) {
	body := base64.StdEncoding.EncodeToString([]byte(de + "\n" + hy2 + "\n"))
	up := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			http.Error(w, "down", 502)
			return
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	cache := filepath.Join(t.TempDir(), "subscription.txt")

	links, err := Subscription(srv.URL, cache)
	if err != nil || strings.Fields(links)[1] != hy2 {
		t.Fatalf("links=%q err=%v", links, err)
	}
	if b, _ := os.ReadFile(cache); string(b) != links {
		t.Fatalf("копия: %q", b)
	}
	// Сервер подписки недоступен — берём копию и говорим об этом.
	up = false
	links2, err := Subscription(srv.URL, cache)
	if links2 != links || err == nil {
		t.Fatalf("links=%q err=%v", links2, err)
	}
	if _, err := Subscription(srv.URL, filepath.Join(t.TempDir(), "none")); err == nil {
		t.Fatal("без копии ждали ошибку")
	}
}

func TestIsSubscription(t *testing.T) {
	for s, want := range map[string]bool{
		"https://sub.example.com:9443/tok": true,
		de:                                 false,
		"https://a b":                      false,
	} {
		if IsSubscription(s) != want {
			t.Errorf("%q: want %v", s, want)
		}
	}
}
