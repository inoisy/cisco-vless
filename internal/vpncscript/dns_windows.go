package vpncscript

import (
	"fmt"
	"strings"

	"github.com/inoisy/cisco-vless/internal/config"
	"github.com/inoisy/cisco-vless/internal/netx"
)

// Метка своих правил NRPT: по ней снимаем только их.
const nrptComment = "cvpn"

// Windows: NRPT — правила «домен → DNS-сервер», аналог /etc/resolver.
// ".corp.ru" ловит поддомены, "corp.ru" — сам домен.
func setSplitDNS(_ config.Paths, _ string, dns, domains []string) error {
	var ns []string
	for _, s := range dns {
		ns = append(ns, "'"+s+"'")
	}
	var b strings.Builder
	b.WriteString(removeRules)
	for _, d := range domains {
		fmt.Fprintf(&b, "Add-DnsClientNrptRule -Namespace '.%s','%s' -NameServers %s -Comment '%s';\n",
			d, d, strings.Join(ns, ","), nrptComment)
	}
	b.WriteString("Clear-DnsClientCache;")
	_, err := netx.PS(b.String())
	return err
}

var removeRules = fmt.Sprintf("Get-DnsClientNrptRule | Where-Object { $_.Comment -eq '%s' } | ForEach-Object { Remove-DnsClientNrptRule -Name $_.Name -Force };\n", nrptComment)

func Cleanup(config.Paths) {
	netx.PS(removeRules + "Clear-DnsClientCache;")
}
