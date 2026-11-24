package daemon

import (
	"reflect"
	"testing"

	"github.com/inoisy/cisco-vless/internal/config"
)

func TestSetSettingsRouting(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{paths: config.Paths{Etc: dir, Run: dir, Log: dir}, cfg: &config.Config{Routing: config.LegacyRouting()}}

	// Клиент без этой настройки (старый CLI) её не стирает.
	if err := m.SetSettings(Settings{}); err != nil {
		t.Fatal(err)
	}
	if got := m.Settings().Routing; !reflect.DeepEqual(got, config.LegacyRouting()) {
		t.Fatalf("routing: %+v", got)
	}

	in := &config.Routing{ProxyDomains: []string{"API.example.com", "не домен!", "api.example.com"}}
	if err := m.SetSettings(Settings{Routing: in}); err != nil {
		t.Fatal(err)
	}
	want := &config.Routing{ProxyDomains: []string{"api.example.com"}}
	if got := m.Settings().Routing; !reflect.DeepEqual(got, want) {
		t.Fatalf("routing: %+v", got)
	}
	c, err := config.Load(m.paths)
	if err != nil || !reflect.DeepEqual(c.Routing, want) {
		t.Fatalf("config.json: %+v err=%v", c.Routing, err)
	}
}
