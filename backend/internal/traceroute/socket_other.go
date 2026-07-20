//go:build !linux

package traceroute

import "net"

func listenICMPPacket(network, address string) (net.PacketConn, error) {
	return net.ListenPacket(network, address)
}
