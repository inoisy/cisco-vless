package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/inoisy/cisco-vless/internal/client"
	"github.com/inoisy/cisco-vless/internal/singbox"
)

// setup — те же вопросы, что задавал install.sh. Текущие значения — по
// умолчанию, Enter их оставляет.
func setup() error {
	c := client.New()
	s, err := c.Settings()
	if err != nil {
		return err
	}

	s.CiscoHost = ask("Сервер Cisco (например vpn.example.com)", s.CiscoHost)
	if s.CiscoHost == "" {
		return fmt.Errorf("сервер Cisco обязателен")
	}
	s.CiscoUser = ask("Логин Cisco", s.CiscoUser)
	if s.CiscoUser == "" {
		return fmt.Errorf("логин обязателен")
	}
	s.CiscoGroup = ask("Группа Cisco (authgroup), Enter — без группы", s.CiscoGroup)
	s.CorpDomains = strings.Fields(ask("Корп-домены для DNS через пробел (example.ru corp.local)", strings.Join(s.CorpDomains, " ")))
	s.CorpCheckHost = ask("Корп-хост для проверки в cvpn status", s.CorpCheckHost)

	if subs, links := singbox.Sources(s.VLESSLink); len(subs)+len(links) > 0 {
		fmt.Printf("vless сейчас: подписок %d, ссылок %d\n", len(subs), len(links))
	}
	link := askSecret("Ссылки vless:// / hysteria2:// и адреса подписок https:// через пробел (не отображается;\n" +
		"  Enter — оставить как есть, + в начале — добавить к текущим)")
	if link != "" {
		if rest, ok := strings.CutPrefix(link, "+"); ok {
			link = s.VLESSLink + " " + rest
		}
		link = strings.Join(strings.Fields(link), " ")
		if err := singbox.CheckSources(link); err != nil {
			return err
		}
		s.VLESSLink = link
	}
	for _, t := range []struct {
		name string
		conf **string
		has  bool
	}{{"WireGuard", &s.WGConf, s.HasWG}, {"AmneziaWG", &s.AWGConf, s.HasAWG}} {
		hint := "Enter — без него"
		if t.has {
			hint = "Enter — оставить текущий, - — удалить"
		}
		path := ask(fmt.Sprintf("Путь к конфигу %s (%s)", t.name, hint), "")
		switch path {
		case "":
		case "-":
			empty := ""
			*t.conf = &empty
		default:
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			body := string(b)
			*t.conf = &body
		}
	}

	// Подписку качаем здесь, от пользователя: у него может быть путь до
	// сервера подписки (через текущий VPN), которого нет у службы до старта
	// туннеля. Служба сохранит копию и поднимется на ней.
	s.VLESSSubs = prefetchSubs(s.VLESSLink)

	probe, err := c.Probe(s.CiscoHost)
	switch {
	case err != nil && s.CiscoPin != "":
		// Сервер сейчас недоступен (DNS у другого VPN, нет сети) — сертификат
		// уже доверен, остальные настройки сохраняем.
		fmt.Printf("ВНИМАНИЕ: сертификат не проверен (%v) — оставляю сохранённый\n", err)
	case err != nil:
		return err
	case probe.Pin != s.CiscoPin:
		fmt.Printf("\nСертификат %s (%s): %s\n", s.CiscoHost, probe.IP, probe.Pin)
		if s.CiscoPin != "" {
			fmt.Printf("ВНИМАНИЕ: сохранён другой: %s\n", s.CiscoPin)
		}
		if !yesNo("Доверять этому сертификату?") {
			return fmt.Errorf("отменено")
		}
		s.CiscoPin = probe.Pin
	}

	saved, err := c.SetSettings(s)
	if err != nil {
		return err
	}
	fmt.Println("настройки сохранены")

	if !c.HasPassword(saved) || yesNo("Сменить пароль Cisco?") {
		return setPassword(c, saved.CiscoUser)
	}
	return nil
}

// prefetchSubs качает подписки настройки vless от пользователя; что не
// скачалось — служба попробует сама при vless on или возьмёт копию.
func prefetchSubs(spec string) map[string]string {
	got, warns := singbox.FetchSubscriptions(spec)
	for _, w := range warns {
		fmt.Println("ВНИМАНИЕ:", w)
	}
	for u, links := range got {
		fmt.Printf("подписка %s: %d ссылок\n", singbox.SubHost(u), len(strings.Fields(links)))
	}
	return got
}

func password() error {
	c := client.New()
	s, err := c.Settings()
	if err != nil {
		return err
	}
	if s.CiscoUser == "" {
		return fmt.Errorf("сначала cvpn setup")
	}
	return setPassword(c, s.CiscoUser)
}

func setPassword(c *client.Client, user string) error {
	pw := askSecret("Пароль Cisco для " + user)
	if pw == "" {
		return fmt.Errorf("пустой пароль, не сохраняю")
	}
	if err := c.SetPassword(user, pw); err != nil {
		return err
	}
	if client.KeychainUsed() {
		fmt.Println("сохранён в Keychain")
	} else {
		fmt.Println("сохранён (читает только root/SYSTEM)")
	}
	return nil
}
