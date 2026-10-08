package netx

import (
	"io"
	"net/http"
	"time"
)

func httpGet(url string, timeout time.Duration) (string, error) {
	c := &http.Client{Timeout: timeout}
	resp, err := c.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return string(b), err
}
