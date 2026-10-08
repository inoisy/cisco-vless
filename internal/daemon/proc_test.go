//go:build !windows

package daemon

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Как openconnect: читает пароль и код из stdin, при успехе печатает
// «Configured as» и живёт дальше.
const fakeOC = `read pw; read otp
[ "$pw" = good ] || { echo "Login failed."; exit 1; }
echo "otp=$otp"; echo "Configured as 10.1.2.3, with SSL connected"; exec sleep 30`

func startFake(t *testing.T, pw string) (*proc, *waitFor, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "oc.log")
	cmd := exec.Command("/bin/sh", "-c", fakeOC)
	cmd.Stdin = strings.NewReader(pw + "\n123456\n")
	ready := newWaitFor("Configured as", "Connected as")
	p, err := startProc(cmd, logPath, ready)
	if err != nil {
		t.Fatal(err)
	}
	return p, ready, logPath
}

func TestProcConnected(t *testing.T) {
	p, ready, logPath := startFake(t, "good")
	select {
	case <-ready.hit:
	case <-p.done:
		t.Fatal("процесс умер")
	case <-time.After(5 * time.Second):
		t.Fatal("нет Configured as")
	}
	if !p.alive() {
		t.Fatal("должен жить")
	}
	p.stop(2 * time.Second)
	if p.alive() {
		t.Fatal("не остановился")
	}
	if !strings.Contains(tailFile(logPath, 5), "otp=123456") {
		t.Fatalf("лог: %q", tailFile(logPath, 5))
	}
}

func TestProcLoginFailed(t *testing.T) {
	p, ready, logPath := startFake(t, "bad")
	select {
	case <-ready.hit:
		t.Fatal("ложный успех")
	case <-p.done:
	case <-time.After(5 * time.Second):
		t.Fatal("не завершился")
	}
	if got := tailFile(logPath, 5); got != "Login failed." {
		t.Fatalf("лог: %q", got)
	}
}

func TestStopNilProc(t *testing.T) {
	var p *proc
	if p.alive() {
		t.Fatal("nil жив")
	}
	p.stop(time.Second)
}
