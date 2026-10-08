package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"runtime"

	"github.com/kardianos/service"

	"github.com/inoisy/cisco-vless/internal/config"
	"github.com/inoisy/cisco-vless/internal/daemon"
	"github.com/inoisy/cisco-vless/internal/ipc"
)

type program struct {
	m *daemon.Manager
}

func (p *program) Start(service.Service) error {
	paths := config.Default()
	m, err := daemon.NewManager(paths)
	if err != nil {
		return err
	}
	p.m = m
	l, err := ipc.Listen(paths.Socket, m.Owner())
	if err != nil {
		return fmt.Errorf("сокет %s: %w", paths.Socket, err)
	}
	go m.Boot()
	go func() {
		if err := m.Serve(l); err != nil {
			log.Printf("serve: %v", err)
		}
	}()
	log.Printf("cvpn %s: служба запущена", version)
	return nil
}

func (p *program) Stop(service.Service) error {
	if p.m != nil {
		p.m.Shutdown()
	}
	log.Print("служба остановлена")
	return nil
}

func newService() (service.Service, error) {
	return service.New(&program{}, &service.Config{
		Name:        "cvpn",
		DisplayName: "cvpn — Cisco + VLESS/WireGuard/AmneziaWG",
		Description: "Cisco AnyConnect вместе с VLESS, WireGuard или AmneziaWG",
		Arguments:   []string{"daemon"},
		// systemd: выход при загрузке поднимаем, когда сеть уже есть.
		Dependencies: []string{"Wants=network-online.target", "After=network-online.target"},
		Option: service.KeyValue{
			"KeepAlive":   true,
			"RunAtLoad":   true,
			"OnFailure":   "restart",
			"StartType":   "automatic",
			"Restart":     "on-failure",
			"LogOutput":   false,
			"UserService": false,
		},
	})
}

func runDaemon() error {
	paths := config.Default()
	os.MkdirAll(paths.Log, 0o755)
	if f, err := os.OpenFile(paths.DaemonLog(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(f, os.Stderr))
	}
	s, err := newService()
	if err != nil {
		return err
	}
	return s.Run()
}

func serviceCmd(action string) error {
	s, err := newService()
	if err != nil {
		return err
	}
	switch action {
	case "install":
		paths := config.Default()
		if err := secureDirs(paths); err != nil {
			return err
		}
		if err := setOwner(paths); err != nil {
			return err
		}
		s.Uninstall() // переустановка: новый путь бинарника, новые опции
		if err := s.Install(); err != nil {
			return err
		}
		if err := s.Start(); err != nil {
			return err
		}
		fmt.Println("служба cvpn установлена и запущена")
		return nil
	case "uninstall":
		s.Stop()
		if err := s.Uninstall(); err != nil {
			return err
		}
		fmt.Println("служба cvpn удалена")
		return nil
	case "start", "stop", "restart":
		return service.Control(s, action)
	case "status":
		st, err := s.Status()
		if err != nil {
			return err
		}
		fmt.Println(map[service.Status]string{
			service.StatusRunning: "работает", service.StatusStopped: "остановлена",
		}[st])
		return nil
	}
	return fmt.Errorf("Usage: cvpn service install|uninstall|start|stop|restart|status")
}

// setOwner — кому служба отвечает по сокету без sudo: тот, кто ставит
// (SUDO_USER под sudo), или явно CVPN_OWNER.
func setOwner(p config.Paths) error {
	owner := os.Getenv("CVPN_OWNER")
	if owner == "" {
		owner = os.Getenv("SUDO_USER")
	}
	if owner == "" && runtime.GOOS == "windows" {
		if u, err := user.Current(); err == nil {
			owner = u.Username
		}
	}
	if owner == "" || owner == "root" {
		fmt.Println("ВНИМАНИЕ: владелец не определён — без sudo командами cvpn пользоваться не выйдет. Задай CVPN_OWNER=<логин>")
		return nil
	}
	cfg, err := config.Load(p)
	if err != nil {
		return err
	}
	cfg.Owner = owner
	if err := config.Save(p, cfg); err != nil {
		return err
	}
	fmt.Println("владелец:", owner)
	return nil
}
