//go:build !linux

package service

import "fmt"

func forceCleanupPlatformSingboxState(string) (platformCleanupResult, error) {
	return platformCleanupResult{}, fmt.Errorf("强制网络修复仅支持 Linux/OpenWrt")
}

func stopPlatformSingboxForRepair() error {
	return fmt.Errorf("强制网络修复仅支持 Linux/OpenWrt")
}
