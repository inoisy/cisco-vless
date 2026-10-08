package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUnquoteShell(t *testing.T) {
	for in, want := range map[string]string{
		`vpn.example.com`:        "vpn.example.com",
		`''`:                     "",
		`example.ru\ corp.local`: "example.ru corp.local",
		`pin-sha256:abc\+/=`:     "pin-sha256:abc+/=",
		`$'a\'b'`:                "a'b",
		`'x y'`:                  "x y",
	} {
		if got := unquoteShell(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestLoadLegacy(t *testing.T) {
	dir := t.TempDir()
	p := Paths{Etc: dir}
	legacy := "# cvpn: создан install.sh 2026-09-25\n" +
		"CISCO_HOST=vpn.example.com\nCISCO_IP=1.2.3.4\nCISCO_USER=ivan\nCISCO_GROUP=''\n" +
		"CISCO_PIN=pin-sha256:AAA=\nCORP_DOMAINS=example.ru\\ corp.local\nCORP_CHECK_HOST=git.example.ru\n" +
		"VPNC_SCRIPT=/opt/homebrew/etc/vpnc/vpnc-script\nEXTRA_PATH=/opt/homebrew/bin:/opt/homebrew/sbin\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.CiscoHost != "vpn.example.com" || c.CiscoUser != "ivan" || c.CiscoGroup != "" ||
		!reflect.DeepEqual(c.CorpDomains, []string{"example.ru", "corp.local"}) ||
		c.VPNCScript != "/opt/homebrew/etc/vpnc/vpnc-script" {
		t.Fatalf("legacy: %+v", c)
	}
	// config.json важнее старого конфига.
	c.CiscoUser = "petr"
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	c2, _ := Load(p)
	if c2.CiscoUser != "petr" {
		t.Fatalf("после Save: %+v", c2)
	}
}
