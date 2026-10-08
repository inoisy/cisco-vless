package main

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/inoisy/cisco-vless/internal/config"
	"github.com/inoisy/cisco-vless/internal/daemon"
)

var exitNames = map[string]string{
	config.ExitVLESS: "VLESS",
	config.ExitWG:    "WireGuard",
	config.ExitAWG:   "AmneziaWG",
}

var (
	lastMu     sync.Mutex
	lastStatus *daemon.Status
	refreshCh  = make(chan struct{}, 1)
)

// poll — статус службы раз в 3 секунды или сразу после действия.
func (g *gui) poll() {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		g.update()
		select {
		case <-t.C:
		case <-refreshCh:
		}
	}
}

func (g *gui) refresh() {
	if g.tray == nil {
		return
	}
	select {
	case refreshCh <- struct{}{}:
	default:
	}
}

func (g *gui) update() {
	s, err := g.c.Status(false)
	var sp *daemon.Status
	if err == nil {
		s.Warnings = nil // в меню не показываем, а меняются они часто
		sp = &s
	}
	lastMu.Lock()
	same := reflect.DeepEqual(sp, lastStatus) && lastStatus != nil
	lastStatus = sp
	lastMu.Unlock()
	if same {
		return
	}
	application.InvokeAsync(func() {
		g.tray.SetIcon(trayIcon(iconState(sp)))
		g.tray.SetTooltip(tooltip(sp))
		g.buildMenu(sp)
	})
}

func tooltip(s *daemon.Status) string {
	if s == nil {
		return "cvpn: служба недоступна"
	}
	parts := []string{"cisco: выкл"}
	if s.Cisco {
		parts[0] = "cisco: вкл"
	}
	if s.Exit != "" {
		parts = append(parts, "выход: "+exitNames[s.Exit])
	} else {
		parts = append(parts, "выход: напрямую")
	}
	return "cvpn — " + strings.Join(parts, ", ")
}

// act — действие из меню в фоне; ошибку показываем окном.
func (g *gui) act(f func() (string, error)) {
	go func() {
		if _, err := f(); err != nil {
			application.InvokeAsync(func() {
				g.app.Dialog.Error().SetTitle("cvpn").SetMessage(err.Error()).Show()
			})
		}
		g.refresh()
	}()
}

func (g *gui) buildMenu(s *daemon.Status) {
	m := g.app.Menu.New()

	if s == nil {
		m.Add("Служба cvpn недоступна").SetEnabled(false)
		m.Add("Установи: cvpn service install").SetEnabled(false)
	} else {
		m.Add(tooltip(s)).SetEnabled(false)
		m.AddSeparator()

		if !s.Cisco && s.Exit == "" && len(s.Exits) > 0 {
			m.Add("Включить всё…").OnClick(func(*application.Context) { g.show("connect") })
		}
		if s.Cisco || s.Exit != "" {
			m.Add("Выключить всё").OnClick(func(*application.Context) { g.act(g.c.Off) })
		}
		m.AddSeparator()

		if s.Cisco {
			m.Add("Cisco: отключить").OnClick(func(*application.Context) { g.act(g.c.CiscoOff) })
		} else if s.Configured {
			m.Add("Cisco: подключить…").OnClick(func(*application.Context) { g.show("connect") })
		} else {
			m.Add("Cisco: не настроен").SetEnabled(false)
		}

		sub := m.AddSubmenu("Выход в интернет")
		sub.AddRadio("Напрямую", s.Exit == "").OnClick(func(*application.Context) {
			g.act(func() (string, error) { return g.c.ExitOff("") })
		})
		for _, e := range config.Exits {
			e := e
			item := sub.AddRadio(exitNames[e], s.Exit == e)
			if !contains(s.Exits, e) {
				item.SetLabel(exitNames[e] + " (не настроен)").SetEnabled(false)
				continue
			}
			item.OnClick(func(*application.Context) {
				g.act(func() (string, error) { return g.c.ExitOn(e) })
			})
		}
	}

	m.AddSeparator()
	m.Add("Открыть cvpn").OnClick(func(*application.Context) { g.show("status") })
	m.Add("Настройки…").OnClick(func(*application.Context) { g.show("settings") })
	m.Add("Логи").OnClick(func(*application.Context) { g.show("logs") })
	m.AddSeparator()
	m.Add(fmt.Sprintf("Закрыть значок (VPN %s)", map[bool]string{true: "останется", false: "выключен"}[s != nil && (s.Cisco || s.Exit != "")])).
		OnClick(func(*application.Context) { g.app.Quit() })

	g.tray.SetMenu(m)
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
