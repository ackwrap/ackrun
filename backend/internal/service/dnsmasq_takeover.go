package service

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/paths"
)

const (
	dnsInboundTag         = "ackwrap-dns-in"
	defaultDNSInboundPort = 1053
	dnsmasqStateVersion   = 1
)

type dnsmasqLifecycle interface {
	Activate() error
	Restore() (bool, error)
}

type noopDNSMasqLifecycle struct{}

func (noopDNSMasqLifecycle) Activate() error        { return nil }
func (noopDNSMasqLifecycle) Restore() (bool, error) { return false, nil }

type dnsmasqOptionsSnapshot struct {
	Section        string   `json:"section"`
	NoResolvExists bool     `json:"noresolv_exists"`
	NoResolv       string   `json:"noresolv,omitempty"`
	ServersExist   bool     `json:"servers_exist"`
	Servers        []string `json:"servers,omitempty"`
}

type dnsmasqTakeoverState struct {
	Version  int                    `json:"version"`
	Phase    string                 `json:"phase"`
	Original dnsmasqOptionsSnapshot `json:"original"`
	Managed  dnsmasqOptionsSnapshot `json:"managed"`
}

func managedDNSMasqSnapshot(section string) dnsmasqOptionsSnapshot {
	return dnsmasqOptionsSnapshot{
		Section:        section,
		NoResolvExists: true,
		NoResolv:       "1",
		ServersExist:   true,
		Servers:        []string{"127.0.0.1#1053"},
	}
}

func dnsmasqSnapshotsEqual(left, right dnsmasqOptionsSnapshot) bool {
	if left.Section != right.Section || left.NoResolvExists != right.NoResolvExists || left.ServersExist != right.ServersExist {
		return false
	}
	if left.NoResolvExists && left.NoResolv != right.NoResolv {
		return false
	}
	if left.ServersExist && !slicesEqual(left.Servers, right.Servers) {
		return false
	}
	return true
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func parseDNSMasqSnapshot(output string) (dnsmasqOptionsSnapshot, error) {
	type sectionData struct {
		disabled []string
		options  map[string][]string
	}
	sections := make(map[string]*sectionData)
	order := make([]string, 0)
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		left, raw, found := strings.Cut(line, "=")
		if !found {
			return dnsmasqOptionsSnapshot{}, fmt.Errorf("无法解析 UCI 输出")
		}
		parts := strings.Split(left, ".")
		if len(parts) == 2 && parts[0] == "dhcp" && raw == "dnsmasq" {
			sections[parts[1]] = &sectionData{options: make(map[string][]string)}
			order = append(order, parts[1])
			continue
		}
		if len(parts) != 3 || parts[0] != "dhcp" {
			continue
		}
		section := sections[parts[1]]
		if section == nil {
			continue
		}
		values, err := parseUCIWords(raw)
		if err != nil {
			return dnsmasqOptionsSnapshot{}, fmt.Errorf("解析 dnsmasq.%s 失败: %w", parts[2], err)
		}
		section.options[parts[2]] = values
		if parts[2] == "disabled" {
			section.disabled = values
		}
	}
	enabled := make([]string, 0, len(order))
	for _, name := range order {
		section := sections[name]
		if len(section.disabled) > 0 && section.disabled[0] == "1" {
			continue
		}
		enabled = append(enabled, name)
	}
	if len(enabled) == 0 {
		return dnsmasqOptionsSnapshot{}, fmt.Errorf("没有启用的 dnsmasq 实例")
	}
	if len(enabled) != 1 {
		return dnsmasqOptionsSnapshot{}, fmt.Errorf("检测到 %d 个启用的 dnsmasq 实例，无法安全自动接管", len(enabled))
	}
	name := enabled[0]
	options := sections[name].options
	noResolv, noResolvExists := options["noresolv"]
	servers, serversExist := options["server"]
	snapshot := dnsmasqOptionsSnapshot{
		Section:        name,
		NoResolvExists: noResolvExists,
		ServersExist:   serversExist,
		Servers:        append([]string(nil), servers...),
	}
	if noResolvExists {
		if len(noResolv) != 1 {
			return dnsmasqOptionsSnapshot{}, fmt.Errorf("dnsmasq noresolv 配置无效")
		}
		snapshot.NoResolv = noResolv[0]
	}
	return snapshot, nil
}

func parseUCIWords(value string) ([]string, error) {
	var words []string
	for index := 0; index < len(value); {
		for index < len(value) && (value[index] == ' ' || value[index] == '\t') {
			index++
		}
		if index == len(value) {
			break
		}
		var word strings.Builder
		for index < len(value) && value[index] != ' ' && value[index] != '\t' {
			switch value[index] {
			case '\'':
				index++
				for index < len(value) && value[index] != '\'' {
					word.WriteByte(value[index])
					index++
				}
				if index == len(value) {
					return nil, fmt.Errorf("引号未闭合")
				}
				index++
			case '\\':
				index++
				if index == len(value) {
					return nil, fmt.Errorf("转义符不完整")
				}
				word.WriteByte(value[index])
				index++
			default:
				word.WriteByte(value[index])
				index++
			}
		}
		words = append(words, word.String())
	}
	return words, nil
}

func readDNSMasqTakeoverState(path string) (dnsmasqTakeoverState, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return dnsmasqTakeoverState{}, false, nil
	}
	if err != nil {
		return dnsmasqTakeoverState{}, false, err
	}
	var state dnsmasqTakeoverState
	if err := json.Unmarshal(data, &state); err != nil {
		return dnsmasqTakeoverState{}, false, err
	}
	if state.Version != dnsmasqStateVersion || state.Original.Section == "" || state.Managed.Section != state.Original.Section {
		return dnsmasqTakeoverState{}, false, fmt.Errorf("dnsmasq 接管状态无效")
	}
	return state, true, nil
}

func writeDNSMasqTakeoverState(path string, state dnsmasqTakeoverState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	staged, err := os.CreateTemp(filepath.Dir(path), ".dnsmasq-state-*.tmp")
	if err != nil {
		return err
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if err := staged.Chmod(0600); err != nil {
		staged.Close()
		return err
	}
	if _, err := staged.Write(data); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Sync(); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	if err := atomicReplaceFile(stagedPath, path); err != nil {
		return err
	}
	return syncDNSMasqStateDirectory(path)
}

func removeDNSMasqTakeoverState(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDNSMasqStateDirectory(path)
}

func newDNSMasqLifecycle(p *paths.Paths) dnsmasqLifecycle {
	return newPlatformDNSMasqLifecycle(p)
}

func waitForDNSInbound(processExited <-chan struct{}, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case <-processExited:
			return fmt.Errorf("sing-box 在本地 DNS 就绪前退出")
		default:
		}
		if err := probeDNSInbound(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("等待本地 DNS 端口就绪超时: %w", lastErr)
}

func probeDNSInbound() error {
	connection, err := net.DialTimeout("udp", fmt.Sprintf("127.0.0.1:%d", defaultDNSInboundPort), 500*time.Millisecond)
	if err != nil {
		return err
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		return err
	}
	id := uint16(time.Now().UnixNano())
	query := make([]byte, 12, 29)
	binary.BigEndian.PutUint16(query[0:2], id)
	binary.BigEndian.PutUint16(query[2:4], 0x0100)
	binary.BigEndian.PutUint16(query[4:6], 1)
	for _, label := range []string{"example", "com"} {
		query = append(query, byte(len(label)))
		query = append(query, label...)
	}
	query = append(query, 0, 0, 1, 0, 1)
	if _, err := connection.Write(query); err != nil {
		return err
	}
	response := make([]byte, 4096)
	read, err := connection.Read(response)
	if err != nil {
		return err
	}
	if read < 12 || binary.BigEndian.Uint16(response[0:2]) != id || binary.BigEndian.Uint16(response[2:4])&0x8000 == 0 {
		return fmt.Errorf("本地 DNS 返回无效响应")
	}
	if rcode := binary.BigEndian.Uint16(response[2:4]) & 0x000f; rcode != 0 {
		return fmt.Errorf("本地 DNS 返回错误码 %d", rcode)
	}
	return nil
}
