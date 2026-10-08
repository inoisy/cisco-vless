package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/inoisy/cisco-vless/internal/config"
	"github.com/inoisy/cisco-vless/internal/netx"
)

func extendPath() {}

// Имя адаптера wintun: по префиксу cvpn- PhysRoute отличает свои туннели.
func ciscoIfaceArgs() []string { return []string{"--interface=cvpn-cisco"} }

func linuxRules(bool) {}

// openconnect на Windows запускает скрипт через cscript, поэтому кладём
// крошечный JScript, который зовёт cvpn.exe _vpnc-script (окружение
// наследуется).
func ciscoScript(p config.Paths) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	js := fmt.Sprintf(`var sh = new ActiveXObject("WScript.Shell");
WScript.Quit(sh.Run('"%s" _vpnc-script', 0, true));
`, strings.ReplaceAll(exe, `\`, `\\`))
	path := filepath.Join(p.Etc, "cvpn-script.js")
	return path, os.WriteFile(path, []byte(js), 0o644)
}

func programFiles(rel ...string) []string {
	var out []string
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if d := os.Getenv(env); d != "" {
			for _, r := range rel {
				out = append(out, filepath.Join(d, r))
			}
		}
	}
	return out
}

func findBinary(configured, name string, cands ...string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	for _, p := range cands {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("не найден %s — установи его (install.ps1) или укажи путь в настройках", name)
}

func findOpenconnect(cfg *config.Config) (string, error) {
	return findBinary(cfg.Openconnect, "openconnect.exe",
		programFiles(`OpenConnect-GUI\openconnect.exe`, `OpenConnect\openconnect.exe`, `cvpn\openconnect\openconnect.exe`)...)
}

func findVPNCScript(cfg *config.Config) (string, error) {
	if cfg.VPNCScript != "" {
		return cfg.VPNCScript, nil
	}
	oc, err := findOpenconnect(cfg)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(oc)
	return findBinary("", "vpnc-script-win.js",
		filepath.Join(dir, "vpnc-script-win.js"), filepath.Join(dir, "vpnc-script.js"))
}

func findSingBox(cfg *config.Config, p config.Paths) (string, error) {
	return findBinary(cfg.SingBox, "sing-box.exe", filepath.Join(p.Lib, "sing-box.exe"))
}

// tunnelExit — wg и awg службой туннеля их Windows-клиентов.
type tunnelExit struct {
	kind string
	conf string
}

func (t tunnelExit) name() string { return strings.TrimSuffix(filepath.Base(t.conf), ".conf") }

func (t tunnelExit) exe() (string, error) {
	if t.kind == config.ExitAWG {
		return findBinary("", "amneziawg.exe (клиент AmneziaWG)", programFiles(`AmneziaWG\amneziawg.exe`)...)
	}
	return findBinary("", "wireguard.exe (клиент WireGuard)", programFiles(`WireGuard\wireguard.exe`)...)
}

func (t tunnelExit) up() error {
	exe, err := t.exe()
	if err != nil {
		return err
	}
	_, err = netx.Run(exe, "/installtunnelservice", t.conf)
	return err
}

func (t tunnelExit) down() error {
	exe, err := t.exe()
	if err != nil {
		return err
	}
	_, err = netx.Run(exe, "/uninstalltunnelservice", t.name())
	return err
}

// Адаптер туннеля называется как конфиг.
func (t tunnelExit) isUp() bool {
	out, err := netx.PS(fmt.Sprintf(`(Get-NetAdapter -Name '%s' -ErrorAction SilentlyContinue).Status`, t.name()))
	return err == nil && strings.TrimSpace(out) == "Up"
}
