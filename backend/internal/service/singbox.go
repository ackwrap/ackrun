package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	goruntime "runtime"
	"strconv"
	"strings"
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
	coreLogs  *CoreLogService
	store     *store.Store
	cmd       *exec.Cmd
	pid       int
	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	stopping  bool
	cachedVer string
	lastError string
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func NewSingboxService(p *paths.Paths, rt *RealtimeService, logs *CoreLogService, s *store.Store) *SingboxService {
	return &SingboxService{paths: p, realtime: rt, coreLogs: logs, store: s}
}

func (svc *SingboxService) Start() (*model.ActionResponse, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	if svc.stopping {
		return nil, fmt.Errorf("sing-box is stopping")
	}
	if svc.isRunning() {
		return nil, fmt.Errorf("sing-box is already running (pid=%d)", svc.pid)
	}

	if _, err := os.Stat(svc.paths.BinaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("sing-box binary not found")
	}

	configPath, err := svc.validateActiveConfig()
	if err != nil {
		svc.realtime.Broadcast("core.status", map[string]any{
			"status": "error",
			"pid":    0,
			"error":  err.Error(),
		})
		return nil, err
	}

	logging.Info("core.start", "starting sing-box")
	svc.realtime.Broadcast("core.status", map[string]any{
		"status": "starting",
		"pid":    0,
		"error":  "",
	})

	ctx, cancel := context.WithCancel(context.Background())
	svc.cancel = cancel
	svc.lastError = ""

	cmd := exec.CommandContext(ctx, svc.paths.BinaryPath, "run", "-c", configPath, "--disable-color")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		svc.cancel = nil
		return nil, fmt.Errorf("capture sing-box stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		svc.cancel = nil
		return nil, fmt.Errorf("capture sing-box stderr: %w", err)
	}

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
	svc.done = make(chan struct{})
	svc.stopping = false
	done := svc.done

	logging.Info("core.start", "sing-box started, pid=%d", svc.pid)
	svc.realtime.Broadcast("core.status", map[string]any{
		"status": "running",
		"pid":    svc.pid,
	})
	svc.broadcastRuntimeStatus(model.RuntimeRunning, svc.pid)

	var logWG sync.WaitGroup
	logWG.Add(2)
	go func() {
		defer logWG.Done()
		svc.captureCoreLog(stdout, "stdout")
	}()
	go func() {
		defer logWG.Done()
		svc.captureCoreLog(stderr, "stderr")
	}()

	go func() {
		err := cmd.Wait()
		logWG.Wait()
		svc.mu.Lock()
		intentionalStop := svc.cmd == cmd && svc.stopping
		if svc.cmd == cmd {
			svc.stopping = true
			svc.pid = 0
			svc.cmd = nil
			svc.cancel = nil
			svc.done = nil
		}
		svc.mu.Unlock()

		statusMsg, runtimeStatus, errorMessage := coreExitState(err, intentionalStop, svc.lastError)
		logging.Info("core.start", "sing-box exited: %v", err)
		svc.realtime.Broadcast("core.status", map[string]any{
			"status": statusMsg,
			"pid":    0,
			"error":  errorMessage,
		})
		svc.broadcastRuntimeStatus(runtimeStatus, 0)
		svc.mu.Lock()
		svc.stopping = false
		svc.mu.Unlock()
		close(done)
	}()

	return &model.ActionResponse{Success: true, Message: "service started"}, nil
}

func (svc *SingboxService) Stop() (*model.ActionResponse, error) {
	svc.mu.Lock()
	if !svc.isRunning() {
		svc.mu.Unlock()
		return nil, fmt.Errorf("sing-box is not running")
	}

	pid := svc.pid
	cmd := svc.cmd
	cancel := svc.cancel
	done := svc.done
	svc.stopping = true
	svc.mu.Unlock()

	logging.Info("core.stop", "stopping sing-box, pid=%d", pid)

	svc.realtime.Broadcast("core.status", map[string]any{
		"status": "stopping",
		"pid":    pid,
	})

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			logging.Info("core.stop", "force killing sing-box, pid=%d", pid)
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				return nil, fmt.Errorf("sing-box process did not exit after force kill (pid=%d)", pid)
			}
		}
	}

	return &model.ActionResponse{Success: true, Message: "service stopped"}, nil
}

func (svc *SingboxService) Restart() (*model.ActionResponse, error) {
	if _, err := svc.validateActiveConfig(); err != nil {
		return nil, err
	}
	if _, err := svc.Stop(); err != nil {
		return nil, err
	}

	return svc.Start()
}

func (svc *SingboxService) validateActiveConfig() (string, error) {
	configPath, ok, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("config file not found")
	}
	if output, err := exec.Command(svc.paths.BinaryPath, "check", "-c", configPath).CombinedOutput(); err != nil {
		message := strings.TrimSpace(cleanLogLine(string(output)))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("config check failed: %s", message)
	}
	return configPath, nil
}

func (svc *SingboxService) ReloadConfig() (*model.ActionResponse, error) {
	logging.Info("core.reload_config", "reloading sing-box config")
	if !svc.IsRunning() {
		return &model.ActionResponse{Success: true, Message: "core is stopped; config will be used on next start"}, nil
	}
	return svc.Restart()
}

func (svc *SingboxService) isRunning() bool {
	return svc.pid > 0 && svc.cmd != nil && svc.cmd.Process != nil
}

func (svc *SingboxService) IsRunning() bool {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return svc.isRunning()
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

func (svc *SingboxService) captureCoreLog(reader io.Reader, source string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := cleanLogLine(scanner.Text())
		if line == "" {
			continue
		}
		now := time.Now().UnixMilli()
		entry := svc.coreLogs.Append(source, now, line)
		svc.realtime.Broadcast("core.log", entry)
		if source == "stderr" && strings.Contains(line, "FATAL") {
			svc.mu.Lock()
			svc.lastError = line
			svc.mu.Unlock()
		}
	}
}

func coreExitErrorMessage(lastError string, processErr error) string {
	lower := strings.ToLower(lastError)
	if strings.Contains(lower, "configure tun interface") && strings.Contains(lower, "access is denied") {
		return "TUN 模式启动失败：Windows 拒绝创建 TUN 网卡，请以管理员身份运行 AckWrap"
	}
	if lastError != "" {
		return lastError
	}
	return processErr.Error()
}

func coreExitState(processErr error, intentionalStop bool, lastError string) (string, model.RuntimeStatus, string) {
	if processErr != nil && !intentionalStop {
		return "error", model.RuntimeError, coreExitErrorMessage(lastError, processErr)
	}
	return "stopped", model.RuntimeStopped, ""
}

// CloseConnections closes active connections through sing-box's Clash API.
func (svc *SingboxService) CloseConnections() (*model.ActionResponse, error) {
	logging.Info("core.close_connections", "closing all connections")
	if !svc.IsRunning() {
		return nil, fmt.Errorf("sing-box is not running")
	}
	settings, err := svc.store.GetExperimentalSettings()
	if err != nil {
		return nil, fmt.Errorf("read Clash API settings: %w", err)
	}
	port := "9090"
	secret := ""
	if settings != nil {
		if settings.ClashAPIPort != "" {
			port = settings.ClashAPIPort
		}
		secret = settings.ClashAPISecret
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, fmt.Errorf("invalid Clash API port")
	}
	target := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(portNumber)) + "/connections"
	if err := requestClashAPI(http.MethodDelete, target, secret); err != nil {
		return nil, fmt.Errorf("failed to close connections: %w", err)
	}
	return &model.ActionResponse{Success: true, Message: "all connections closed"}, nil
}

func requestClashAPI(method, target, secret string) error {
	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		return err
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Clash API returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// ResetFirewall resets local firewall rules on Windows.
func (svc *SingboxService) ResetFirewall() (*model.ActionResponse, error) {
	logging.Info("core.reset_firewall", "resetting firewall rules")
	if goruntime.GOOS != "windows" {
		return nil, fmt.Errorf("reset firewall is only supported on Windows")
	}
	cmd := exec.Command("netsh", "advfirewall", "reset")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.Info("core.reset_firewall", "failed: %v, output: %s", err, string(output))
		return nil, fmt.Errorf("failed to reset firewall: %w", err)
	}
	logging.Info("core.reset_firewall", "firewall reset successful")
	return &model.ActionResponse{Success: true, Message: "firewall rules reset"}, nil
}

// FlushDNS clears the local DNS cache on Windows.
func (svc *SingboxService) FlushDNS() (*model.ActionResponse, error) {
	logging.Info("core.flush_dns", "flushing DNS cache")
	if goruntime.GOOS != "windows" {
		return nil, fmt.Errorf("flush DNS is only supported on Windows")
	}
	cmd := exec.Command("ipconfig", "/flushdns")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.Info("core.flush_dns", "failed: %v, output: %s", err, string(output))
		return nil, fmt.Errorf("failed to flush DNS cache: %w", err)
	}
	logging.Info("core.flush_dns", "DNS cache flushed successfully")
	return &model.ActionResponse{Success: true, Message: "DNS cache flushed"}, nil
}

func (svc *SingboxService) CheckUpdate() (*model.ActionResponse, error) {
	logging.Info("core.check_update", "checking for updates")
	currentVersion := svc.getVersion()
	if currentVersion == "" {
		return nil, fmt.Errorf("failed to get current version")
	}
	settings, err := svc.store.GetUpdateSettings()
	if err != nil {
		return nil, fmt.Errorf("读取更新设置失败: %w", err)
	}
	latestVersion, err := fetchLatestSingboxVersionWithSettings(settings, singboxVersionURL)
	if err != nil {
		logging.Error("core.check_update", "检查更新失败: %v", err)
		return nil, fmt.Errorf("检查最新版本失败: %w", err)
	}
	if compareSingboxVersions(currentVersion, latestVersion) < 0 {
		logging.Info("core.check_update", "发现新版本 current=%s latest=%s", currentVersion, latestVersion)
		return &model.ActionResponse{Success: true, Message: fmt.Sprintf("发现新版本 %s（当前版本 %s），可通过核心安装器更新", latestVersion, currentVersion)}, nil
	}
	logging.Info("core.check_update", "当前已是最新版本: %s", currentVersion)
	return &model.ActionResponse{Success: true, Message: fmt.Sprintf("当前已是最新版本: %s", currentVersion)}, nil
}
