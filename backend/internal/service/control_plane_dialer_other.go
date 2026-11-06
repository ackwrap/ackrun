//go:build !linux

package service

import (
	"net"
	"time"
)

func controlPlaneDialer(timeout time.Duration) net.Dialer {
	return net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
}
