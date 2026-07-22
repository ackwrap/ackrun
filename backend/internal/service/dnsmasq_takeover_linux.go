//go:build linux

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/paths"
)

const (
	openWrtReleasePath = "/etc/openwrt_release"
	openWrtUCIPath     = "/sbin/uci"
	dnsmasqInitPath    = "/etc/init.d/dnsmasq"
	dnsmasqCommandTime = 15 * time.Second
)

type dnsmasqCommandRunner func(stdin, path string, args ...string) ([]byte, error)

type openWrtDNSMasqLifecycle struct {
	statePath   string
	releasePath string
	uciPath     string
	initPath    string
	run         dnsmasqCommandRunner
}

func platformSupportsDNSMasqTakeover() bool {
	_, err := os.Stat(openWrtReleasePath)
	return err == nil
}

func newPlatformDNSMasqLifecycle(p *paths.Paths) dnsmasqLifecycle {
	if p == nil {
		return noopDNSMasqLifecycle{}
	}
	return &openWrtDNSMasqLifecycle{
		statePath:   p.DNSMasqTakeoverStatePath(),
		releasePath: openWrtReleasePath,
		uciPath:     openWrtUCIPath,
		initPath:    dnsmasqInitPath,
		run:         runDNSMasqCommand,
	}
}

func runDNSMasqCommand(stdin, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dnsmasqCommandTime)
	defer cancel()
	command := exec.CommandContext(ctx, path, args...)
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return output, ctx.Err()
	}
	return output, err
}

func (manager *openWrtDNSMasqLifecycle) supported() (bool, error) {
	if _, err := os.Stat(manager.releasePath); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	for name, path := range map[string]string{"uci": manager.uciPath, "dnsmasq init": manager.initPath} {
		if info, err := os.Stat(path); err != nil {
			return false, fmt.Errorf("检查 %s 失败: %w", name, err)
		} else if info.IsDir() {
			return false, fmt.Errorf("%s 路径无效", name)
		}
	}
	return true, nil
}

func (manager *openWrtDNSMasqLifecycle) Activate() error {
	supported, err := manager.supported()
	if err != nil || !supported {
		return err
	}
	if output, err := manager.run("", manager.initPath, "enabled"); err != nil {
		return fmt.Errorf("dnsmasq 服务未启用: %w", commandFailure(err, output))
	}
	if output, err := manager.run("", manager.uciPath, "-q", "changes", "dhcp"); err != nil {
		return fmt.Errorf("检查 DHCP 待提交修改失败: %w", commandFailure(err, output))
	} else if strings.TrimSpace(string(output)) != "" {
		return fmt.Errorf("DHCP 存在未提交的 UCI 修改，请先提交或撤销后再启动核心")
	}

	current, err := manager.currentSnapshot()
	if err != nil {
		return err
	}
	state, stateExists, err := readDNSMasqTakeoverState(manager.statePath)
	if err != nil {
		return fmt.Errorf("读取 dnsmasq 接管状态失败: %w", err)
	}
	if stateExists {
		switch {
		case dnsmasqSnapshotsEqual(current, state.Managed):
			if err := manager.restart(); err != nil {
				return err
			}
			state.Phase = "active"
			return writeDNSMasqTakeoverState(manager.statePath, state)
		case !dnsmasqSnapshotsEqual(current, state.Original):
			return fmt.Errorf("dnsmasq 配置在 Ackwrap 接管期间被外部修改，拒绝覆盖")
		}
	} else {
		state = dnsmasqTakeoverState{
			Version:  dnsmasqStateVersion,
			Phase:    "prepared",
			Original: current,
			Managed:  managedDNSMasqSnapshot(current.Section),
		}
		if err := writeDNSMasqTakeoverState(manager.statePath, state); err != nil {
			return fmt.Errorf("保存 dnsmasq 原始设置失败: %w", err)
		}
	}

	current, err = manager.currentSnapshot()
	if err != nil {
		return manager.rollbackActivation(state, err)
	}
	if !dnsmasqSnapshotsEqual(current, state.Original) {
		return manager.rollbackActivation(state, fmt.Errorf("dnsmasq 配置在接管前发生变化"))
	}
	if err := manager.applySnapshot(current, state.Managed); err != nil {
		return manager.rollbackActivation(state, fmt.Errorf("写入 dnsmasq 接管设置失败: %w", err))
	}
	current, err = manager.currentSnapshot()
	if err != nil || !dnsmasqSnapshotsEqual(current, state.Managed) {
		if err == nil {
			err = fmt.Errorf("写入后的配置不匹配")
		}
		return manager.rollbackActivation(state, fmt.Errorf("验证 dnsmasq 接管设置失败: %w", err))
	}
	if err := manager.restart(); err != nil {
		return manager.rollbackActivation(state, err)
	}
	state.Phase = "active"
	if err := writeDNSMasqTakeoverState(manager.statePath, state); err != nil {
		return manager.rollbackActivation(state, fmt.Errorf("确认 dnsmasq 接管状态失败: %w", err))
	}
	logging.Info("dnsmasq.takeover", "OpenWrt dnsmasq 已转发到 Ackwrap 本地 DNS")
	return nil
}

func (manager *openWrtDNSMasqLifecycle) Restore() (bool, error) {
	supported, err := manager.supported()
	if err != nil || !supported {
		return false, err
	}
	state, exists, err := readDNSMasqTakeoverState(manager.statePath)
	if err != nil {
		return false, fmt.Errorf("读取 dnsmasq 接管状态失败: %w", err)
	}
	current, err := manager.currentSnapshot()
	if err != nil {
		return false, err
	}
	if !exists {
		managed := managedDNSMasqSnapshot(current.Section)
		if !dnsmasqSnapshotsEqual(current, managed) {
			return false, nil
		}
		fallback := dnsmasqOptionsSnapshot{Section: current.Section}
		rollbackState := dnsmasqTakeoverState{Original: managed, Managed: fallback}
		if err := manager.applySnapshot(current, fallback); err != nil {
			return false, manager.rollbackActivation(rollbackState, fmt.Errorf("清理无状态 dnsmasq 接管失败: %w", err))
		}
		restored, readErr := manager.currentSnapshot()
		if readErr != nil {
			return false, manager.rollbackActivation(rollbackState, readErr)
		}
		if !dnsmasqSnapshotsEqual(restored, fallback) {
			return false, manager.rollbackActivation(rollbackState, errors.New("清理后的 dnsmasq 配置不匹配"))
		}
		if err := manager.restart(); err != nil {
			return false, manager.rollbackActivation(rollbackState, err)
		}
		logging.Info("dnsmasq.restore", "已清理缺少状态文件的 Ackwrap dnsmasq 接管")
		return true, nil
	}
	switch {
	case dnsmasqSnapshotsEqual(current, state.Original):
		if err := manager.restart(); err != nil {
			return false, err
		}
	case dnsmasqSnapshotsEqual(current, state.Managed):
		if err := manager.applySnapshot(current, state.Original); err != nil {
			return false, fmt.Errorf("恢复 dnsmasq 原始设置失败: %w", err)
		}
		restored, readErr := manager.currentSnapshot()
		if readErr != nil {
			return false, readErr
		}
		if !dnsmasqSnapshotsEqual(restored, state.Original) {
			return false, fmt.Errorf("恢复后的 dnsmasq 配置不匹配")
		}
		if err := manager.restart(); err != nil {
			return false, err
		}
	default:
		return false, fmt.Errorf("dnsmasq 配置在 Ackwrap 接管期间被外部修改，拒绝自动恢复")
	}
	if err := removeDNSMasqTakeoverState(manager.statePath); err != nil {
		return false, fmt.Errorf("删除 dnsmasq 接管状态失败: %w", err)
	}
	logging.Info("dnsmasq.restore", "OpenWrt dnsmasq 原始上游已恢复")
	return true, nil
}

func (manager *openWrtDNSMasqLifecycle) rollbackActivation(state dnsmasqTakeoverState, cause error) error {
	current, err := manager.currentSnapshot()
	if err != nil {
		return errors.Join(cause, fmt.Errorf("读取回滚前 dnsmasq 配置失败: %w", err))
	}
	if !dnsmasqSnapshotsEqual(current, state.Original) {
		if !dnsmasqSnapshotsEqual(current, state.Managed) {
			return errors.Join(cause, fmt.Errorf("dnsmasq 配置发生外部变化，无法安全回滚"))
		}
		if err := manager.applySnapshot(current, state.Original); err != nil {
			return errors.Join(cause, fmt.Errorf("回滚 dnsmasq 配置失败: %w", err))
		}
	}
	if err := manager.restart(); err != nil {
		return errors.Join(cause, fmt.Errorf("回滚后重启 dnsmasq 失败: %w", err))
	}
	if err := removeDNSMasqTakeoverState(manager.statePath); err != nil {
		return errors.Join(cause, fmt.Errorf("清理 dnsmasq 接管状态失败: %w", err))
	}
	return cause
}

func (manager *openWrtDNSMasqLifecycle) currentSnapshot() (dnsmasqOptionsSnapshot, error) {
	output, err := manager.run("", manager.uciPath, "-q", "-X", "show", "dhcp")
	if err != nil {
		return dnsmasqOptionsSnapshot{}, fmt.Errorf("读取 DHCP 配置失败: %w", commandFailure(err, output))
	}
	snapshot, err := parseDNSMasqSnapshot(string(output))
	if err != nil {
		return dnsmasqOptionsSnapshot{}, err
	}
	return snapshot, nil
}

func (manager *openWrtDNSMasqLifecycle) applySnapshot(current, target dnsmasqOptionsSnapshot) error {
	deltaDir, err := os.MkdirTemp("/tmp", "ackwrap-uci-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(deltaDir)
	runUCI := func(args ...string) error {
		base := []string{"-q", "-t", deltaDir}
		output, err := manager.run("", manager.uciPath, append(base, args...)...)
		if err != nil {
			return commandFailure(err, output)
		}
		return nil
	}
	serverOption := "dhcp." + target.Section + ".server"
	if current.ServersExist {
		if err := runUCI("delete", serverOption); err != nil {
			return err
		}
	}
	if target.ServersExist {
		for _, server := range target.Servers {
			if err := runUCI("add_list", serverOption+"="+server); err != nil {
				return err
			}
		}
	}
	noResolvOption := "dhcp." + target.Section + ".noresolv"
	if target.NoResolvExists {
		if err := runUCI("set", noResolvOption+"="+target.NoResolv); err != nil {
			return err
		}
	} else if current.NoResolvExists {
		if err := runUCI("delete", noResolvOption); err != nil {
			return err
		}
	}
	return runUCI("commit", "dhcp")
}

func (manager *openWrtDNSMasqLifecycle) restart() error {
	if output, err := manager.run("", manager.initPath, "restart"); err != nil {
		return fmt.Errorf("重启 dnsmasq 失败: %w", commandFailure(err, output))
	}
	if output, err := manager.run("", manager.initPath, "running"); err != nil {
		return fmt.Errorf("dnsmasq 重启后未运行: %w", commandFailure(err, output))
	}
	return nil
}

func commandFailure(err error, output []byte) error {
	message := strings.TrimSpace(string(output))
	if message == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, message)
}
