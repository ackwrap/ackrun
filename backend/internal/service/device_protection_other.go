//go:build !linux

package service

import "github.com/ackwrap/ackrun/internal/paths"

func newDeviceProtection(*paths.Paths) deviceProtectionFirewall { return nil }
