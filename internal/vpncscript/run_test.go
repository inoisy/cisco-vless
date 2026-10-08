//go:build !windows

package vpncscript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inoisy/cisco-vless/internal/config"
)

// Штатный скрипт получает окружение openconnect без DNS: его DNS делаем мы.
func TestRunStripsDNS(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "env")
	stock := filepath.Join(dir, "vpnc-script")
	if err := os.WriteFile(stock, []byte("#!/bin/sh\nenv > "+out+"\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvStock, stock)
	t.Setenv("reason", "pre-init")
	t.Setenv("INTERNAL_IP4_DNS", "10.0.0.53")
	t.Setenv("CISCO_SPLIT_DNS", "corp.ru")
	t.Setenv("CISCO_SPLIT_INC", "1")

	if rc := Run(&config.Config{}, config.Paths{Run: dir}); rc != 3 {
		t.Fatalf("rc=%d, want 3 (код штатного скрипта)", rc)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	env := string(b)
	for _, v := range dnsVars {
		if strings.Contains(env, v+"=") {
			t.Errorf("%s дошёл до штатного скрипта", v)
		}
	}
	if !strings.Contains(env, "CISCO_SPLIT_INC=1") || !strings.Contains(env, "reason=pre-init") {
		t.Errorf("маршрутные переменные потерялись:\n%s", env)
	}
}
