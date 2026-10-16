package service

import "fmt"

type deviceDNSLifecycle interface {
	Activate([]string, ...string) error
	Restore() error
}

type unsupportedDeviceDNS struct{}

func (unsupportedDeviceDNS) Activate([]string, ...string) error {
	return fmt.Errorf("设备 DNS 接管仅支持 Linux/OpenWrt")
}
func (unsupportedDeviceDNS) Restore() error { return nil }
