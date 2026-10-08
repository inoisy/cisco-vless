package daemon

import (
	"path/filepath"
	"strings"

	"github.com/inoisy/cisco-vless/internal/netx"
)

// Имя tun cisco закрепляем. Иначе openconnect берёт первый свободный tunN —
// имя зависит от порядка запуска с sing-box, а на имена завязан фаервол.
func ciscoIfaceArgs() []string { return []string{"--interface=tun1"} }

// linuxRules — правила в обход tun sing-box.
//
//	55 — трафик с адреса физического интерфейса (ответы на входящие, SSH)
//	     идёт через него, а не в tun;
//	60 — всё, у чего есть точный маршрут в main (tun cisco, шлюз cisco,
//	     локальная сеть), идёт по нему; в tun — только то, что шло бы в default.
func linuxRules(up bool) {
	netx.Run("ip", "rule", "del", "priority", "55")
	netx.Run("ip", "rule", "del", "priority", "60")
	if !up {
		return
	}
	if r, err := netx.PhysRoute(); err == nil {
		out, _ := netx.Run("ip", "-4", "-o", "addr", "show", "dev", r.Dev)
		if f := strings.Fields(out); len(f) >= 4 {
			src, _, _ := strings.Cut(f[3], "/")
			netx.Run("ip", "rule", "add", "priority", "55", "from", src, "lookup", "main")
		}
	}
	netx.Run("ip", "rule", "add", "priority", "60", "lookup", "main", "suppress_prefixlength", "0")
}

func (t tunnelExit) isUp() bool {
	name := strings.TrimSuffix(filepath.Base(t.conf), ".conf")
	_, err := netx.Run("ip", "link", "show", name)
	return err == nil
}
