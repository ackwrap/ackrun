//go:build !linux

package service

import "github.com/ackwrap/ackrun/internal/paths"

// The transition firewall protects Linux/OpenWrt route-managing TUN mode.
func newCoreTransitionGuard(*paths.Paths) coreTransitionGuard { return nil }
