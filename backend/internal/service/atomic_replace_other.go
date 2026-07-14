//go:build !windows

package service

import "os"

func atomicReplaceFile(source, target string) error {
	return os.Rename(source, target)
}
