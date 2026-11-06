//go:build linux

package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

func tcpingDialer(timeout time.Duration) net.Dialer {
	return markedControlPlaneDialer(timeout)
}

func resolveTCPingServer(ctx context.Context, server string) ([]net.IP, error) {
	if address := net.ParseIP(strings.TrimSpace(server)); address != nil {
		return []net.IP{address}, nil
	}
	nameServers := controlPlaneNameServers()
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
