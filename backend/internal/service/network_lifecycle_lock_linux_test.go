//go:build linux

package service

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNetworkLifecycleFileLockSerializesProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".network-lifecycle.lock")
	releaseFirst, err := acquireNetworkLifecycleFileLock(path)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan func(), 1)
	errorsCh := make(chan error, 1)
	go func() {
		release, lockErr := acquireNetworkLifecycleFileLock(path)
		if lockErr != nil {
			errorsCh <- lockErr
			return
		}
		acquired <- release
	}()

	select {
	case release := <-acquired:
		release()
		t.Fatal("second lock acquired before first lock was released")
	case err := <-errorsCh:
		t.Fatal(err)
	case <-time.After(100 * time.Millisecond):
	}

	releaseFirst()
	select {
	case release := <-acquired:
		release()
	case err := <-errorsCh:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("second lock did not acquire after release")
	}
}
