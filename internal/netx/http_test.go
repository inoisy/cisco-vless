package netx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Каждый запрос — по новому соединению, иначе после смены выхода статус
// показывает старый IP.
func TestHTTPGetNewConnection(t *testing.T) {
	addrs := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addrs[r.RemoteAddr] = true
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	for i := 0; i < 2; i++ {
		if _, err := httpGet(srv.URL, 5*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if len(addrs) != 2 {
		t.Fatalf("соединений %d, нужно 2", len(addrs))
	}
}
