// Package daemon — служба cvpn: держит openconnect и sing-box дочерними
// процессами, поднимает wg/awg, отвечает CLI и GUI по сокету.
package daemon

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/inoisy/cisco-vless/internal/config"
	"github.com/inoisy/cisco-vless/internal/netx"
	"github.com/inoisy/cisco-vless/internal/singbox"
	"github.com/inoisy/cisco-vless/internal/vpncscript"
)

type Manager struct {
	op      sync.Mutex // одна операция (on/off/настройки) за раз
	mu      sync.Mutex // поля ниже; держится коротко, статус не ждёт операций
	paths   config.Paths
	cfg     *config.Config
	cisco   *proc
	ciscoIP string // куда поставлен host-маршрут шлюза
	vless   *proc
}

func NewManager(p config.Paths) (*Manager, error) {
	extendPath()
	for _, d := range []string{p.Etc, p.Run, p.Log} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	cfg, err := config.Load(p)
	if err != nil {
		return nil, fmt.Errorf("конфиг %s: %w", p.ConfigFile(), err)
	}
	return &Manager{paths: p, cfg: cfg}, nil
}

func (m *Manager) Owner() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Owner
}

// Boot — при старте службы: снять хвосты прошлого запуска и поднять выход
// из настроек.
func (m *Manager) Boot() {
	vpncscript.Cleanup(m.paths)
	linuxRules(false)
	if e := m.cfg.BootExit; e != "" {
		if msg, err := m.ExitOn(e); err != nil {
			log.Printf("boot %s: %v", e, err)
		} else {
			log.Print(msg)
		}
	}
}

// Shutdown — служба останавливается: гасим своих детей. wg/awg живут
// своими службами/интерфейсами и остаются как есть.
func (m *Manager) Shutdown() {
	m.CiscoOff()
	m.op.Lock()
	defer m.op.Unlock()
	m.vlessOff()
}

// --- status ---------------------------------------------------------------

func (m *Manager) Status(withIP bool) Status {
	c, _ := m.procs()
	m.mu.Lock()
	s := Status{
		Cisco:      c.alive(),
		Exits:      m.configuredExits(),
		Configured: m.cfg.CiscoConfigured(),
		CorpHost:   m.cfg.CorpCheckHost,
		OS:         runtime.GOOS,
	}
	m.mu.Unlock()
	s.Exit = m.currentExit()
	if withIP {
		s.IP = netx.ExternalIP()
	}
	if s.Cisco && s.CorpHost != "" {
		s.CorpIP = netx.LookupSystem(s.CorpHost)
	}
	s.Warnings = netx.Conflicts()
	return s
}

func (m *Manager) configuredExits() []string {
	var out []string
	if m.vlessConfigured() {
		out = append(out, config.ExitVLESS)
	}
	for _, e := range []string{config.ExitWG, config.ExitAWG} {
		if _, err := os.Stat(m.paths.TunnelConf(e)); err == nil {
			out = append(out, e)
		}
	}
	return out
}

// procs — дочерние процессы под mu.
func (m *Manager) procs() (cisco, vless *proc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cisco, m.vless
}

// vlessConfigured: ссылка в настройках или singbox.json от bash-версии.
func (m *Manager) vlessConfigured() bool {
	if m.cfg.VLESSLink != "" {
		return true
	}
	_, err := os.Stat(m.paths.SingBoxFile())
	return err == nil
}

func (m *Manager) currentExit() string {
	if _, v := m.procs(); v.alive() {
		return config.ExitVLESS
	}
	for _, e := range []string{config.ExitWG, config.ExitAWG} {
		if t, ok := m.tunnel(e); ok && t.isUp() {
			return e
		}
	}
	return ""
}

func (m *Manager) tunnel(kind string) (tunnelExit, bool) {
	conf := m.paths.TunnelConf(kind)
	_, err := os.Stat(conf)
	return tunnelExit{kind: kind, conf: conf}, err == nil
}

// --- cisco ----------------------------------------------------------------

func (m *Manager) CiscoOn(req CiscoOn) (string, error) {
	m.op.Lock()
	defer m.op.Unlock()
	if c, _ := m.procs(); c.alive() {
		return "cisco: уже подключён", nil
	}
	c := m.cfg
	if !c.CiscoConfigured() {
		return "", errors.New("cisco не настроен: сервер, логин и сертификат — в настройках или cvpn setup")
	}
	vpncscript.Cleanup(m.paths)

	pass := req.Password
	if pass == "" {
		b, err := os.ReadFile(m.paths.PasswordFile())
		if err != nil || len(b) == 0 {
			return "", errors.New("cisco: нет сохранённого пароля — cvpn password")
		}
		pass = strings.TrimRight(string(b), "\r\n")
	}

	oc, err := findOpenconnect(c)
	if err != nil {
		return "", err
	}
	stock, err := findVPNCScript(c)
	if err != nil {
		return "", err
	}
	script, err := ciscoScript(m.paths)
	if err != nil {
		return "", err
	}

	ip := netx.ResolveIP(c.CiscoHost)
	if ip == "" {
		ip = c.CiscoIP
	}
	if ip == "" {
		return "", fmt.Errorf("cisco: не резолвится %s", c.CiscoHost)
	}
	route, err := netx.PhysRoute()
	if err != nil {
		return "", fmt.Errorf("cisco: %w", err)
	}
	// Шлюз cisco — строго мимо vless/wg. Иначе TLS поднимется через них, а
	// после host-маршрута от vpnc-script пакеты сессии уйдут другим путём с
	// другого IP: SSL read error, reconnect, обрыв.
	if err := netx.AddHostRoute(ip, route); err != nil {
		return "", fmt.Errorf("cisco: маршрут к шлюзу: %w", err)
	}

	args := []string{"--protocol=anyconnect", "--user=" + c.CiscoUser,
		"--servercert", c.CiscoPin, "--resolve", c.CiscoHost + ":" + ip,
		"--script", script,
		"--force-dpd=20", "--reconnect-timeout=3600"}
	if c.CiscoGroup != "" {
		args = append(args, "--authgroup="+c.CiscoGroup)
	}
	args = append(args, ciscoIfaceArgs()...)
	args = append(args, c.CiscoHost)

	cmd := exec.Command(oc, args...)
	// LC_ALL=C — «Configured as» без перевода; путь к штатному скрипту —
	// для cvpn _vpnc-script.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C", vpncscript.EnvStock+"="+stock)
	// Пароль и одноразовый код — ответы на форму «пароль, затем код», как у
	// AnyConnect с двухфакторкой.
	cmd.Stdin = strings.NewReader(pass + "\n" + req.OTP + "\n")
	ready := newWaitFor("Configured as", "Connected as")
	p, err := startProc(cmd, m.paths.CiscoLog(), ready)
	if err != nil {
		netx.DelHostRoute(ip)
		return "", fmt.Errorf("cisco: запуск openconnect: %w", err)
	}

	select {
	case <-ready.hit:
	case <-p.done:
	case <-time.After(60 * time.Second):
		p.stop(5 * time.Second)
	}
	if !p.alive() {
		netx.DelHostRoute(ip)
		vpncscript.Cleanup(m.paths)
		return "", fmt.Errorf("cisco: не подключился. Лог %s:\n%s", m.paths.CiscoLog(), tailFile(m.paths.CiscoLog(), 15))
	}
	m.mu.Lock()
	m.cisco, m.ciscoIP = p, ip
	m.mu.Unlock()
	go m.watchCisco(p, ip)
	return "cisco: подключён", nil
}

// openconnect умер сам (сервер выкинул, истёк reconnect-timeout) — убрать
// маршрут к шлюзу и split-DNS.
func (m *Manager) watchCisco(p *proc, ip string) {
	<-p.done
	m.mu.Lock()
	if m.cisco == p {
		m.cisco = nil
	}
	m.mu.Unlock()
	netx.DelHostRoute(ip)
	vpncscript.Cleanup(m.paths)
	log.Printf("cisco: openconnect завершился")
}

func (m *Manager) CiscoOff() string {
	m.op.Lock()
	defer m.op.Unlock()
	p, _ := m.procs()
	if !p.alive() {
		vpncscript.Cleanup(m.paths)
		return "cisco: и так выключен"
	}
	p.stop(10 * time.Second)
	// watchCisco дочистит маршрут и DNS; ждём его, чтобы статус был честный.
	for i := 0; i < 20; i++ {
		m.mu.Lock()
		gone := m.cisco != p
		m.mu.Unlock()
		if gone {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "cisco: отключён"
}

// --- выходы: vless / wg / awg ----------------------------------------------

func (m *Manager) ExitOn(kind string) (string, error) {
	m.op.Lock()
	defer m.op.Unlock()
	if m.currentExit() == kind {
		return kind + ": уже включён", nil
	}
	var msgs []string
	switch kind {
	case config.ExitVLESS:
		m.mu.Lock()
		ok := m.vlessConfigured()
		m.mu.Unlock()
		if !ok {
			return "", errors.New("vless: нет ссылки vless:// в настройках")
		}
		msgs = m.exitsOff()
		if err := m.vlessOn(); err != nil {
			return "", err
		}
	case config.ExitWG, config.ExitAWG:
		t, ok := m.tunnel(kind)
		if !ok {
			return "", fmt.Errorf("%s: не настроен (нет %s)", kind, t.conf)
		}
		msgs = m.exitsOff()
		if err := t.up(); err != nil {
			return "", fmt.Errorf("%s: не поднялся: %w", kind, err)
		}
	default:
		return "", fmt.Errorf("неизвестный выход %q", kind)
	}
	return strings.Join(append(msgs, kind+": включён"), "\n"), nil
}

func (m *Manager) ExitOff(kind string) string {
	m.op.Lock()
	defer m.op.Unlock()
	if kind == "" {
		msgs := m.exitsOff()
		if len(msgs) == 0 {
			return "выход и так выключен"
		}
		return strings.Join(msgs, "\n")
	}
	if m.currentExit() != kind {
		return kind + ": и так выключен"
	}
	m.exitOff(kind)
	return kind + ": отключён"
}

// exitsOff выключает текущий выход: они взаимоисключающие.
func (m *Manager) exitsOff() []string {
	var msgs []string
	for _, e := range config.Exits {
		if e == config.ExitVLESS {
			if _, v := m.procs(); !v.alive() {
				continue
			}
		} else if t, ok := m.tunnel(e); !ok || !t.isUp() {
			continue
		}
		m.exitOff(e)
		msgs = append(msgs, e+": отключён")
	}
	return msgs
}

func (m *Manager) exitOff(kind string) {
	if kind == config.ExitVLESS {
		m.vlessOff()
		return
	}
	if t, ok := m.tunnel(kind); ok {
		if err := t.down(); err != nil {
			log.Printf("%s down: %v", kind, err)
		}
	}
}

func (m *Manager) vlessOn() error {
	sb, err := findSingBox(m.cfg, m.paths)
	if err != nil {
		return err
	}
	if err := m.writeSingBox(); err != nil {
		return err
	}
	linuxRules(true)
	cmd := exec.Command(sb, "run", "-c", m.paths.SingBoxFile())
	p, err := startProc(cmd, m.paths.VLESSLog(), nil)
	if err != nil {
		linuxRules(false)
		return fmt.Errorf("vless: запуск sing-box: %w", err)
	}
	select {
	case <-p.done:
	case <-time.After(2 * time.Second):
	}
	if !p.alive() {
		linuxRules(false)
		return fmt.Errorf("vless: не поднялся. Лог:\n%s", tailFile(m.paths.VLESSLog(), 15))
	}
	m.mu.Lock()
	m.vless = p
	m.mu.Unlock()
	go func() {
		<-p.done
		m.mu.Lock()
		if m.vless == p {
			m.vless = nil
			linuxRules(false)
		}
		m.mu.Unlock()
	}()
	return nil
}

func (m *Manager) vlessOff() {
	m.mu.Lock()
	p := m.vless
	m.vless = nil
	m.mu.Unlock()
	if !p.alive() {
		return
	}
	p.stop(5 * time.Second)
	linuxRules(false)
}

// writeSingBox — конфиг из ссылки. В обход туннеля: шлюз Cisco и (Linux)
// публичные DNS провайдера.
func (m *Manager) writeSingBox() error {
	if m.cfg.VLESSLink == "" {
		return nil // конфиг от bash-версии: ссылки нет, берём singbox.json как есть
	}
	var exclude []string
	if ip := m.cfg.CiscoIP; ip != "" {
		exclude = append(exclude, ip)
	}
	if ip := netx.ResolveIP(m.cfg.CiscoHost); ip != "" && ip != m.cfg.CiscoIP {
		exclude = append(exclude, ip)
	}
	exclude = append(exclude, netx.PublicDNS()...)
	b, err := singbox.Config(m.cfg.VLESSLink, exclude, runtime.GOOS)
	if err != nil {
		return fmt.Errorf("vless: %w", err)
	}
	return config.WriteFileAtomic(m.paths.SingBoxFile(), b, 0o600)
}

// --- всё сразу ------------------------------------------------------------

// On — выход (vless, если не указан) + cisco.
func (m *Manager) On(exit string, req CiscoOn) (string, error) {
	if exit == "" {
		exit = config.ExitVLESS
	}
	var msgs []string
	msg, err := m.ExitOn(exit)
	if err != nil {
		msgs = append(msgs, err.Error())
	} else {
		msgs = append(msgs, msg)
	}
	msg, err2 := m.CiscoOn(req)
	if err2 != nil {
		return strings.Join(msgs, "\n"), err2
	}
	msgs = append(msgs, msg)
	return strings.Join(msgs, "\n"), err
}

func (m *Manager) Off() string {
	return m.CiscoOff() + "\n" + m.ExitOff("")
}
