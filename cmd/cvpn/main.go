// cvpn — Cisco AnyConnect (openconnect) одновременно с VLESS, WireGuard
// или AmneziaWG. macOS, Linux, Windows.
//
//	cisco — корп-сеть, split-tunnel: только маршруты от сервера Cisco и
//	        корп-домены. Совместим с любым выходом, порядок не важен.
//	vless — весь остальной трафик через VLESS (sing-box, tun).
//	wg    — весь остальной трафик через WireGuard.
//	awg   — весь остальной трафик через AmneziaWG.
//	vless, wg и awg взаимоисключающие: включение одного выключает другой.
//
// Работу делает служба (cvpn daemon); команды ниже — её клиент.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/inoisy/cisco-vless/internal/client"
	"github.com/inoisy/cisco-vless/internal/config"
	"github.com/inoisy/cisco-vless/internal/singbox"
	"github.com/inoisy/cisco-vless/internal/vpncscript"
)

var version = "dev"

const usage = `Usage:
  cvpn [status]
  cvpn on [vless|wg|awg] [otp]   выход (по умолчанию vless) + cisco
  cvpn off                       выключить всё
  cvpn cisco on [otp] | off
  cvpn vless|wg|awg on | off     выход; включение одного выключает другой
  cvpn boot vless|wg|awg|off     какой выход поднимать при старте системы
  cvpn setup                     настроить (сервер, логин, ссылка vless...)
  cvpn password                  сменить пароль Cisco
  cvpn log                       логи cisco, vless и службы
  cvpn service install|uninstall|start|stop|restart   (sudo / администратор)
  cvpn version`

func main() {
	args := os.Args[1:]
	cmd := "status"
	if len(args) > 0 {
		cmd = args[0]
	}
	if err := run(cmd, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	switch cmd {
	case "status":
		return status()
	case "on":
		exit, otp := "", ""
		for _, a := range args[1:] {
			if slices.Contains(config.Exits, a) {
				exit = a
			} else {
				otp = a
			}
		}
		return on(exit, otp)
	case "off":
		return say(client.New().Off())
	case "cisco":
		switch arg(1) {
		case "on":
			return say(client.New().CiscoOn(askOTP(arg(2))))
		case "off":
			return say(client.New().CiscoOff())
		}
	case config.ExitVLESS, config.ExitWG, config.ExitAWG:
		switch arg(1) {
		case "on":
			return say(client.New().ExitOn(cmd))
		case "off":
			return say(client.New().ExitOff(cmd))
		case "boot": // совместимость с bash-версией: cvpn vless boot on|off
			if arg(2) == "on" {
				return boot(cmd)
			}
			return boot("off")
		}
	case "boot":
		return boot(arg(1))
	case "setup":
		return setup()
	case "password":
		return password()
	case "log":
		return logs()
	case "service":
		return serviceCmd(arg(1))
	case "daemon":
		return runDaemon()
	case "_vpnc-script":
		os.Exit(vpncScript())
	case "_singbox-config":
		return singboxConfig(args[1:])
	case "version", "--version":
		fmt.Println("cvpn", version)
		return nil
	case "help", "-h", "--help":
		fmt.Println(usage)
		return nil
	}
	fmt.Fprintln(os.Stderr, usage)
	os.Exit(2)
	return nil
}

// singboxConfig печатает singbox.json без службы: для bash-версии и отладки.
//
//	cvpn _singbox-config <ссылки | https://подписка> [geoip-ru.srs] [адрес-в-обход ...]
func singboxConfig(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("Usage: cvpn _singbox-config <ссылки | https://подписка> [geoip-ru.srs] [адрес-в-обход ...]")
	}
	links, warns, err := singbox.Resolve(args[0], func(u string) string {
		return filepath.Join(os.TempDir(), "cvpn-subscription-"+singbox.SubHost(u)+".txt")
	})
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, w)
	}
	if err != nil {
		return err
	}
	geoip := ""
	if len(args) > 1 {
		geoip = args[1]
	}
	var exclude []string
	if len(args) > 2 {
		exclude = args[2:]
	}
	// Настроек службы здесь нет (config.json читает только root) — правила те
	// же, что были зашиты.
	r := config.LegacyRouting()
	b, err := singbox.Config(links, singbox.Options{
		Exclude:      exclude,
		GOOS:         runtime.GOOS,
		RUDirect:     r.RUDirect,
		GeoIP:        geoip,
		ProxyDomains: r.ProxyDomains,
	})
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func say(msg string, err error) error {
	if msg != "" {
		fmt.Println(msg)
	}
	return err
}

func on(exit, otp string) error {
	c := client.New()
	if exit == "" {
		s, err := c.Status(false)
		if err != nil {
			return err
		}
		exit = config.ExitVLESS
		if !slices.Contains(s.Exits, exit) && len(s.Exits) > 0 {
			exit = s.Exits[0]
		}
		printWarnings(s.Warnings)
	}
	if err := say(c.On(exit, askOTP(otp))); err != nil {
		return err
	}
	fmt.Println()
	return status()
}

func status() error {
	s, err := client.New().Status(true)
	if err != nil {
		return err
	}
	onoff := func(b bool) string {
		if b {
			return "ON"
		}
		return "off"
	}
	fmt.Printf("cisco : %s\n", onoff(s.Cisco))
	if s.CiscoDropped {
		fmt.Println("        отключилась сама — нужен новый код: cvpn cisco on <OTP>")
	}
	for _, e := range s.Exits {
		fmt.Printf("%-6s: %s\n", e, onoff(s.Exit == e))
	}
	ip := s.IP
	if ip == "" {
		ip = "нет ответа"
	}
	fmt.Printf("IP    : %s\n", ip)
	if s.Cisco && s.CorpHost != "" {
		corp := s.CorpIP
		if corp == "" {
			corp = "НЕ РЕЗОЛВИТСЯ"
		}
		fmt.Printf("корп  : %s -> %s\n", s.CorpHost, corp)
	}
	if !s.Configured {
		fmt.Println("cisco не настроен — cvpn setup")
	}
	printWarnings(s.Warnings)
	return nil
}

func printWarnings(w []string) {
	for _, m := range w {
		fmt.Println("ВНИМАНИЕ:", m)
	}
}

func boot(exit string) error {
	if exit == "off" {
		exit = ""
	}
	if exit != "" && !slices.Contains(config.Exits, exit) {
		return fmt.Errorf("Usage: cvpn boot vless|wg|awg|off")
	}
	c := client.New()
	s, err := c.Settings()
	if err != nil {
		return err
	}
	s.BootExit = exit
	if _, err := c.SetSettings(s); err != nil {
		return err
	}
	if exit == "" {
		fmt.Println("при старте системы выход не поднимается")
	} else {
		fmt.Printf("%s: будет включаться при старте системы\n", exit)
	}
	return nil
}

func logs() error {
	l, err := client.New().Logs()
	if err != nil {
		return err
	}
	fmt.Printf("== cisco\n%s\n== vless\n%s\n== служба\n%s\n", l.Cisco, l.VLESS, l.Daemon)
	return nil
}

// vpncScript — вызов от openconnect. На Windows вывод скрипта никто не
// читает — пишем в лог.
func vpncScript() int {
	p := config.Default()
	if isWindows() {
		if f, err := os.OpenFile(p.Log+string(os.PathSeparator)+"cvpn-script.log",
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			os.Stdout, os.Stderr = f, f
		}
	}
	cfg, err := config.Load(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cvpn:", err)
		return 1
	}
	return vpncscript.Run(cfg, p)
}

func isWindows() bool { return runtime.GOOS == "windows" }
