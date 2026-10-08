package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func prepareCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// У службы нет консоли — Ctrl+C детям не послать. Убиваем; wintun-адаптер
// и его маршруты уходят вместе с процессом, остальное снимает cleanup.
func terminate(p *os.Process) { p.Kill() }
