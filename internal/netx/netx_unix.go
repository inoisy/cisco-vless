//go:build !windows

package netx

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func hideWindow(*exec.Cmd) {}

var conflictRe = regexp.MustCompile(`(?i)hiddify|v2box|v2rayn|nekoray|clash|amneziavpn|\bhapp\b|\bxray\b`)

// Conflicts — клиенты, которые ломают схему: сами перехватывают маршруты и
// DNS. Демон Cisco Secure Client (vpnagentd) работает всегда, пока клиент
// установлен, — мешает только активное подключение, его и спрашиваем.
func Conflicts() []string {
	var w []string
	for _, c := range []string{"/opt/cisco/secureclient/bin/vpn", "/opt/cisco/anyconnect/bin/vpn"} {
		if _, err := os.Stat(c); err != nil {
			continue
		}
		if out, _ := Run(c, "state"); strings.Contains(out, "state: Connected") {
			w = append(w, "подключён Cisco Secure Client — отключись в нём, он конфликтует с cvpn")
			break
		}
	}
	if out, err := Run("ps", "-axo", "comm="); err == nil && conflictRe.MatchString(out) {
		w = append(w, "запущен другой VPN-клиент (Hiddify/V2Box/Clash/Amnezia/Happ...) — если он подключён, отключи")
	}
	return w
}
