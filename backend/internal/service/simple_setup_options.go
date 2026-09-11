package service

import (
	"errors"
	"net/netip"
	"os"
	"reflect"
	"strings"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

func validateSimpleSetupOptions(options *model.SimpleSetupOptions) error {
	if options == nil {
		return errors.New("配置选项不能为空")
	}
	if options.CNOutbound != "bypass" && options.CNOutbound != "direct" {
		return errors.New("国内流量请选择内核绕过或直连")
	}
	if options.DefaultOutbound != "proxy" && options.DefaultOutbound != "direct" {
		return errors.New("默认去向请选择代理或直连")
	}
	defaults := model.DefaultSimpleSetupOptions()
	if len(options.AppRouting) != len(defaults.AppRouting) {
		return errors.New("请为每个应用分类选择分流方式")
	}
	for name, outbound := range options.AppRouting {
		if defaults.AppRouting[name] == "" || (outbound != "proxy" && outbound != "direct") {
			return errors.New("应用分流仅支持代理或直连")
		}
	}
	for _, address := range []*string{&options.LocalDNS, &options.ProxyDNS} {
		ip, err := netip.ParseAddr(strings.TrimSpace(*address))
		if err != nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() || ip.Zone() != "" {
			return errors.New("DNS 请输入有效的服务器 IP 地址")
		}
		*address = ip.String()
	}
	if options.DNSStrategy != "prefer_ipv4" && options.DNSStrategy != "prefer_ipv6" && options.DNSStrategy != "ipv4_only" {
		return errors.New("请选择 IPv4 优先、IPv6 优先或仅解析 IPv4")
	}
	if len(options.DirectDevices) > 128 {
		return errors.New("设备直连最多支持 128 个地址或网段")
	}
	devices := []string{}
	for _, value := range options.DirectDevices {
		value = strings.TrimSpace(value)
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, addressErr := netip.ParseAddr(value)
			if addressErr != nil || address.Zone() != "" {
				return errors.New("设备直连请输入 IP 地址或 CIDR 网段，每行一个")
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		if prefix.Bits() == 0 || prefix.Addr().IsUnspecified() || prefix.Addr().IsMulticast() || prefix.Addr().IsLoopback() {
			return errors.New("设备直连不能使用默认路由、回环或组播地址")
		}
		devices = appendUniqueStrings(devices, prefix.Masked().String())
	}
	options.DirectDevices = devices
	return nil
}

func (svc *SimpleSetupService) Options() (*model.SimpleSetupOptions, error) {
	svc.mutations.RLock()
	defer svc.mutations.RUnlock()
	return svc.store.SimpleSetupOptions()
}

func (svc *SimpleSetupService) UpdateOptions(options *model.SimpleSetupOptions) error {
	if err := validateSimpleSetupOptions(options); err != nil {
		return err
	}
	svc.mutations.Lock()
	defer svc.mutations.Unlock()
	svc.mu.Lock()
	busy := svc.closed || svc.state.Status == "running"
	svc.mu.Unlock()
	if busy {
		return ErrSimpleSetupBusy
	}
	status, err := svc.Status()
	if err != nil {
		return err
	}
	if !status.Supported || status.HasExistingConfig {
		return errors.New("当前配置请在专业模式管理")
	}
	previous, err := svc.store.SimpleSetupOptions()
	if err != nil {
		return err
	}
	if reflect.DeepEqual(previous, options) {
		return nil
	}
	if status.OptionsLocked {
		return errors.New("配置已应用，请先重试完成启动，再调整选项")
	}
	release := svc.store.HoldConfigSnapshot()
	defer release()
	if !status.Configured {
		if err := svc.store.SaveSimpleSetupOptions(options); err != nil {
			return err
		}
		logging.Info("setup.options", "简易配置选项已保存，等待一键配置")
		return nil
	}
	svc.generator.configMu.Lock()
	defer svc.generator.configMu.Unlock()
	defer svc.generator.discardGeneratedConfig()
	activePath, exists, err := svc.paths.ActiveConfigPath()
	if err != nil || !exists {
		return errors.New("活动配置不存在，请重新读取状态")
	}
	backup, err := os.CreateTemp(svc.paths.ConfigDir, ".simple-options-*.tmp")
	if err != nil {
		return err
	}
	backupPath := backup.Name()
	defer os.Remove(backupPath)
	if err := backup.Close(); err != nil {
		return err
	}
	if err := copyFile(activePath, backupPath); err != nil {
		return err
	}
	if err := svc.store.SaveSimpleSetupOptions(options); err != nil {
		return err
	}
	wasRunning := svc.core != nil && svc.core.IsRunning()
	result, applyErr := svc.generator.generateCurrentLocked()
	if applyErr == nil && (result == nil || !result.Valid) {
		applyErr = errors.New("新配置未通过核心校验，请检查 DNS 和分流设置")
	}
	if applyErr == nil {
		applyErr = svc.generator.applyLocked("")
	}
	if applyErr == nil && svc.core != nil {
		_, applyErr = svc.core.ApplyConfigRuntime(false)
	}
	if applyErr != nil {
		logging.Error("setup.options", "简易配置应用失败，正在恢复原配置")
		rollbackErr := svc.store.SaveSimpleSetupOptions(previous)
		rollbackErr = errors.Join(rollbackErr, atomicReplaceFile(backupPath, activePath))
		if rollbackErr == nil && wasRunning {
			_, rollbackErr = svc.core.ApplyConfigRuntime(true)
		}
		if rollbackErr != nil {
			return errors.New("新配置应用失败，原配置未能完整恢复，请在专业模式查看日志")
		}
		return errors.New("新配置应用失败，已恢复原配置，请检查设置或在专业模式查看生成结果")
	}
	logging.Info("setup.options", "简易配置已校验并应用，核心运行: %t", wasRunning)
	return nil
}
