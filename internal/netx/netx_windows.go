package netx

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// PS выполняет команду PowerShell.
func PS(script string) (string, error) {
	return Run("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
}

// PhysRoute: default с наименьшей метрикой, кроме своих адаптеров (cvpn-*)
// и туннелей, у которых нет шлюза (sing-box, wintun openconnect).
func PhysRoute() (Route, error) {
	out, err := PS(`Get-NetRoute -AddressFamily IPv4 -DestinationPrefix 0.0.0.0/0 -ErrorAction SilentlyContinue |
		Where-Object { $_.NextHop -ne '0.0.0.0' -and $_.InterfaceAlias -notlike 'cvpn-*' } |
		Sort-Object { $_.RouteMetric + $_.InterfaceMetric } |
		Select-Object -First 1 NextHop, InterfaceIndex | ConvertTo-Json -Compress`)
	if err != nil {
		return Route{}, err
	}
	var r struct {
		NextHop        string
		InterfaceIndex int
	}
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &r) != nil || r.NextHop == "" {
		return Route{}, errNoDefault
	}
	return Route{Gateway: r.NextHop, Dev: fmt.Sprint(r.InterfaceIndex)}, nil
}

func AddHostRoute(ip string, r Route) error {
	DelHostRoute(ip)
	_, err := Run("route.exe", "add", ip, "mask", "255.255.255.255", r.Gateway, "metric", "1", "if", r.Dev)
	return err
}

func DelHostRoute(ip string) { Run("route.exe", "delete", ip) }

// LookupSystem — через GetAddrInfo, с учётом NRPT.
func LookupSystem(host string) string {
	ips, err := net.LookupIP(host)
	if err != nil {
		return ""
	}
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String()
		}
	}
	return ""
}

func PublicDNS() []string { return nil }

var conflictProcs = []string{"hiddify", "v2box", "v2rayn", "nekoray", "clash", "amneziavpn"}

func Conflicts() []string {
	var w []string
	for _, dir := range []string{`Cisco\Cisco Secure Client`, `Cisco\Cisco AnyConnect Secure Mobility Client`} {
		cli := filepath.Join(os.Getenv("ProgramFiles(x86)"), dir, "vpncli.exe")
		if _, err := os.Stat(cli); err != nil {
			continue
		}
		if out, _ := Run(cli, "state"); strings.Contains(out, "state: Connected") {
			w = append(w, "подключён Cisco Secure Client — отключись в нём, он конфликтует с cvpn")
			break
		}
	}
	out, _ := Run("tasklist.exe", "/fo", "csv", "/nh")
	low := strings.ToLower(out)
	for _, p := range conflictProcs {
		if strings.Contains(low, p) {
			w = append(w, "запущен другой VPN-клиент (Hiddify/V2Box/Clash/Amnezia...) — если он подключён, отключи")
			break
		}
	}
	return w
}
