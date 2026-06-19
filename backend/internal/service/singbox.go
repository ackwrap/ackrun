package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

type SingboxService struct {
	paths     *paths.Paths
	realtime  *RealtimeService
	store     *store.Store
	cmd       *exec.Cmd
	pid       int
	mu        sync.Mutex
	cancel    context.CancelFunc
	cachedVer string
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func NewSingboxService(p *paths.Paths, rt *RealtimeService, s *store.Store) *SingboxService {
	return &SingboxService{paths: p, realtime: rt, store: s}
}

func (svc *SingboxService) Start() (*model.ActionResponse, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	if svc.isRunning() {
		return nil, fmt.Errorf("sing-box is already running (pid=%d)", svc.pid)
	}

	if _, err := os.Stat(svc.paths.BinaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("sing-box binary not found")
	}

	configPath, ok, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("config file not found")
	}

	logging.Info("core.start", "starting sing-box")
	svc.realtime.Broadcast("core.status", map[string]any{
		"status": "starting",
		"pid":    0,
		"error":  "",
	})

	ctx, cancel := context.WithCancel(context.Background())
	svc.cancel = cancel

	cmd := exec.CommandContext(ctx, svc.paths.BinaryPath, "run", "-c", configPath)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		cancel()
		svc.cancel = nil
		svc.realtime.Broadcast("core.status", map[string]any{
			"status": "error",
			"pid":    0,
			"error":  err.Error(),
		})
		return nil, fmt.Errorf("start sing-box: %w", err)
	}

	svc.cmd = cmd
	svc.pid = cmd.Process.Pid

	logging.Info("core.start", "sing-box started, pid=%d", svc.pid)
	svc.realtime.Broadcast("core.status", map[string]any{
		"status": "running",
		"pid":    svc.pid,
	})
	svc.broadcastRuntimeStatus(model.RuntimeRunning, svc.pid)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				svc.realtime.Broadcast("core.log", map[string]any{
					"line": cleanLogLine(string(buf[:n])),
				})
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				svc.realtime.Broadcast("core.log", map[string]any{
					"line": cleanLogLine(string(buf[:n])),
				})
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		err := cmd.Wait()
		svc.mu.Lock()
		svc.pid = 0
		svc.cmd = nil
		svc.cancel = nil
		svc.mu.Unlock()

		statusMsg := "stopped"
		if err != nil {
			statusMsg = "error"
		}
		logging.Info("core.start", "sing-box exited: %v", err)
		svc.realtime.Broadcast("core.status", map[string]any{
			"status": statusMsg,
			"pid":    0,
			"error":  "",
		})
		svc.broadcastRuntimeStatus(model.RuntimeStopped, 0)
	}()

	return &model.ActionResponse{Success: true, Message: "service started"}, nil
}

func (svc *SingboxService) Stop() (*model.ActionResponse, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	if !svc.isRunning() {
		return nil, fmt.Errorf("sing-box is not running")
	}

	logging.Info("core.stop", "stopping sing-box, pid=%d", svc.pid)

	if svc.cancel != nil {
		svc.cancel()
	}

	svc.realtime.Broadcast("core.status", map[string]any{
		"status": "stopping",
		"pid":    svc.pid,
	})

	time.AfterFunc(10*time.Second, func() {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		if svc.isRunning() {
			logging.Info("core.stop", "force killing sing-box, pid=%d", svc.pid)
			cmd := svc.cmd
			if cmd != nil && cmd.Process != nil {
				cmd.Process.Kill()
			}
		}
	})

	return &model.ActionResponse{Success: true, Message: "service stopped"}, nil
}

func (svc *SingboxService) Restart() (*model.ActionResponse, error) {
	if _, err := svc.Stop(); err != nil {
		return nil, err
	}

	time.Sleep(500 * time.Millisecond)

	return svc.Start()
}

func (svc *SingboxService) ReloadConfig() (*model.ActionResponse, error) {
	logging.Info("core.reload_config", "reloading sing-box config")
	if !svc.isRunning() {
		return &model.ActionResponse{Success: true, Message: "core is stopped; config will be used on next start"}, nil
	}
	return svc.Restart()
}

func (svc *SingboxService) isRunning() bool {
	return svc.pid > 0 && svc.cmd != nil && svc.cmd.Process != nil
}

func (svc *SingboxService) GetPID() int {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.isRunning() {
		return svc.pid
	}
	return 0
}

func (svc *SingboxService) broadcastRuntimeStatus(status model.RuntimeStatus, pid int) {
	version := ""
	if st, err := svc.store.GetInstallState(); err == nil && isSingboxVersion(st.Version) {
		version = st.Version
	} else if svc.cachedVer != "" {
		version = svc.cachedVer
	} else if v := svc.getVersion(); v != "" {
		version = v
		svc.cachedVer = v
	}
	svc.realtime.Broadcast("runtime.status", model.RuntimeResponse{Status: status, PID: pid, Version: version})
}

func (svc *SingboxService) getVersion() string {
	return readSingboxVersion(svc.paths.BinaryPath)
}

func cleanLogLine(line string) string {
	return ansiEscapePattern.ReplaceAllString(line, "")
}

// CloseConnections closes all active connections by restarting the service
func (svc *SingboxService) CloseConnections() (*model.ActionResponse, error) {
	logging.Info("core.close_connections", "closing all connections")

	if !svc.isRunning() {
		return nil, fmt.Errorf("sing-box is not running")
	}

	// Restart to close all connections
	_, err := svc.Restart()
	if err != nil {
		return nil, fmt.Errorf("failed to close connections: %w", err)
	}

	return &model.ActionResponse{Success: true, Message: "all connections closed"}, nil
}

// ResetFirewall resets firewall rules (platform-specific implementation)
func (svc *SingboxService) ResetFirewall() (*model.ActionResponse, error) {
	logging.Info("core.reset_firewall", "resetting firewall rules")

	// For Windows
	cmd := exec.Command("netsh", "advfirewall", "reset")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.Info("core.reset_firewall", "failed: %v, output: %s", err, string(output))
		return nil, fmt.Errorf("failed to reset firewall: %w", err)
	}

	logging.Info("core.reset_firewall", "firewall reset successful")
	return &model.ActionResponse{Success: true, Message: "firewall rules reset"}, nil
}

// FlushDNS clears the DNS cache (platform-specific implementation)
func (svc *SingboxService) FlushDNS() (*model.ActionResponse, error) {
	logging.Info("core.flush_dns", "flushing DNS cache")

	// For Windows
	cmd := exec.Command("ipconfig", "/flushdns")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.Info("core.flush_dns", "failed: %v, output: %s", err, string(output))
		return nil, fmt.Errorf("failed to flush DNS cache: %w", err)
	}

	logging.Info("core.flush_dns", "DNS cache flushed successfully")
	return &model.ActionResponse{Success: true, Message: "DNS cache flushed"}, nil
}

// CheckUpdate checks for available updates
func (svc *SingboxService) CheckUpdate() (*model.ActionResponse, error) {
	logging.Info("core.check_update", "checking for updates")

	currentVersion := svc.getVersion()
	if currentVersion == "" {
		return nil, fmt.Errorf("failed to get current version")
	}

	// TODO: Implement actual version checking logic
	// For now, just return current version info
	message := fmt.Sprintf("current version: %s (update check not yet implemented)", currentVersion)

	return &model.ActionResponse{Success: true, Message: message}, nil
}
