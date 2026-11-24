package netx

import (
	"io"
	"net/http"
	"time"
)

// Без keep-alive: соединение, открытое до включения vless/wg, живёт мимо
// туннеля, и повторный запрос по нему показывал бы прежний внешний IP.
var httpTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableKeepAlives = true
	return t
}()

func httpGet(url string, timeout time.Duration) (string, error) {
	c := &http.Client{Timeout: timeout, Transport: httpTransport}
	resp, err := c.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return string(b), err
}
