package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const (
	sshMonitorTimeout       = 5 * time.Second
	sshMonitorMaximumOutput = 64 << 10
	sshMonitorMaximumDisks  = 12
)

const sshMonitorCommand = `LC_ALL=C
export LC_ALL
printf 'ACKWRAP_MONITOR_V1\n'
hostname 2>/dev/null | awk 'NR == 1 { print "HOST\t" $0 }'
id -un 2>/dev/null | awk 'NR == 1 { print "USER\t" $0 }'
awk '/^cpu / { total = 0; for (i = 2; i <= NF; i++) total += $i; printf "CPU\t%.0f\t%.0f\n", total, $5 + $6; exit }' /proc/stat 2>/dev/null
awk '/^MemTotal:/ { total = $2 } /^MemAvailable:/ { available = $2 } /^MemFree:/ { free = $2 } /^Buffers:/ { buffers = $2 } /^Cached:/ { cached = $2 } END { if (!available) available = free + buffers + cached; printf "MEM\t%.0f\t%.0f\n", total * 1024, available * 1024 }' /proc/meminfo 2>/dev/null
awk -F '[: ]+' 'NR > 2 && $2 != "lo" { received += $3; transmitted += $11 } END { printf "NET\t%.0f\t%.0f\n", received, transmitted }' /proc/net/dev 2>/dev/null
awk 'NR == 1 { printf "UPTIME\t%.0f\n", $1 }' /proc/uptime 2>/dev/null
who 2>/dev/null | awk 'END { print "LOGINS\t" NR }'
df -Pk 2>/dev/null | awk 'NR > 1 && count < 12 { for (i = 2; i + 4 <= NF; i++) { if ($i ~ /^[0-9]+$/ && $(i + 1) ~ /^[0-9]+$/ && $(i + 2) ~ /^[0-9]+$/ && $(i + 3) ~ /^[0-9]+%$/) { mount = $(i + 4); for (j = i + 5; j <= NF; j++) mount = mount " " $j; printf "DISK\t-\t%.0f\t%.0f\t%.0f\t%s\n", $i * 1024, $(i + 1) * 1024, $(i + 2) * 1024, mount; count++; break } } }'
printf 'END\n'
`

const sshDeviceDetailsCommand = `LC_ALL=C
export LC_ALL
printf 'ACKWRAP_DETAILS_V1\n'
awk -F: '/^(model name|Hardware)[ \t]*:/ { value = $2; sub(/^[ \t]+/, "", value); print "CPU_MODEL\t" value; exit }' /proc/cpuinfo 2>/dev/null
awk '/^processor[ \t]*:/ { cores++ } END { print "CPU_CORES\t" (cores ? cores : 1) }' /proc/cpuinfo 2>/dev/null
awk '/^MemTotal:/ { total = $2 } /^MemAvailable:/ { available = $2 } /^MemFree:/ { free = $2 } /^Buffers:/ { buffers = $2 } /^Cached:/ { cached = $2 } /^SwapTotal:/ { swap_total = $2 } /^SwapFree:/ { swap_free = $2 } END { if (!available) available = free + buffers + cached; printf "MEM\t%.0f\t%.0f\t%.0f\t%.0f\n", total * 1024, available * 1024, swap_total * 1024, swap_free * 1024 }' /proc/meminfo 2>/dev/null
awk 'NR == 1 { printf "UPTIME\t%.0f\n", $1 }' /proc/uptime 2>/dev/null
awk 'NR == 1 { printf "LOAD\t%s\t%s\t%s\n", $1, $2, $3 }' /proc/loadavg 2>/dev/null
if command -v docker >/dev/null 2>&1; then
  docker_version="$(docker --version 2>/dev/null | head -n 1)"
  printf 'SOFTWARE\tdocker\t1\t%s\n' "$docker_version"
else
  printf 'SOFTWARE\tdocker\t0\t\n'
fi
compose_version=""
if command -v docker >/dev/null 2>&1; then compose_version="$(docker compose version 2>/dev/null | head -n 1)"; fi
if test -z "$compose_version" && command -v docker-compose >/dev/null 2>&1; then compose_version="$(docker-compose --version 2>/dev/null | head -n 1)"; fi
if test -n "$compose_version"; then
  printf 'SOFTWARE\tdocker-compose\t1\t%s\n' "$compose_version"
else
  printf 'SOFTWARE\tdocker-compose\t0\t\n'
fi
printf 'END\n'
`

type sshMonitorCommandResult struct {
	output   []byte
	overflow bool
	err      error
}

type sshMonitorOutput struct {
	buffer   bytes.Buffer
	overflow bool
}

func (output *sshMonitorOutput) Write(content []byte) (int, error) {
	written := len(content)
	remaining := sshMonitorMaximumOutput - output.buffer.Len()
	if remaining <= 0 {
		output.overflow = true
		return written, nil
	}
	if len(content) > remaining {
		content = content[:remaining]
		output.overflow = true
	}
	_, _ = output.buffer.Write(content)
	return written, nil
}

func (svc *SSHHostService) GetSessionMonitor(ctx context.Context, sessionID, token string) (*model.SSHMonitorSnapshot, error) {
	output, managed, err := svc.collectSessionOutput(ctx, sessionID, token, sshMonitorCommand, "ssh_monitor.collect")
	if err != nil {
		return nil, err
	}
	snapshot, err := parseSSHMonitorOutput(output, svc.now())
	if err != nil {
		logging.Error("ssh_monitor.collect", "解析 SSH 主机监控失败: host_id=%d code=SSH_MONITOR_UNSUPPORTED", managed.host.ID)
		return nil, sshError("SSH_MONITOR_UNSUPPORTED", "远端主机不支持 Linux 监控数据采集", err)
	}
	return snapshot, nil
}

func (svc *SSHHostService) GetSessionDetails(ctx context.Context, sessionID, token string) (*model.SSHDeviceDetails, error) {
	output, managed, err := svc.collectSessionOutput(ctx, sessionID, token, sshDeviceDetailsCommand, "ssh_device.details")
	if err != nil {
		return nil, err
	}
	details, err := parseSSHDeviceDetailsOutput(output, svc.now())
	if err != nil {
		logging.Error("ssh_device.details", "解析 SSH 设备详情失败: host_id=%d code=SSH_MONITOR_UNSUPPORTED", managed.host.ID)
		return nil, sshError("SSH_MONITOR_UNSUPPORTED", "远端主机不支持设备详情采集", err)
	}
	logging.Info("ssh_device.details", "读取 SSH 设备详情: host_id=%d", managed.host.ID)
	return details, nil
}

func (svc *SSHHostService) collectSessionOutput(ctx context.Context, sessionID, token, command, action string) ([]byte, *managedSSHSession, error) {
	managed, client, err := svc.monitorClient(sessionID, token)
	if err != nil {
		return nil, nil, err
	}
	if !managed.monitorMu.TryLock() {
		managed.sftpOps.Done()
		return nil, managed, sshError("SSH_MONITOR_BUSY", "远端主机数据采集正在进行", nil)
	}
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			managed.monitorMu.Unlock()
			managed.sftpOps.Done()
		})
	}
	releaseDeferred := false
	defer func() {
		if !releaseDeferred {
			release()
		}
	}()

	commandCtx, cancel := context.WithTimeout(ctx, sshMonitorTimeout)
	defer cancel()
	output, commandCompletion, err := runSSHMonitorCommand(commandCtx, client, command)
	if commandCompletion != nil {
		releaseDeferred = true
		go func() {
			<-commandCompletion
			release()
		}()
	}
	if err != nil {
		code, message := "SSH_MONITOR_UNAVAILABLE", "读取远端主机监控数据失败"
		if errors.Is(err, context.DeadlineExceeded) {
			code, message = "SSH_MONITOR_TIMEOUT", "读取远端主机监控数据超时"
		}
		logging.Error(action, "读取 SSH 远端数据失败: host_id=%d code=%s", managed.host.ID, code)
		return nil, managed, sshError(code, message, err)
	}
	managed.lastActive.Store(svc.now().UnixMilli())
	return output, managed, nil
}

func (svc *SSHHostService) monitorClient(sessionID, token string) (*managedSSHSession, *ssh.Client, error) {
	if sessionID == "" || token == "" {
		return nil, nil, sshError("SSH_SESSION_NOT_FOUND", "SSH 会话不存在或授权无效", nil)
	}
	svc.sessionMu.Lock()
	managed := svc.sessions[sessionID]
	svc.sessionMu.Unlock()
	if managed == nil {
		return nil, nil, sshError("SSH_SESSION_NOT_FOUND", "SSH 会话不存在或授权无效", nil)
	}
	tokenHash := sha256.Sum256([]byte(token))
	managed.mu.Lock()
	defer managed.mu.Unlock()
	if managed.terminated || subtle.ConstantTimeCompare(managed.sftpToken[:], tokenHash[:]) != 1 {
		return nil, nil, sshError("SSH_SESSION_NOT_FOUND", "SSH 会话不存在或授权无效", nil)
	}
	managed.sftpOps.Add(1)
	return managed, managed.client, nil
}

func runSSHMonitorCommand(ctx context.Context, client *ssh.Client, command string) ([]byte, <-chan struct{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, nil, err
	}
	defer session.Close()
	output := &sshMonitorOutput{}
	session.Stdout = output
	session.Stderr = io.Discard
	result := make(chan sshMonitorCommandResult, 1)
	go func() {
		commandErr := session.Run(command)
		result <- sshMonitorCommandResult{
			output: output.buffer.Bytes(), overflow: output.overflow, err: commandErr,
		}
	}()
	select {
	case completed := <-result:
		if completed.err != nil {
			return nil, nil, completed.err
		}
		if completed.overflow {
			return nil, nil, errors.New("SSH monitor output is too large")
		}
		return completed.output, nil, nil
	case <-ctx.Done():
		_ = session.Close()
		completion := make(chan struct{})
		go func() {
			<-result
			close(completion)
		}()
		return nil, completion, ctx.Err()
	}
}

func parseSSHMonitorOutput(output []byte, collectedAt time.Time) (*model.SSHMonitorSnapshot, error) {
	if len(output) == 0 || len(output) > sshMonitorMaximumOutput {
		return nil, errors.New("SSH monitor output has invalid size")
	}
	snapshot := &model.SSHMonitorSnapshot{Disks: make([]model.SSHMonitorDisk, 0)}
	seenMounts := make(map[string]bool)
	seenHeader, seenEnd, seenCPU, seenMemory, seenNetwork, seenUptime := false, false, false, false, false, false
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Buffer(make([]byte, 1024), 16<<10)
	for scanner.Scan() {
		line := scanner.Text()
		if !seenHeader {
			if line != "ACKWRAP_MONITOR_V1" {
				return nil, errors.New("SSH monitor output header is invalid")
			}
			seenHeader = true
			continue
		}
		if line == "END" {
			seenEnd = true
			break
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "HOST":
			snapshot.Hostname = monitorText(fields[1])
		case "USER":
			snapshot.Username = monitorText(fields[1])
		case "CPU":
			if len(fields) != 3 {
				continue
			}
			total, totalErr := monitorInt(fields[1])
			idle, idleErr := monitorInt(fields[2])
			if totalErr == nil && idleErr == nil && total > 0 && idle <= total {
				snapshot.CPUTotal, snapshot.CPUIdle, seenCPU = total, idle, true
			}
		case "MEM":
			if len(fields) != 3 {
				continue
			}
			total, totalErr := monitorInt(fields[1])
			available, availableErr := monitorInt(fields[2])
			if totalErr == nil && availableErr == nil && total > 0 && available <= total {
				snapshot.MemoryTotalBytes, snapshot.MemoryAvailableBytes, seenMemory = total, available, true
			}
		case "NET":
			if len(fields) != 3 {
				continue
			}
			received, receivedErr := monitorInt(fields[1])
			transmitted, transmittedErr := monitorInt(fields[2])
			if receivedErr == nil && transmittedErr == nil {
				snapshot.NetworkReceivedBytes, snapshot.NetworkTransmittedBytes, seenNetwork = received, transmitted, true
			}
		case "UPTIME":
			if len(fields) == 2 {
				uptime, uptimeErr := monitorInt(fields[1])
				if uptimeErr == nil {
					snapshot.UptimeSeconds, seenUptime = uptime, true
				}
			}
		case "LOGINS":
			if len(fields) == 2 {
				if sessions, sessionsErr := monitorInt(fields[1]); sessionsErr == nil {
					snapshot.LoginSessions = sessions
				}
			}
		case "DISK":
			if len(fields) != 6 || len(snapshot.Disks) >= sshMonitorMaximumDisks {
				continue
			}
			total, totalErr := monitorInt(fields[2])
			used, usedErr := monitorInt(fields[3])
			available, availableErr := monitorInt(fields[4])
			mountPoint := monitorText(fields[5])
			if totalErr != nil || usedErr != nil || availableErr != nil || total <= 0 || mountPoint == "" || seenMounts[mountPoint] {
				continue
			}
			seenMounts[mountPoint] = true
			usage := int(float64(used)/float64(total)*100 + 0.5)
			if usage > 100 {
				usage = 100
			}
			snapshot.Disks = append(snapshot.Disks, model.SSHMonitorDisk{
				MountPoint: mountPoint, TotalBytes: total, UsedBytes: used,
				AvailableBytes: available, UsagePercent: usage,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !seenHeader || !seenEnd || !seenCPU || !seenMemory || !seenNetwork || !seenUptime {
		return nil, errors.New("SSH monitor output is incomplete")
	}
	snapshot.CollectedAt = collectedAt.UnixMilli()
	return snapshot, nil
}

func parseSSHDeviceDetailsOutput(output []byte, collectedAt time.Time) (*model.SSHDeviceDetails, error) {
	if len(output) == 0 || len(output) > sshMonitorMaximumOutput {
		return nil, errors.New("SSH device details output has invalid size")
	}
	details := &model.SSHDeviceDetails{Software: make([]model.SSHSoftware, 0)}
	seenSoftware := make(map[string]bool)
	seenHeader, seenEnd, seenCores, seenMemory, seenUptime, seenLoad := false, false, false, false, false, false
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Buffer(make([]byte, 1024), 16<<10)
	for scanner.Scan() {
		line := scanner.Text()
		if !seenHeader {
			if line != "ACKWRAP_DETAILS_V1" {
				return nil, errors.New("SSH device details output header is invalid")
			}
			seenHeader = true
			continue
		}
		if line == "END" {
			seenEnd = true
			break
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "CPU_MODEL":
			details.CPUModel = monitorText(fields[1])
		case "CPU_CORES":
			if len(fields) == 2 {
				if cores, coresErr := monitorInt(fields[1]); coresErr == nil && cores > 0 {
					details.CPUCores, seenCores = cores, true
				}
			}
		case "MEM":
			if len(fields) != 5 {
				continue
			}
			total, totalErr := monitorInt(fields[1])
			available, availableErr := monitorInt(fields[2])
			swapTotal, swapTotalErr := monitorInt(fields[3])
			swapAvailable, swapAvailableErr := monitorInt(fields[4])
			if totalErr == nil && availableErr == nil && swapTotalErr == nil && swapAvailableErr == nil && total > 0 && available <= total && swapAvailable <= swapTotal {
				details.MemoryTotalBytes, details.MemoryAvailableBytes = total, available
				details.SwapTotalBytes, details.SwapAvailableBytes = swapTotal, swapAvailable
				seenMemory = true
			}
		case "UPTIME":
			if len(fields) == 2 {
				if uptime, uptimeErr := monitorInt(fields[1]); uptimeErr == nil {
					details.UptimeSeconds, seenUptime = uptime, true
				}
			}
		case "LOAD":
			if len(fields) == 4 {
				one, oneErr := monitorFloat(fields[1])
				five, fiveErr := monitorFloat(fields[2])
				fifteen, fifteenErr := monitorFloat(fields[3])
				if oneErr == nil && fiveErr == nil && fifteenErr == nil {
					details.LoadAverage1, details.LoadAverage5, details.LoadAverage15 = one, five, fifteen
					seenLoad = true
				}
			}
		case "SOFTWARE":
			if len(fields) != 4 {
				continue
			}
			key := monitorText(fields[1])
			if key == "" || seenSoftware[key] || (fields[2] != "0" && fields[2] != "1") {
				continue
			}
			seenSoftware[key] = true
			details.Software = append(details.Software, model.SSHSoftware{
				Key: key, Installed: fields[2] == "1", Version: monitorText(fields[3]),
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !seenHeader || !seenEnd || !seenCores || !seenMemory || !seenUptime || !seenLoad {
		return nil, errors.New("SSH device details output is incomplete")
	}
	details.CollectedAt = collectedAt.UnixMilli()
	return details, nil
}

func monitorInt(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid monitor integer")
	}
	return parsed, nil
}

func monitorFloat(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed < 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("invalid monitor float")
	}
	return parsed, nil
}

func monitorText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 255 {
		value = value[:255]
	}
	return strings.Map(func(char rune) rune {
		if char < 0x20 || char == 0x7f {
			return -1
		}
		return char
	}, value)
}
