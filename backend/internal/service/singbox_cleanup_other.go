//go:build !linux && !windows

package service

import (
	"fmt"
	"os"
	"path/filepath"
)

func cleanupPlatformSingboxState(string) (platformCleanupResult, error) {
	return platformCleanupResult{}, nil
}

func snapshotPlatformPriorityOneTables(activeTUNState) (platformNetworkBaseline, error) {
	return platformNetworkBaseline{}, nil
}

func recordPlatformSingboxRouteTables(string, <-chan struct{}) error {
	return nil
}

func syncOwnershipStateParentDirectory(path string) error {
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open route-table ownership state directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync route-table ownership state directory: %w", err)
	}
	return nil
}
