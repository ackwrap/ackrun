//go:build !linux

package service

func acquireNetworkLifecycleFileLock(string) (func(), error) {
	return func() {}, nil
}
