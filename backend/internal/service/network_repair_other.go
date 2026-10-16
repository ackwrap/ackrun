//go:build !linux

package service

import (
	"fmt"

	"github.com/ackwrap/ackrun/internal/paths"
)

func resetPlatformDeviceDNS(*paths.Paths) error {
	return fmt.Errorf("强制网络重置仅支持 Linux/OpenWrt")
}

func resetPlatformTransitionGuard(*paths.Paths) error {
	return fmt.Errorf("强制网络重置仅支持 Linux/OpenWrt")
}

func forceCleanupPlatformSingboxState(string) (platformCleanupResult, error) {
	return platformCleanupResult{}, fmt.Errorf("强制网络修复仅支持 Linux/OpenWrt")
}

func stopPlatformSingboxForRepair() error {
	return fmt.Errorf("强制网络修复仅支持 Linux/OpenWrt")
}
