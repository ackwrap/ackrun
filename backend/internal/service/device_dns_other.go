//go:build !linux

package service

import "github.com/ackwrap/ackrun/internal/paths"

func newDeviceDNSLifecycle(*paths.Paths) deviceDNSLifecycle { return unsupportedDeviceDNS{} }
