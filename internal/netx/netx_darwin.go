package netx

import (
	"strings"
)

func PhysRoute() (Route, error) {
	out, err := Run("netstat", "-rn", "-f", "inet")
	if err != nil {
		return Route{}, err
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 4 && f[0] == "default" && !strings.HasPrefix(f[len(f)-1], "utun") {
			return Route{Gateway: f[1], Dev: f[len(f)-1]}, nil
		}
	}
	return Route{}, errNoDefault
}

func AddHostRoute(ip string, r Route) error {
	if _, err := Run("route", "-n", "add", "-host", ip, r.Gateway); err != nil {
		_, err = Run("route", "-n", "change", "-host", ip, r.Gateway)
		return err
	}
	return nil
}

func DelHostRoute(ip string) { Run("route", "-n", "delete", "-host", ip) }

// LookupSystem — резолв как у приложений (с /etc/resolver).
func LookupSystem(host string) string {
	out, _ := Run("dscacheutil", "-q", "host", "-a", "name", host)
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(line, "ip_address: "); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// PublicDNS — только Linux.
func PublicDNS() []string { return nil }
