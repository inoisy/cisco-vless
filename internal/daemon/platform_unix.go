//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/inoisy/cisco-vless/internal/config"
	"github.com/inoisy/cisco-vless/internal/netx"
)

// launchd и systemd дают службе урезанный PATH — добавляем Homebrew и
// /usr/local, где лежат openconnect, sing-box, wg-quick.
func extendPath() {
	extra := "/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/local/sbin:/usr/bin:/bin:/usr/sbin:/sbin"
	os.Setenv("PATH", extra+":"+os.Getenv("PATH"))
}

// openconnect зовёт скрипт через /bin/sh -c, аргументы можно.
func ciscoScript(config.Paths) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return "'" + strings.ReplaceAll(exe, "'", `'\''`) + "' _vpnc-script", nil
}

func findBinary(configured, name string, extra ...string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, p := range extra {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("не найден %s — установи его (install.sh) или укажи путь в настройках", name)
}

func findVPNCScript(cfg *config.Config) (string, error) {
	return findBinary(cfg.VPNCScript, "vpnc-script",
		"/opt/homebrew/etc/vpnc/vpnc-script", "/usr/local/etc/vpnc/vpnc-script",
		"/usr/share/vpnc-scripts/vpnc-script", "/etc/vpnc/vpnc-script")
}

func findSingBox(cfg *config.Config, _ config.Paths) (string, error) {
	return findBinary(cfg.SingBox, "sing-box")
}

func findOpenconnect(cfg *config.Config) (string, error) {
	return findBinary(cfg.Openconnect, "openconnect")
}

// tunnelExit — wg и awg через wg-quick / awg-quick.
type tunnelExit struct {
	kind string // wg | awg
	conf string
}

func (t tunnelExit) tool() string {
	if t.kind == config.ExitAWG {
		return "awg-quick"
	}
	return "wg-quick"
}

func (t tunnelExit) up() error {
	if _, err := exec.LookPath(t.tool()); err != nil {
		return fmt.Errorf("не найден %s — установи %s", t.tool(), map[string]string{
			"wg-quick": "wireguard-tools", "awg-quick": "amneziawg-tools",
		}[t.tool()])
	}
	_, err := netx.Run(t.tool(), "up", t.conf)
	return err
}

func (t tunnelExit) down() error {
	_, err := netx.Run(t.tool(), "down", t.conf)
	return err
}
