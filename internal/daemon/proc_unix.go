//go:build !windows

package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

// Своя группа процессов: сигналы терминала демона детей не задевают.
func prepareCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminate(p *os.Process) { p.Signal(syscall.SIGTERM) }
