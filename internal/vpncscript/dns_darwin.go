package vpncscript

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/inoisy/cisco-vless/internal/config"
)

// macOS: /etc/resolver/<домен> — резолвер только для этого домена.
func setSplitDNS(p config.Paths, _ string, dns, domains []string) error {
	if err := os.MkdirAll("/etc/resolver", 0o755); err != nil {
		return err
	}
	var body strings.Builder
	for _, ns := range dns {
		body.WriteString("nameserver " + ns + "\n")
	}
	var done []string
	for _, d := range domains {
		if err := os.WriteFile(filepath.Join("/etc/resolver", d), []byte(body.String()), 0o644); err != nil {
			return err
		}
		done = append(done, d)
	}
	return os.WriteFile(p.DNSState(), []byte(strings.Join(done, "\n")+"\n"), 0o644)
}

// Cleanup снимает split-DNS; зовётся и при disconnect, и демоном, если
// openconnect умер, не дойдя до disconnect.
func Cleanup(p config.Paths) {
	b, err := os.ReadFile(p.DNSState())
	if err != nil {
		return
	}
	for _, d := range SafeDomains(strings.Fields(string(b))) {
		os.Remove(filepath.Join("/etc/resolver", d))
	}
	os.Remove(p.DNSState())
}
