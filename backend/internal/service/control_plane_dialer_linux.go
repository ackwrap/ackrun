//go:build linux

package service

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

var controlPlaneResolverFiles = []string{
	"/tmp/resolv.conf.d/resolv.conf.auto",
	"/run/systemd/resolve/resolv.conf",
	"/etc/resolv.conf",
}

// Use the same socket mark as the core's direct sockets: it bypasses both TUN
// capture and the transition guard, without exempting a UID or forwarded traffic.
func markedControlPlaneDialer(timeout time.Duration) net.Dialer {
	return net.Dialer{
		Timeout: timeout,
		Control: func(_, _ string, raw syscall.RawConn) error {
			var markErr error
			if err := raw.Control(func(fd uintptr) {
				markErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, defaultAutoRedirectMark)
			}); err != nil {
				return err
			}
			// Unprivileged, non-TUN installations must still be able to connect.
			if errors.Is(markErr, syscall.EPERM) || errors.Is(markErr, syscall.EACCES) {
				return nil
			}
			return markErr
		},
	}
}

func controlPlaneDialer(timeout time.Duration) net.Dialer {
	dialer := markedControlPlaneDialer(timeout)
	dialer.KeepAlive = 30 * time.Second
	dnsDialer := markedControlPlaneDialer(4 * time.Second)
	dialer.Resolver = controlPlaneResolver(dnsDialer.DialContext)
	return dialer
}

func controlPlaneResolver(dial func(context.Context, string, string) (net.Conn, error)) *net.Resolver {
	var next atomic.Uint64
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// A loopback resolver may forward into the stopped core. Query the
			// configured WAN/system upstreams with marked sockets instead. Reload
			// the files on each attempt to follow WAN renewals without a restart.
			if servers := controlPlaneNameServers(); len(servers) > 0 {
				address = servers[(next.Add(1)-1)%uint64(len(servers))]
			}
			// With no upstream file, preserve the platform's normal resolver
			// behavior rather than silently choosing a public DNS provider.
			return dial(ctx, network, address)
		},
	}
}

func controlPlaneNameServers() []string {
	seen := make(map[string]bool)
	servers := make([]string, 0, 2)
	for _, path := range controlPlaneResolverFiles {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 2 || fields[0] != "nameserver" {
				continue
			}
			address, err := netip.ParseAddr(strings.Trim(fields[1], "[]"))
			if err != nil || address.IsLoopback() || address.IsUnspecified() || address.IsMulticast() {
				continue
			}
			server := net.JoinHostPort(address.Unmap().String(), "53")
			if !seen[server] {
				seen[server] = true
				servers = append(servers, server)
			}
		}
		_ = file.Close()
	}
	return servers
}
