// Package netx — сеть хоста по ОС: физический шлюз, host-маршруты,
// резолв в обход корп-DNS, конфликтующие VPN-клиенты.
package netx

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

// Route — шлюз и интерфейс физической сети. vless/wg/awg default не трогают
// (половинки 0/1 и 128/1 или свои таблицы), так что это настоящий выход.
type Route struct {
	Gateway string
	Dev     string // имя (unix) или индекс (Windows)
}

// Run выполняет команду и возвращает stdout; в ошибке — stderr.
func Run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		return out.String(), fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, msg)
	}
	return out.String(), nil
}

// ResolveIP — IPv4 хоста через обычный DNS системы, не через split-DNS
// (/etc/resolver, NRPT): шлюз Cisco должен резолвиться и без туннеля.
func ResolveIP(host string) string {
	if ip := net.ParseIP(host); ip != nil {
		return host
	}
	r := &net.Resolver{PreferGo: true}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	ips, err := r.LookupIP(ctx, "ip4", host)
	if err != nil || len(ips) == 0 {
		return ""
	}
	return ips[0].String()
}

// ExternalIP — внешний IP, как его видит интернет.
func ExternalIP() string {
	out, err := httpGet("https://api.ipify.org", 5*time.Second)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
