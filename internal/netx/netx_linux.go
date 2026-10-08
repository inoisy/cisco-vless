package netx

import (
	"net"
	"os"
	"sort"
	"strings"
)

func PhysRoute() (Route, error) {
	out, err := Run("ip", "-4", "route", "show", "table", "main", "default")
	if err != nil {
		return Route{}, err
	}
	line, _, _ := strings.Cut(out, "\n")
	f := strings.Fields(line)
	var r Route
	for i := 0; i+1 < len(f); i++ {
		switch f[i] {
		case "via":
			r.Gateway = f[i+1]
		case "dev":
			r.Dev = f[i+1]
		}
	}
	if r.Gateway == "" {
		return r, errNoDefault
	}
	return r, nil
}

func AddHostRoute(ip string, r Route) error {
	_, err := Run("ip", "route", "replace", ip, "via", r.Gateway, "dev", r.Dev)
	return err
}

func DelHostRoute(ip string) { Run("ip", "route", "del", ip) }

func LookupSystem(host string) string {
	out, _ := Run("getent", "hosts", host)
	if f := strings.Fields(out); len(f) > 0 {
		return f[0]
	}
	return ""
}

// PublicDNS — публичные DNS-серверы системы: их пускаем мимо tun — многие
// провайдерские резолверы не отвечают на запросы с чужого IP (выхода VLESS).
func PublicDNS() []string {
	var cand []string
	if out, err := Run("resolvectl", "dns"); err == nil {
		cand = append(cand, strings.Fields(out)...)
	}
	if b, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if f := strings.Fields(line); len(f) == 2 && f[0] == "nameserver" {
				cand = append(cand, f[1])
			}
		}
	}
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	_, bench, _ := net.ParseCIDR("198.18.0.0/15")
	seen := map[string]bool{}
	var res []string
	for _, c := range cand {
		ip := net.ParseIP(c).To4()
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || cgnat.Contains(ip) || bench.Contains(ip) || seen[c] {
			continue
		}
		seen[c] = true
		res = append(res, c)
	}
	sort.Strings(res)
	return res
}
