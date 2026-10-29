//go:build !linux

package service

// Automatic recovery of system-wide sing-box processes is Linux/OpenWrt-only.
func stopPlatformSingboxForStart() error { return nil }

func verifyPlatformSingboxStopped() error { return nil }
