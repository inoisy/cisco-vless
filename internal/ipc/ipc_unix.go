//go:build !windows

package ipc

import (
	"context"
	"net"
	"os"
	"os/user"
	"strconv"
)

func dial(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", addr)
}

// Listen: сокет 0600, владелец — owner (если задан), иначе только root.
func Listen(addr, owner string) (net.Listener, error) {
	os.Remove(addr)
	l, err := net.Listen("unix", addr)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(addr, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	if owner != "" {
		if u, err := user.Lookup(owner); err == nil {
			uid, _ := strconv.Atoi(u.Uid)
			os.Chown(addr, uid, -1)
		}
	}
	return l, nil
}
