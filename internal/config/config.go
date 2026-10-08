// Package config — настройки cvpn и пути файлов по ОС.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// Выходы — куда идёт весь не-корпоративный трафик. Взаимоисключающие.
const (
	ExitVLESS = "vless"
	ExitWG    = "wg"
	ExitAWG   = "awg"
)

var Exits = []string{ExitVLESS, ExitWG, ExitAWG}

type Config struct {
	CiscoHost  string `json:"cisco_host"`
	CiscoIP    string `json:"cisco_ip"` // запасной, если DNS не ответил
	CiscoUser  string `json:"cisco_user"`
	CiscoGroup string `json:"cisco_group,omitempty"`
	CiscoPin   string `json:"cisco_pin"` // pin-sha256:...

	CorpDomains   []string `json:"corp_domains,omitempty"`
	CorpCheckHost string   `json:"corp_check_host,omitempty"`

	VLESSLink string `json:"vless_link,omitempty"`

	// Выход, который демон поднимает при старте ("" — никакой).
	BootExit string `json:"boot_exit,omitempty"`

	// Пользователь, которому демон отвечает по сокету (кроме root/админов).
	Owner string `json:"owner,omitempty"`

	// Пути к внешним программам; пусто — искать в PATH и стандартных местах.
	Openconnect string `json:"openconnect,omitempty"`
	VPNCScript  string `json:"vpnc_script,omitempty"`
	SingBox     string `json:"sing_box,omitempty"`
}

func (c *Config) CiscoConfigured() bool {
	return c.CiscoHost != "" && c.CiscoUser != "" && c.CiscoPin != ""
}

// Paths — где что лежит.
type Paths struct {
	Etc    string // конфиг, singbox.json, конфиги wg/awg, пароль (Linux, Windows)
	Run    string // состояние DNS
	Log    string
	Socket string // unix-сокет или named pipe
	Lib    string // где лежит сам cvpn
}

func Default() Paths {
	// Песочница для разработки и тестов: всё под одним каталогом, без root.
	if root := os.Getenv("CVPN_ROOT"); root != "" {
		return Paths{
			Etc:    filepath.Join(root, "etc"),
			Run:    filepath.Join(root, "run"),
			Log:    filepath.Join(root, "log"),
			Socket: filepath.Join(root, "cvpn.sock"),
			Lib:    filepath.Join(root, "lib"),
		}
	}
	if runtime.GOOS == "windows" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		dir := filepath.Join(pd, "cvpn")
		pf := os.Getenv("ProgramFiles")
		if pf == "" {
			pf = `C:\Program Files`
		}
		return Paths{
			Etc:    dir,
			Run:    dir,
			Log:    filepath.Join(dir, "log"),
			Socket: `\\.\pipe\cvpn`,
			Lib:    filepath.Join(pf, "cvpn"),
		}
	}
	return Paths{
		Etc:    "/usr/local/etc/cvpn",
		Run:    "/var/run",
		Log:    "/var/log",
		Socket: "/var/run/cvpn.sock",
		Lib:    "/usr/local/lib/cvpn",
	}
}

func (p Paths) ConfigFile() string  { return filepath.Join(p.Etc, "config.json") }
func (p Paths) SingBoxFile() string { return filepath.Join(p.Etc, "singbox.json") }
func (p Paths) PasswordFile() string {
	return filepath.Join(p.Etc, "cisco-password")
}
func (p Paths) DNSState() string { return filepath.Join(p.Run, "cvpn-dns-domains") }
func (p Paths) CiscoLog() string { return filepath.Join(p.Log, "cvpn-cisco.log") }
func (p Paths) VLESSLog() string { return filepath.Join(p.Log, "cvpn-vless.log") }
func (p Paths) DaemonLog() string {
	return filepath.Join(p.Log, "cvpn-daemon.log")
}

// TunnelConf — конфиг wg/awg. Имя файла = имя туннеля (wg-quick, служба
// туннеля на Windows), поэтому фиксированное.
func (p Paths) TunnelConf(exit string) string {
	return filepath.Join(p.Etc, "cvpn-"+exit+".conf")
}

func Load(p Paths) (*Config, error) {
	b, err := os.ReadFile(p.ConfigFile())
	if errors.Is(err, os.ErrNotExist) {
		if c, ok := loadLegacy(p); ok {
			return c, nil
		}
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	c := &Config{}
	return c, json.Unmarshal(b, c)
}

func Save(p Paths, c *Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.Etc, 0o755); err != nil {
		return err
	}
	// В конфиге ссылка vless с ключами — только владельцу.
	return WriteFileAtomic(p.ConfigFile(), b, 0o600)
}

func WriteFileAtomic(path string, b []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
