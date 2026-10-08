package daemon

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/inoisy/cisco-vless/internal/config"
)

func ciscoIfaceArgs() []string { return nil }

func linuxRules(bool) {}

// wg-quick на macOS: имя utunN — в /var/run/<wireguard|amneziawg>/<имя>.name,
// рядом сокет userspace-реализации, пока туннель жив.
func (t tunnelExit) isUp() bool {
	dir := "/var/run/wireguard"
	if t.kind == config.ExitAWG {
		dir = "/var/run/amneziawg"
	}
	name := strings.TrimSuffix(filepath.Base(t.conf), ".conf")
	b, err := os.ReadFile(filepath.Join(dir, name+".name"))
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, strings.TrimSpace(string(b))+".sock"))
	return err == nil
}
