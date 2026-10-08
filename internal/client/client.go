// Package client — общий клиент демона для CLI и GUI: запросы плюс пароль
// Cisco (на macOS он в Keychain пользователя и передаётся при подключении).
package client

import (
	"runtime"

	"github.com/inoisy/cisco-vless/internal/config"
	"github.com/inoisy/cisco-vless/internal/daemon"
	"github.com/inoisy/cisco-vless/internal/ipc"
)

type Client struct {
	c *ipc.Client
}

func New() *Client {
	return &Client{c: ipc.NewClient(config.Default().Socket)}
}

func (c *Client) Status(withIP bool) (daemon.Status, error) {
	var s daemon.Status
	path := "/status"
	if withIP {
		path += "?ip=1"
	}
	return s, c.c.Do("GET", path, nil, &s)
}

func (c *Client) msg(path string, in any) (string, error) {
	var m daemon.Message
	err := c.c.Do("POST", path, in, &m)
	return m.Message, err
}

// ciscoReq добавляет пароль из Keychain (macOS).
func (c *Client) ciscoReq(otp string) (daemon.CiscoOn, error) {
	req := daemon.CiscoOn{OTP: otp}
	if KeychainUsed() {
		s, err := c.Settings()
		if err != nil {
			return req, err
		}
		if s.CiscoUser == "" {
			return req, nil // демон сам скажет, что cisco не настроен
		}
		pw, err := keychainGet(s.CiscoUser)
		if err != nil {
			return req, err
		}
		req.Password = pw
	}
	return req, nil
}

func (c *Client) On(exit, otp string) (string, error) {
	req, err := c.ciscoReq(otp)
	if err != nil {
		return "", err
	}
	return c.msg("/on", struct {
		Exit string `json:"exit"`
		daemon.CiscoOn
	}{exit, req})
}

func (c *Client) Off() (string, error) { return c.msg("/off", nil) }

func (c *Client) CiscoOn(otp string) (string, error) {
	req, err := c.ciscoReq(otp)
	if err != nil {
		return "", err
	}
	return c.msg("/cisco/on", req)
}

func (c *Client) CiscoOff() (string, error) { return c.msg("/cisco/off", nil) }

func (c *Client) ExitOn(kind string) (string, error) { return c.msg("/exit/"+kind+"/on", nil) }

func (c *Client) ExitOff(kind string) (string, error) {
	if kind == "" {
		return c.msg("/exit/off", nil)
	}
	return c.msg("/exit/"+kind+"/off", nil)
}

func (c *Client) Settings() (daemon.Settings, error) {
	var s daemon.Settings
	return s, c.c.Do("GET", "/settings", nil, &s)
}

func (c *Client) SetSettings(s daemon.Settings) (daemon.Settings, error) {
	var out daemon.Settings
	return out, c.c.Do("PUT", "/settings", s, &out)
}

func (c *Client) Probe(host string) (daemon.ProbeResult, error) {
	var r daemon.ProbeResult
	return r, c.c.Do("POST", "/probe", daemon.Probe{Host: host}, &r)
}

func (c *Client) Logs() (daemon.Logs, error) {
	var l daemon.Logs
	return l, c.c.Do("GET", "/logs", nil, &l)
}

// KeychainUsed — пароль хранит клиент (macOS), а не демон.
func KeychainUsed() bool { return runtime.GOOS == "darwin" }

// SetPassword: macOS — Keychain пользователя, иначе — файл у демона.
func (c *Client) SetPassword(user, pw string) error {
	if KeychainUsed() {
		return keychainSet(user, pw)
	}
	_, err := c.msg("/password", daemon.Password{Password: pw})
	return err
}

func (c *Client) HasPassword(s daemon.Settings) bool {
	if KeychainUsed() {
		_, err := keychainGet(s.CiscoUser)
		return err == nil
	}
	return s.HasPassword
}
