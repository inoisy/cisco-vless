package singbox

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// GeoIPURL — российские IP-сети для правила geoip-ru.
const GeoIPURL = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs"

// geoipMaxAge — после этого срока файл пробуем обновить; не вышло — берём
// старый.
const geoipMaxAge = 30 * 24 * time.Hour

var srsMagic = []byte("SRS")

// EnsureGeoIP держит path свежим: нет файла или старше geoipMaxAge — качает
// заново. Возвращает true, если файл годен (свежий или старый, но есть).
// Качает напрямую, до старта sing-box: туннеля ещё нет.
func EnsureGeoIP(path string) (bool, error) {
	st, err := os.Stat(path)
	have := err == nil && st.Size() > 0
	if have && time.Since(st.ModTime()) < geoipMaxAge {
		return true, nil
	}
	if err := download(GeoIPURL, path, 20*time.Second); err != nil {
		return have, err
	}
	return true, nil
}

func download(url, path string, timeout time.Duration) error {
	c := &http.Client{Timeout: timeout}
	resp, err := c.Get(url)
	if err != nil {
		return fmt.Errorf("geoip-ru: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("geoip-ru: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("geoip-ru: %w", err)
	}
	if !bytes.HasPrefix(b, srsMagic) {
		return fmt.Errorf("geoip-ru: не rule-set sing-box")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".geoip-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
