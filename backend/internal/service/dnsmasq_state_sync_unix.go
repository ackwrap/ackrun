//go:build !windows

package service

import (
	"os"
	"path/filepath"
)

func syncDNSMasqStateDirectory(path string) error {
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
