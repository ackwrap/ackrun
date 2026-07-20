//go:build linux

package traceroute

import (
	"context"
	"net"
	"syscall"
)

const autoRedirectOutputMark = 0x2024

func listenICMPPacket(network, address string) (net.PacketConn, error) {
	config := net.ListenConfig{
		Control: func(_, _ string, raw syscall.RawConn) error {
			var markErr error
			if err := raw.Control(func(fd uintptr) {
				markErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, autoRedirectOutputMark)
			}); err != nil {
				return err
			}
			return markErr
		},
	}
	return config.ListenPacket(context.Background(), network, address)
}
