//go:build !windows

package main

import (
	"os"

	"github.com/inoisy/cisco-vless/internal/config"
)

func secureDirs(p config.Paths) error {
	return os.MkdirAll(p.Etc, 0o755)
}
