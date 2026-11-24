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
	de    = "vless://11111111-2222-3333-4444-555555555555@192.0.2.10:443?security=reality&sni=www.google.com&fp=chrome&pbk=PUBKEY&sid=ab12&flow=xtls-rprx-vision&type=tcp#DE-TCP-443"
	ru    = "vless://11111111-2222-3333-4444-555555555555@198.51.100.20:9443?security=reality&sni=www.google.com&fp=chrome&pbk=PUBKEY&sid=ab12&flow=xtls-rprx-vision&type=tcp#RU-TCP-9443"
	xhttp = "vless://11111111-2222-3333-4444-555555555555@198.51.100.20:2053?security=reality&sni=dl.google.com&pbk=PUBKEY&type=xhttp&path=%2Fxh#RU-XHTTP-2053"
	hy2   = "hysteria2://pa%3Ass@198.51.100.20:4443?sni=www.bing.com&insecure=1&obfs=salamander&obfs-password=ob#RU-HY2-4443"
)

func TestGroup(t *testing.T) {
	b, err := Config(strings.Join([]string{de, ru, xhttp, hy2, de}, "\n"), Options{GOOS: "darwin"})
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
	if _, err := Config(xhttp+" "+xhttp, Options{GOOS: "linux"}); err == nil || !strings.Contains(err.Error(), "ни одна") {
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

func TestResolveMixed(t *testing.T) {
	up := map[string]bool{"/a": true, "/b": true}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up[r.URL.Path] {
			http.Error(w, "down", 503)
			return
		}
		if r.URL.Path == "/a" {
			w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(de + "\n"))))
			return
		}
		w.Write([]byte(hy2 + "\n"))
	}))
	defer srv.Close()
	// httptest отдаёт http://, а Sources считает подпиской только https:// —
	// разбор проверяем на https-адресах, скачивание — через resolveList.
	a, b := srv.URL+"/a", srv.URL+"/b"
	dir := t.TempDir()
	cache := func(u string) string { return filepath.Join(dir, filepath.Base(u)) }

	spec := ru + " " + a + " " + b
	if subs, links := Sources(strings.ReplaceAll(spec, "http://", "https://")); len(subs) != 2 || len(links) != 1 {
		t.Fatalf("subs=%d links=%d", len(subs), len(links))
	}
	got, warns, err := resolveList([]string{a, b}, []string{ru}, cache)
	if err != nil || len(warns) != 0 || len(strings.Fields(got)) != 3 {
		t.Fatalf("got=%q warns=%v err=%v", got, warns, err)
	}
	// Одна подписка упала — берётся её копия, ссылки остаются.
	up["/a"] = false
	got, warns, err = resolveList([]string{a, b}, []string{ru}, cache)
	if err != nil || len(warns) != 1 || !strings.Contains(got, "DE-TCP-443") {
		t.Fatalf("got=%q warns=%v err=%v", got, warns, err)
	}
	// Упала подписка без копии — пропускается, остальное живо.
	os.Remove(cache(a))
	got, warns, err = resolveList([]string{a, b}, []string{ru}, cache)
	if err != nil || len(warns) != 1 || strings.Contains(got, "DE-TCP-443") || len(strings.Fields(got)) != 2 {
		t.Fatalf("got=%q warns=%v err=%v", got, warns, err)
	}
	// Ничего не осталось — ошибка.
	up["/b"] = false
	os.Remove(cache(b))
	if _, _, err := resolveList([]string{a, b}, nil, cache); err == nil {
		t.Fatal("ждали ошибку")
	}
}

func TestCheckSources(t *testing.T) {
	if err := CheckSources(de + " https://sub.example.com/tok " + hy2); err != nil {
		t.Fatal(err)
	}
	if err := CheckSources(de + " trojan://x@h:1"); err == nil {
		t.Fatal("trojan:// должен отказать")
	}
}

func TestSubscriptionErrNoToken(t *testing.T) {
	_, err := Subscription("https://127.0.0.1:1/SECRETTOKEN", filepath.Join(t.TempDir(), "none"))
	if err == nil || strings.Contains(err.Error(), "SECRETTOKEN") {
		t.Fatalf("err=%v", err)
	}
}
