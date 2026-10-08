// Package vpncscript — обёртка над штатным vpnc-script для openconnect
// (cvpn _vpnc-script).
//
// Штатный скрипт, получив от сервера DNS, перехватывает DNS всей системы
// (macOS — networksetup/OverridePrimary, Windows — DNS на адаптере с низкой
// метрикой): публичные домены уходят в корп-DNS и не резолвятся, а tun cisco
// спорит с vless/wg за primary-интерфейс. Поэтому DNS у него отбираем и
// делаем split-DNS: в корп-DNS идут только корп-домены (из конфига плюс
// присланные сервером). Маршруты штатный скрипт настраивает как обычно.
package vpncscript

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/inoisy/cisco-vless/internal/config"
)

var dnsVars = []string{"INTERNAL_IP4_DNS", "INTERNAL_IP6_DNS", "CISCO_DEF_DOMAIN", "CISCO_SPLIT_DNS"}

// Run вызывается openconnect'ом; reason, TUNDEV и прочее — в окружении.
func Run(cfg *config.Config, p config.Paths) int {
	corpDNS := strings.Fields(os.Getenv("INTERNAL_IP4_DNS"))
	domains := SafeDomains(append(append([]string{}, cfg.CorpDomains...),
		strings.FieldsFunc(os.Getenv("CISCO_DEF_DOMAIN")+","+os.Getenv("CISCO_SPLIT_DNS"), isSep)...))

	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !contains(dnsVars, k) {
			env = append(env, kv)
		}
	}

	rc := 0
	if err := runStock(cfg, env); err != nil {
		fmt.Fprintln(os.Stderr, "cvpn: vpnc-script:", err)
		rc = 1
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		}
	}

	reason := os.Getenv("reason")
	switch reason {
	case "connect":
		if len(corpDNS) > 0 && len(domains) > 0 {
			if err := setSplitDNS(p, os.Getenv("TUNDEV"), corpDNS, domains); err != nil {
				fmt.Fprintln(os.Stderr, "cvpn: split-DNS:", err)
			}
		}
	case "disconnect":
		Cleanup(p)
	}
	return rc
}

// EnvStock — путь к штатному vpnc-script, найденный службой.
const EnvStock = "CVPN_VPNC_SCRIPT"

func runStock(cfg *config.Config, env []string) error {
	script := os.Getenv(EnvStock)
	if script == "" {
		script = cfg.VPNCScript
	}
	if script == "" {
		return fmt.Errorf("не задан vpnc_script в конфиге")
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cscript.exe", "//nologo", "/e:JScript", script)
	} else {
		cmd = exec.Command(script)
	}
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

var domainRe = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*$`)

// SafeDomains: домен идёт в имя файла под root и в команды — пропускаем
// всё, кроме букв, цифр, точек и дефисов. Без дублей.
func SafeDomains(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(d))
		if !domainRe.MatchString(d) || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func isSep(r rune) bool { return r == ',' || r == ' ' || r == '\t' }

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
