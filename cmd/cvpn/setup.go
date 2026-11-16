package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/inoisy/cisco-vless/internal/client"
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

	link := askSecret("Ссылка vless://... или адрес подписки https://... (не отображается; Enter — оставить как есть)")
	if link != "" {
		if !strings.HasPrefix(link, "vless://") && !strings.HasPrefix(link, "https://") {
			return fmt.Errorf("нужна ссылка vless:// или адрес подписки https://")
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

	probe, err := c.Probe(s.CiscoHost)
	if err != nil {
		return err
	}
	if probe.Pin != s.CiscoPin {
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
