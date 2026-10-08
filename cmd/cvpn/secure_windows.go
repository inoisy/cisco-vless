package main

import (
	"os"

	"github.com/inoisy/cisco-vless/internal/config"
	"github.com/inoisy/cisco-vless/internal/netx"
)

// В ProgramData по умолчанию пишут все пользователи. Там пароль, ссылка
// vless и скрипт, который запускается от SYSTEM, — оставляем доступ только
// SYSTEM и администраторам.
func secureDirs(p config.Paths) error {
	if err := os.MkdirAll(p.Etc, 0o755); err != nil {
		return err
	}
	_, err := netx.Run("icacls.exe", p.Etc, "/inheritance:r",
		"/grant:r", "*S-1-5-18:(OI)(CI)F", "*S-1-5-32-544:(OI)(CI)F")
	return err
}
