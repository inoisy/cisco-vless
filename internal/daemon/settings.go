package daemon

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/inoisy/cisco-vless/internal/config"
	"github.com/inoisy/cisco-vless/internal/netx"
	"github.com/inoisy/cisco-vless/internal/singbox"
	"github.com/inoisy/cisco-vless/internal/vpncscript"
)

func (m *Manager) Settings() Settings {
	m.mu.Lock()
	c := *m.cfg
	m.mu.Unlock()
	_, wgErr := os.Stat(m.paths.TunnelConf(config.ExitWG))
	_, awgErr := os.Stat(m.paths.TunnelConf(config.ExitAWG))
	st, pwErr := os.Stat(m.paths.PasswordFile())
	return Settings{
		CiscoHost:     c.CiscoHost,
		CiscoUser:     c.CiscoUser,
		CiscoGroup:    c.CiscoGroup,
		CiscoPin:      c.CiscoPin,
		CorpDomains:   c.CorpDomains,
		CorpCheckHost: c.CorpCheckHost,
		VLESSLink:     c.VLESSLink,
		BootExit:      c.BootExit,
		Openconnect:   c.Openconnect,
		VPNCScript:    c.VPNCScript,
		SingBox:       c.SingBox,
		HasWG:         wgErr == nil,
		HasAWG:        awgErr == nil,
		HasPassword:   pwErr == nil && st.Size() > 0,
	}
}

// SetSettings проверяет и сохраняет настройки. Включённое не перезапускает:
// новое применится при следующем on.
func (m *Manager) SetSettings(s Settings) error {
	m.op.Lock()
	defer m.op.Unlock()

	s.CiscoHost = strings.TrimSpace(s.CiscoHost)
	s.VLESSLink = strings.TrimSpace(s.VLESSLink)
	if s.CiscoHost != "" && strings.ContainsAny(s.CiscoHost, " /:") {
		return fmt.Errorf("сервер Cisco — только имя хоста, без https:// и порта")
	}
	if s.CiscoPin != "" && !strings.HasPrefix(s.CiscoPin, "pin-sha256:") {
		return fmt.Errorf("сертификат — в виде pin-sha256:...")
	}
	if s.VLESSLink != "" {
		if _, err := singbox.Config(s.VLESSLink, nil, "linux", ""); err != nil {
			return fmt.Errorf("vless: %w", err)
		}
	}
	switch s.BootExit {
	case "", config.ExitVLESS, config.ExitWG, config.ExitAWG:
	default:
		return fmt.Errorf("неизвестный выход %q", s.BootExit)
	}

	m.mu.Lock()
	c := *m.cfg
	m.mu.Unlock()
	if s.CiscoHost != c.CiscoHost || c.CiscoIP == "" {
		c.CiscoIP = netx.ResolveIP(s.CiscoHost)
	}
	c.CiscoHost = s.CiscoHost
	c.CiscoUser = strings.TrimSpace(s.CiscoUser)
	c.CiscoGroup = strings.TrimSpace(s.CiscoGroup)
	c.CiscoPin = strings.TrimSpace(s.CiscoPin)
	c.CorpDomains = vpncscript.SafeDomains(s.CorpDomains)
	c.CorpCheckHost = strings.TrimSpace(s.CorpCheckHost)
	c.VLESSLink = s.VLESSLink
	c.BootExit = s.BootExit
	c.Openconnect = strings.TrimSpace(s.Openconnect)
	c.VPNCScript = strings.TrimSpace(s.VPNCScript)
	c.SingBox = strings.TrimSpace(s.SingBox)

	for kind, conf := range map[string]*string{config.ExitWG: s.WGConf, config.ExitAWG: s.AWGConf} {
		if conf == nil {
			continue
		}
		path := m.paths.TunnelConf(kind)
		if strings.TrimSpace(*conf) == "" {
			os.Remove(path)
			continue
		}
		if !strings.Contains(*conf, "[Interface]") || !strings.Contains(*conf, "[Peer]") {
			return fmt.Errorf("%s: в конфиге нет секций [Interface] и [Peer]", kind)
		}
		if err := config.WriteFileAtomic(path, []byte(*conf), 0o600); err != nil {
			return err
		}
	}

	if err := config.Save(m.paths, &c); err != nil {
		return err
	}
	m.mu.Lock()
	m.cfg = &c
	m.mu.Unlock()
	return nil
}

// SetOwner — кому отвечать по сокету; ставится при установке службы.
func (m *Manager) SetOwner(owner string) error {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	c := *m.cfg
	m.mu.Unlock()
	c.Owner = owner
	if err := config.Save(m.paths, &c); err != nil {
		return err
	}
	m.mu.Lock()
	m.cfg = &c
	m.mu.Unlock()
	return nil
}

// SetPassword — пароль Cisco в файл, который читает только root/SYSTEM.
// Для Linux без связки ключей; на macOS и Windows пароль держит клиент в
// Keychain / Credential Manager и передаёт при подключении.
func (m *Manager) SetPassword(pw string) error {
	if pw == "" {
		err := os.Remove(m.paths.PasswordFile())
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return config.WriteFileAtomic(m.paths.PasswordFile(), []byte(pw), 0o600)
}

func (m *Manager) Probe(host string) (ProbeResult, error) {
	host = strings.TrimSpace(host)
	ip := netx.ResolveIP(host)
	if ip == "" {
		return ProbeResult{}, fmt.Errorf("не резолвится %s", host)
	}
	pin, err := netx.CertPin(host, ip)
	return ProbeResult{IP: ip, Pin: pin}, err
}

func (m *Manager) Logs() Logs {
	return Logs{
		Cisco:  tailFile(m.paths.CiscoLog(), 40),
		VLESS:  tailFile(m.paths.VLESSLog(), 40),
		Daemon: tailFile(m.paths.DaemonLog(), 40),
	}
}
