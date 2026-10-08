package daemon

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// proc — дочерний процесс демона (openconnect, sing-box) с логом в файл.
type proc struct {
	cmd  *exec.Cmd
	done chan struct{}
	log  *os.File
}

func startProc(cmd *exec.Cmd, logPath string, watch io.Writer) (*proc, error) {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	var w io.Writer = f
	if watch != nil {
		w = io.MultiWriter(f, watch)
	}
	cmd.Stdout, cmd.Stderr = w, w
	prepareCmd(cmd)
	if err := cmd.Start(); err != nil {
		f.Close()
		return nil, err
	}
	p := &proc{cmd: cmd, done: make(chan struct{}), log: f}
	go func() {
		cmd.Wait()
		f.Close()
		close(p.done)
	}()
	return p, nil
}

func (p *proc) alive() bool {
	if p == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// stop: мягко (unix — SIGTERM, openconnect успевает вызвать disconnect
// скрипта), через timeout — kill.
func (p *proc) stop(timeout time.Duration) {
	if !p.alive() {
		return
	}
	terminate(p.cmd.Process)
	select {
	case <-p.done:
	case <-time.After(timeout):
		p.cmd.Process.Kill()
		<-p.done
	}
}

// waitFor ждёт строку в выводе процесса или его смерть.
type waitFor struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	needs []string
	hit   chan struct{}
	once  sync.Once
}

func newWaitFor(needs ...string) *waitFor {
	return &waitFor{needs: needs, hit: make(chan struct{})}
}

func (w *waitFor) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(b)
	s := w.buf.String()
	for _, n := range w.needs {
		if strings.Contains(s, n) {
			w.once.Do(func() { close(w.hit) })
		}
	}
	// Хвоста в 4К хватает, чтобы не порвать искомую строку.
	if w.buf.Len() > 8192 {
		tail := s[len(s)-4096:]
		w.buf.Reset()
		w.buf.WriteString(tail)
	}
	return len(b), nil
}

func tailFile(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
