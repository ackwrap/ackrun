//go:build !linux

package service

import "github.com/ackwrap/ackrun/internal/paths"

func platformSupportsDNSMasqTakeover() bool { return false }

func newPlatformDNSMasqLifecycle(*paths.Paths) dnsmasqLifecycle {
	return noopDNSMasqLifecycle{}
}
