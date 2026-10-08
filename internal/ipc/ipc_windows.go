package ipc

import (
	"context"
	"fmt"
	"net"
	"os/user"

	"github.com/Microsoft/go-winio"
)

func dial(ctx context.Context, addr string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, addr)
}

// Listen: pipe доступен SYSTEM, администраторам и владельцу (по SID);
// без владельца — интерактивным пользователям.
func Listen(addr, owner string) (net.Listener, error) {
	who := "IU"
	if owner != "" {
		if u, err := user.Lookup(owner); err == nil {
			who = u.Uid // на Windows Uid — это SID
		}
	}
	sddl := fmt.Sprintf("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;%s)", who)
	return winio.ListenPipe(addr, &winio.PipeConfig{SecurityDescriptor: sddl})
}
