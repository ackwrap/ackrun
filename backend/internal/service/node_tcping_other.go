//go:build !linux

package service

import (
	"context"
	"net"
	"strings"
	"time"
)

func tcpingDialer(timeout time.Duration) net.Dialer {
	return net.Dialer{Timeout: timeout}
}

func resolveTCPingServer(ctx context.Context, server string) ([]net.IP, error) {
	if address := net.ParseIP(strings.TrimSpace(server)); address != nil {
		return []net.IP{address}, nil
	}
	return net.DefaultResolver.LookupIP(ctx, "ip", server)
}
