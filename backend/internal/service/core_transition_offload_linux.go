//go:build linux

package service

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/paths"
)

const fw4CommandPath = "/sbin/fw4"

type fw4OffloadOptions struct {
	Section  string  `json:"section"`
	Software *string `json:"software"`
	Hardware *string `json:"hardware"`
}

type fw4OffloadRecord struct {
	Version  int               `json:"version"`
	Original fw4OffloadOptions `json:"original"`
}

// Changes only fw4's two offload options. The durable record survives an IPK
// replacing this daemon. No factory defaults or unrelated UCI deltas are saved.
type fw4OffloadPause struct {
	path      string
	supported func() bool
	run       dnsmasqCommandRunner
	nft       func(string, ...string) ([]byte, error)
	drained   func() error
}

func newFW4OffloadPause(p *paths.Paths, nft func(string, ...string) ([]byte, error)) *fw4OffloadPause {
	return &fw4OffloadPause{path: p.CoreTransitionOffloadPath(), run: runDNSMasqCommand, nft: nft,
		supported: func() bool {
			for _, path := range []string{openWrtReleasePath, openWrtUCIPath, fw4CommandPath, "/proc/net/nf_conntrack"} {
				if _, err := os.Stat(path); err != nil {
					return false
				}
			}
			return true
		}, drained: waitForFlowOffloadDrain}
}

func (m *fw4OffloadPause) pending() (bool, error) {
	_, exists, err := m.readRecord()
	return exists, err
}

func (m *fw4OffloadPause) readRecord() (fw4OffloadRecord, bool, error) {
	var record fw4OffloadRecord
	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return record, false, nil
	}
	if err != nil {
		return record, false, err
	}
	if json.Unmarshal(data, &record) != nil || record.Version != 1 || !validOffloadOptions(record.Original) {
		return record, false, errors.New("流量卸载恢复记录无效，拒绝覆盖防火墙配置")
	}
	return record, true, nil
}

var uciSectionName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func validOffloadOptions(options fw4OffloadOptions) bool {
	if !uciSectionName.MatchString(options.Section) {
		return false
	}
	for _, value := range []*string{options.Software, options.Hardware} {
		if value != nil {
			switch *value {
			case "0", "1", "true", "false", "yes", "no", "on", "off":
			default:
				return false
			}
		}
	}
	return true
}

func pausedOffloadOptions(original fw4OffloadOptions) fw4OffloadOptions {
	zero := "0"
	result := fw4OffloadOptions{Section: original.Section, Software: &zero}
	if original.Hardware != nil {
		result.Hardware = &zero
	}
	return result
}

func (m *fw4OffloadPause) current() (fw4OffloadOptions, error) {
	output, err := m.run("", openWrtUCIPath, "-q", "changes", "firewall")
	if err != nil || strings.TrimSpace(string(output)) != "" {
		return fw4OffloadOptions{}, errors.New("防火墙存在待提交修改或无法读取，未自动修改流量卸载")
	}
	output, err = m.run("", openWrtUCIPath, "-q", "-X", "show", "firewall")
	if err != nil {
		return fw4OffloadOptions{}, fmt.Errorf("读取防火墙卸载配置失败: %w", err)
	}
	return parseFW4OffloadOptions(string(output))
}

func parseFW4OffloadOptions(output string) (fw4OffloadOptions, error) {
	var result fw4OffloadOptions
	for line := range strings.SplitSeq(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		parts := strings.Split(key, ".")
		if ok && len(parts) == 2 && parts[0] == "firewall" && value == "defaults" {
			if result.Section != "" {
				return result, errors.New("防火墙含多个 defaults 段，无法自动接管卸载")
			}
			result.Section = parts[1]
		}
	}
	for line := range strings.SplitSeq(output, "\n") {
		key, raw, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		var target **string
		switch key {
		case "firewall." + result.Section + ".flow_offloading":
			target = &result.Software
		case "firewall." + result.Section + ".flow_offloading_hw":
			target = &result.Hardware
		default:
			continue
		}
		words, err := parseUCIWords(raw)
		if err != nil || len(words) != 1 || *target != nil {
			return result, errors.New("防火墙流量卸载选项无效")
		}
		*target = &words[0]
	}
	if !validOffloadOptions(result) {
		return result, errors.New("无法识别标准 fw4 卸载设置")
	}
	return result, nil
}

// Stage our deltas privately, so an interrupted preparation leaves no shared
// pending edits. UCI still reads its default delta path: recheck that it is clean
// immediately before commit as well as before staging. Never replace sections.
func (m *fw4OffloadPause) apply(current, target fw4OffloadOptions) error {
	actual, err := m.current()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, current) {
		return errors.New("流量卸载配置在操作前发生变化，拒绝覆盖")
	}
	dir, err := os.MkdirTemp("", "ackwrap-offload-uci-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for _, option := range []struct {
		name     string
		from, to *string
	}{{"flow_offloading", current.Software, target.Software}, {"flow_offloading_hw", current.Hardware, target.Hardware}} {
		if reflect.DeepEqual(option.from, option.to) {
			continue
		}
		args := []string{"-q", "-t", dir, "delete", "firewall." + target.Section + "." + option.name}
		if option.to != nil {
			args[3] = "set"
			args[4] += "=" + *option.to
		}
		if _, err := m.run("", openWrtUCIPath, args...); err != nil {
			return fmt.Errorf("写入流量卸载选项失败: %w", err)
		}
	}
	actual, err = m.current()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, current) {
		return errors.New("流量卸载配置在提交前发生变化，拒绝覆盖")
	}
	if _, err := m.run("", openWrtUCIPath, "-q", "-t", dir, "commit", "firewall"); err != nil {
		return fmt.Errorf("提交流量卸载选项失败: %w", err)
	}
	actual, err = m.current()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, target) {
		return errors.New("写入后的流量卸载设置与预期不一致")
	}
	return nil
}

// Only the fw4-owned table is supported. A foreign flowtable must never cause
// this manager to disable unrelated settings and then claim complete coverage.
func standardFW4Flowtables(data []byte) bool {
	var state struct {
		NFTables []map[string]json.RawMessage `json:"nftables"`
	}
	if json.Unmarshal(data, &state) != nil || state.NFTables == nil {
		return false
	}
	for _, object := range state.NFTables {
		if raw, ok := object["flowtable"]; ok {
			var f *struct{ Family, Table, Name, Hook string }
			if json.Unmarshal(raw, &f) != nil || f == nil || f.Family != "inet" || f.Table != "fw4" || f.Name != "ft" || f.Hook != "ingress" {
				return false
			}
		}
		if len(object) == 0 {
			return false
		}
	}
	return true
}

func (m *fw4OffloadPause) pause(ruleset []byte) error {
	record, exists, err := m.readRecord()
	if err != nil {
		return err
	}
	unsupported := func(reason string) error {
		if exists {
			return fmt.Errorf("存在未完成的流量卸载操作: %s", reason)
		}
		return fmt.Errorf("%w: %s", errTransitionProtectionUnsupported, reason)
	}
	if !m.supported() || !standardFW4Flowtables(ruleset) {
		return unsupported("仅自动处理具备连接状态检查能力的标准 fw4 流量卸载")
	}
	current, err := m.current()
	if err != nil {
		if !exists {
			return unsupported(err.Error())
		}
		return err
	}
	if !exists {
		if _, err := m.run("", fw4CommandPath, "-q", "check"); err != nil {
			return unsupported("fw4 预检查未通过")
		}
		record = fw4OffloadRecord{Version: 1, Original: current}
		data, _ := json.Marshal(record)
		if err := writeProtectionFile(m.path, data); err != nil {
			return err
		}
	}
	managed := pausedOffloadOptions(record.Original)
	if !reflect.DeepEqual(current, managed) {
		if !reflect.DeepEqual(current, record.Original) {
			return errors.New("流量卸载配置被外部修改，拒绝覆盖")
		}
		if err := m.apply(current, managed); err != nil {
			return err
		}
	}
	// fw4 reload atomically rebuilds its own table and deletes its old flowtable.
	// It does not flush the complete ruleset or discard Ackwrap's guard table.
	if _, err := m.run("", fw4CommandPath, "-q", "reload"); err != nil {
		return fmt.Errorf("暂停卸载后重载 fw4 失败，已保留恢复记录: %w", err)
	}
	data, err := m.nft("", "-j", "list", "ruleset")
	if err != nil {
		return err
	}
	if err := checkNFTFlowtables(data, nil); err != nil {
		return fmt.Errorf("暂停卸载后仍有快速转发路径: %w", err)
	}
	if err := m.drained(); err != nil {
		return err
	}
	logging.Info("core.flow_offload", "已临时暂停 fw4 流量卸载并确认旧卸载连接清除")
	return nil
}

func (m *fw4OffloadPause) restore() error {
	record, exists, err := m.readRecord()
	if err != nil || !exists {
		return err
	}
	current, err := m.current()
	if err != nil {
		return err
	}
	managed := pausedOffloadOptions(record.Original)
	if !reflect.DeepEqual(current, managed) && !reflect.DeepEqual(current, record.Original) {
		return errors.New("暂停期间流量卸载配置被外部修改，保留恢复记录且不覆盖用户设置")
	}
	data, err := m.nft("", "-j", "list", "ruleset")
	if err != nil {
		return err
	}
	var state struct {
		NFTables []struct {
			Table *struct{ Family, Name string } `json:"table"`
		} `json:"nftables"`
	}
	if json.Unmarshal(data, &state) != nil || state.NFTables == nil {
		return errors.New("恢复卸载前无法确认防护状态")
	}
	for _, entry := range state.NFTables {
		if table := entry.Table; table != nil && table.Family == "inet" && (table.Name == deviceProtectionTable || table.Name == deviceDNSTable || table.Name == transitionGuardTable) {
			if !reflect.DeepEqual(current, managed) {
				return m.pause(data)
			}
			if err := checkNFTFlowtables(data, nil); err != nil {
				return fmt.Errorf("设备保护仍需暂停卸载，但快速路径已重新出现: %w", err)
			}
			logging.Info("core.flow_offload", "仍有设备或切换保护，保持暂停卸载并保留恢复记录")
			return nil
		}
	}
	if !reflect.DeepEqual(current, record.Original) {
		if err := m.apply(current, record.Original); err != nil {
			return err
		}
	}
	if _, err := m.run("", fw4CommandPath, "-q", "reload"); err != nil {
		return fmt.Errorf("恢复卸载设置后重载 fw4 失败，保留重试记录: %w", err)
	}
	actual, err := m.current()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, record.Original) {
		return errors.New("恢复卸载期间配置发生变化，保留恢复记录")
	}
	if err := os.Remove(m.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	logging.Info("core.flow_offload", "核心已就绪，已恢复原始 fw4 流量卸载设置")
	return nil
}

func waitForFlowOffloadDrain() error {
	deadline := time.Now().Add(3 * time.Second)
	for {
		pending, err := hasOffloadedConnections("/proc/net/nf_conntrack")
		if err != nil {
			return fmt.Errorf("无法确认旧卸载连接已清除: %w", err)
		}
		if !pending {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("旧流量卸载连接尚未清除，保留暂停状态，核心未停止")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func hasOffloadedConnections(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "[OFFLOAD]") || strings.Contains(scanner.Text(), "[HW_OFFLOAD]") {
			return true, nil
		}
	}
	return false, scanner.Err()
}
