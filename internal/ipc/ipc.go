// Package ipc — HTTP между демоном и клиентами (CLI, GUI) через unix-сокет
// (macOS, Linux) или named pipe (Windows). Доступ — root/админы и владелец
// из конфига: права на сам сокет/pipe.
package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type Client struct {
	http *http.Client
}

func NewClient(addr string) *Client {
	return &Client{http: &http.Client{Transport: Transport(addr), Timeout: 3 * time.Minute}}
}

// Transport — HTTP поверх сокета/pipe демона; хост в URL не важен.
func Transport(addr string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dial(ctx, addr)
		},
	}
}

// Error — ошибка, которую вернул демон (текст для человека).
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

// Do: method, path, тело (nil — без тела), out (nil — не разбирать).
func (c *Client) Do(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "http://cvpn"+path, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("демон cvpn недоступен (%v). Служба установлена и запущена? cvpn service install", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct{ Error string }
		if json.Unmarshal(b, &e) == nil && e.Error != "" {
			return &Error{e.Error}
		}
		return &Error{fmt.Sprintf("%s: %s", resp.Status, bytes.TrimSpace(b))}
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}
