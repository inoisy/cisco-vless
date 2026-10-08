package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// loadLegacy читает конфиг bash-версии (/usr/local/etc/cvpn/config,
// строки KEY=значение в кавычках printf %q), чтобы обновление не требовало
// заново вводить настройки. Ссылки vless там нет — остаётся singbox.json.
func loadLegacy(p Paths) (*Config, bool) {
	f, err := os.Open(filepath.Join(p.Etc, "config"))
	if err != nil {
		return nil, false
	}
	defer f.Close()
	v := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, val, ok := strings.Cut(line, "=")
		if ok {
			v[k] = unquoteShell(val)
		}
	}
	return &Config{
		CiscoHost:     v["CISCO_HOST"],
		CiscoIP:       v["CISCO_IP"],
		CiscoUser:     v["CISCO_USER"],
		CiscoGroup:    v["CISCO_GROUP"],
		CiscoPin:      v["CISCO_PIN"],
		CorpDomains:   strings.Fields(v["CORP_DOMAINS"]),
		CorpCheckHost: v["CORP_CHECK_HOST"],
		VPNCScript:    v["VPNC_SCRIPT"],
	}, true
}

// unquoteShell — обратное к bash printf %q для простых случаев: '...',
// $'...' и экранирование обратным слешем.
func unquoteShell(s string) string {
	switch {
	case s == "''":
		return ""
	case strings.HasPrefix(s, "$'") && strings.HasSuffix(s, "'"):
		s = s[2 : len(s)-1]
		r := strings.NewReplacer(`\\`, `\`, `\'`, `'`, `\n`, "\n", `\t`, "\t")
		return r.Replace(s)
	case len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'':
		return s[1 : len(s)-1]
	}
	var b strings.Builder
	esc := false
	for _, r := range s {
		if !esc && r == '\\' {
			esc = true
			continue
		}
		esc = false
		b.WriteRune(r)
	}
	return b.String()
}
