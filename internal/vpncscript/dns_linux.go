package vpncscript

import (
	"fmt"
	"os/exec"

	"github.com/inoisy/cisco-vless/internal/config"
)

// Linux: systemd-resolved, DNS и routing-домены (~домен) на tun cisco.
// Настройки живут на интерфейсе и уходят вместе с ним.
func setSplitDNS(_ config.Paths, tundev string, dns, domains []string) error {
	if _, err := exec.LookPath("resolvectl"); err != nil {
		return fmt.Errorf("нет systemd-resolved, split-DNS для %v не настроен", domains)
	}
	if err := exec.Command("resolvectl", append([]string{"dns", tundev}, dns...)...).Run(); err != nil {
		return err
	}
	args := []string{"domain", tundev}
	for _, d := range domains {
		args = append(args, "~"+d)
	}
	if err := exec.Command("resolvectl", args...).Run(); err != nil {
		return err
	}
	exec.Command("resolvectl", "default-route", tundev, "false").Run()
	return nil
}

func Cleanup(config.Paths) {}
