//go:build linux

package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

var tcpingResolverFiles = []string{
	"/tmp/resolv.conf.d/resolv.conf.auto",
	"/run/systemd/resolve/resolv.conf",
	"/etc/resolv.conf",
}

func tcpingDialer(timeout time.Duration) net.Dialer {
	return net.Dialer{
		Timeout: timeout,
		Control: func(_, _ string, raw syscall.RawConn) error {
			var markErr error
			if err := raw.Control(func(fd uintptr) {
				markErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, defaultAutoRedirectMark)
			}); err != nil {
				return err
			}
			if errors.Is(markErr, syscall.EPERM) || errors.Is(markErr, syscall.EACCES) {
				return nil
			}
			return markErr
		},
	}
}

func resolveTCPingServer(ctx context.Context, server string) ([]net.IP, error) {
	if address := net.ParseIP(strings.TrimSpace(server)); address != nil {
		return []net.IP{address}, nil
	}
	nameServers := tcpingNameServers()
	if len(nameServers) == 0 {
		return nil, errors.New("未找到可用于直连测速的非回环系统 DNS 服务器")
	}
	var lastErr error
	for _, nameServer := range nameServers {
		dialer := tcpingDialer(4 * time.Second)
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, nameServer)
			},
		}
		addresses, err := resolver.LookupIP(ctx, "ip", server)
		if err == nil && len(addresses) > 0 {
			return orderedTCPingIPs(addresses, nil)
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("直连 DNS 解析节点地址失败: %w", lastErr)
	}
	return nil, errors.New("直连 DNS 未返回节点服务器地址")
}

func tcpingNameServers() []string {
	seen := make(map[string]bool)
	servers := make([]string, 0, 2)
	for _, path := range tcpingResolverFiles {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) != 2 || fields[0] != "nameserver" {
				continue
			}
			address := net.ParseIP(strings.Trim(fields[1], "[]"))
			if address == nil || address.IsLoopback() {
				continue
			}
			server := net.JoinHostPort(address.String(), "53")
			if !seen[server] {
				seen[server] = true
				servers = append(servers, server)
			}
		}
		_ = file.Close()
	}
	return servers
}

func orderedTCPingIPs(addresses []net.IP, err error) ([]net.IP, error) {
	if err != nil {
		return nil, err
	}
	ordered := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if address.To4() != nil {
			ordered = append(ordered, address)
		}
	}
	for _, address := range addresses {
		if address.To4() == nil && address.To16() != nil {
			ordered = append(ordered, address)
		}
	}
	return ordered, nil
}
