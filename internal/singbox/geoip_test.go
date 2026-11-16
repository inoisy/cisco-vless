package singbox

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Write([]byte("SRS\x01data"))
		case "/html":
			w.Write([]byte("<html>blocked</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "geoip-ru.srs")

	if err := download(srv.URL+"/ok", path, time.Second); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "SRS\x01data" {
		t.Fatalf("файл: %q", b)
	}
	// Заглушка провайдера или 404 не затирают годный файл.
	for _, u := range []string{"/html", "/404"} {
		if err := download(srv.URL+u, path, time.Second); err == nil {
			t.Fatalf("%s: ждали ошибку", u)
		}
	}
	if b, _ := os.ReadFile(path); string(b) != "SRS\x01data" {
		t.Fatalf("файл затёрт: %q", b)
	}
}

func TestEnsureGeoIPFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "geoip-ru.srs")
	os.WriteFile(path, []byte("SRS\x01"), 0o644)
	// Свежий файл — без сети.
	if ok, err := EnsureGeoIP(path); !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
