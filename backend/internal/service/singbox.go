package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

type SingboxService struct {
	paths              *paths.Paths
	realtime           *RealtimeService
	coreLogs           *CoreLogService
	store              *store.Store
	dnsmasq            dnsmasqLifecycle
	dnsmasqSupported   func() bool
	cmd                *exec.Cmd
	pid                int
	mu                 sync.Mutex
	lifecycleMu        sync.Mutex
	networkLifecycleMu sync.Mutex
	done               chan error
	stopping           bool
	stopReason         string
	cachedVer          string
	lastError          string
}

var (
	ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
)

func NewSingboxService(p *paths.Paths, rt *RealtimeService, logs *CoreLogService, s *store.Store) *SingboxService {
	return &SingboxService{paths: p, realtime: rt, coreLogs: logs, store: s, dnsmasq: newDNSMasqLifecycle(p), dnsmasqSupported: platformSupportsDNSMasqTakeover}
}

func (svc *SingboxService) Start() (*model.ActionResponse, error) {
	svc.lifecycleMu.Lock()
	defer svc.lifecycleMu.Unlock()
	return svc.start()
}

// StartIfConfigured starts the core after backend startup when both its binary
// and an active configuration are available.
func (svc *SingboxService) StartIfConfigured() error {
	return startIfConfigured(
		svc.IsRunning,
		svc.paths.BinaryPath,
		svc.paths.ActiveConfigPath,
		func() error {
			_, err := svc.Start()
			return err
		},
	)
}

func startIfConfigured(isRunning func() bool, binaryPath string, activeConfig func() (string, bool, error), start func() error) error {
	if isRunning() {
		logging.Info("core.auto_start", "sing-box is already running")
		return nil
	}
	if _, err := os.Stat(binaryPath); err != nil {
		if os.IsNotExist(err) {
			logging.Info("core.auto_start", "skipping auto-start because sing-box is not installed")
			return nil
		}
		return fmt.Errorf("check sing-box binary: %w", err)
	}
	_, configured, err := activeConfig()
	if err != nil {
		return fmt.Errorf("check active config: %w", err)
	}
	if !configured {
		logging.Info("core.auto_start", "skipping auto-start because no active config exists")
		return nil
	}

	logging.Info("core.auto_start", "starting sing-box after Ackwrap startup")
	return start()
}

func (svc *SingboxService) start() (*model.ActionResponse, error) {
	svc.networkLifecycleMu.Lock()
	defer svc.networkLifecycleMu.Unlock()
	releaseNetworkLock, err := acquireNetworkLifecycleFileLock(svc.paths.NetworkLifecycleLockPath())
	if err != nil {
		return nil, err
	}
	defer releaseNetworkLock()

	svc.mu.Lock()
	defer svc.mu.Unlock()

	if svc.stopping {
		return nil, fmt.Errorf("sing-box is stopping")
	}
	if svc.isRunning() {
		return nil, fmt.Errorf("sing-box is already running (pid=%d)", svc.pid)
	}
	if err := svc.recoverStoppedState(false, true); err != nil {
		return nil, err
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

	svc.lastError = ""
	svc.stopReason = ""

	cmd := exec.Command(svc.paths.BinaryPath, "run", "-c", configPath, "--disable-color")
	if err := prepareProcessCommand(cmd); err != nil {
		svc.realtime.Broadcast("core.status", map[string]any{
			"status": "error",
			"pid":    0,
			"error":  err.Error(),
		})
		return nil, fmt.Errorf("prepare sing-box process: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("capture sing-box stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("capture sing-box stderr: %w", err)
	}
	processStarted := false
	defer func() {
		if !processStarted {
			_ = stdout.Close()
			_ = stderr.Close()
		}
	}()
	tunState, err := readActiveTUNState(configPath)
	if err != nil {
		return nil, fmt.Errorf("read active TUN state: %w", err)
	}
	dnsmasqTakeover, err := svc.shouldActivateDNSMasqTakeover(tunState)
	if err != nil {
		return nil, fmt.Errorf("check OpenWrt DNS takeover settings: %w", err)
	}
	routeTableBaseline, err := snapshotPlatformPriorityOneTables(tunState)
	if err != nil {
		logging.Error("core.start", "network ownership preflight failed: %v", err)
		return nil, fmt.Errorf("snapshot network state before starting sing-box: %w", err)
	}
	pendingOwnership := routeTableBaseline.Required
	if pendingOwnership {
		pendingState, err := pendingSingboxRouteTableState(routeTableBaseline)
		if err != nil {
			return nil, fmt.Errorf("prepare pending sing-box network ownership: %w", err)
		}
		if err := writeSingboxRouteTableState(svc.routeTableStatePath(), pendingState); err != nil {
			return nil, fmt.Errorf("persist pending sing-box network ownership: %w", err)
		}
	}

	if err := cmd.Start(); err != nil {
		startErr := fmt.Errorf("start sing-box: %w", err)
		if pendingOwnership {
			if discardErr := discardPendingSingboxRouteTableState(svc.routeTableStatePath()); discardErr != nil {
				startErr = errors.Join(startErr, discardErr)
			}
		}
		svc.realtime.Broadcast("core.status", map[string]any{
			"status": "error",
			"pid":    0,
			"error":  startErr.Error(),
		})
		return nil, startErr
	}
	processStarted = true

	svc.cmd = cmd
	svc.pid = cmd.Process.Pid
	svc.done = make(chan error, 1)
	svc.stopping = false
	done := svc.done
	processExited := make(chan struct{})

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
	ownershipResult := make(chan error, 1)
	if pendingOwnership {
		go func() {
			ownershipErr := recordPlatformSingboxRouteTables(svc.routeTableStatePath(), processExited)
			if ownershipErr != nil {
				logging.Error("core.cleanup", "record sing-box route-table ownership failed: %v", ownershipErr)
			}
			ownershipResult <- ownershipErr
		}()
	} else {
		ownershipResult <- nil
	}

	go func() {
		err := cmd.Wait()
		close(processExited)
		svc.networkLifecycleMu.Lock()
		releaseNetworkLock, networkLockErr := acquireNetworkLifecycleFileLock(svc.paths.NetworkLifecycleLockPath())
		logWG.Wait()
		<-ownershipResult
		svc.mu.Lock()
		intentionalStop := svc.cmd == cmd && svc.stopping
		stopReason := svc.stopReason
		if svc.cmd == cmd {
			svc.stopping = true
			svc.pid = 0
			svc.cmd = nil
			svc.done = nil
		}
		svc.mu.Unlock()

		statusMsg, runtimeStatus, errorMessage := coreExitState(err, intentionalStop, svc.lastError)
		cleanupErr := svc.cleanupAfterProcessExit(networkLockErr)
		if cleanupErr != nil {
			statusMsg = "error"
			runtimeStatus = model.RuntimeError
			if errorMessage != "" {
				errorMessage += "；"
			}
			errorMessage += "sing-box 已停止，但系统网络残留清理失败：" + cleanupErr.Error()
			logging.Error("core.cleanup", "cleanup after sing-box exit failed: %v", cleanupErr)
		}
		if intentionalStop {
			logging.Info("core.stop", "sing-box stopped, reason=%s", stopReason)
		} else if err != nil {
			logging.Error("core.start", "sing-box exited unexpectedly: %v", err)
		} else {
			logging.Error("core.start", "sing-box exited unexpectedly without a process error")
		}
		svc.realtime.Broadcast("core.status", map[string]any{
			"status": statusMsg,
			"pid":    0,
			"error":  errorMessage,
		})
		svc.broadcastRuntimeStatus(runtimeStatus, 0)
		svc.mu.Lock()
		svc.stopping = false
		svc.mu.Unlock()
		if releaseNetworkLock != nil {
			releaseNetworkLock()
		}
		svc.networkLifecycleMu.Unlock()
		done <- cleanupErr
		close(done)
	}()
	if dnsmasqTakeover {
		if err := waitForDNSInbound(processExited, 10*time.Second); err == nil && svc.dnsmasq != nil {
			err = svc.dnsmasq.Activate()
		} else if err == nil {
			err = fmt.Errorf("dnsmasq 生命周期服务不可用")
		}
		if err != nil {
			svc.lastError = "OpenWrt DNS 接管失败: " + err.Error()
			logging.Error("dnsmasq.takeover", "%s", svc.lastError)
			if shutdownErr := requestProcessShutdown(cmd.Process); shutdownErr != nil {
				_ = cmd.Process.Kill()
			}
			return nil, errors.New(svc.lastError)
		}
	}

	logging.Info("core.start", "sing-box started, pid=%d", svc.pid)
	svc.realtime.Broadcast("core.status", map[string]any{
		"status": "running",
		"pid":    svc.pid,
	})
	svc.broadcastRuntimeStatus(model.RuntimeRunning, svc.pid)

	return &model.ActionResponse{Success: true, Message: "service started"}, nil
}

func (svc *SingboxService) cleanupAfterProcessExit(networkLockErr error) error {
	if networkLockErr != nil {
		return fmt.Errorf("acquire network lifecycle lock before exit cleanup: %w", networkLockErr)
	}
	return svc.recoverStoppedState(true, false)
}

func (svc *SingboxService) shouldActivateDNSMasqTakeover(tunState activeTUNState) (bool, error) {
	if !tunState.DNSMasqTakeover {
		return false, nil
	}
	if svc.dnsmasqSupported == nil || !svc.dnsmasqSupported() {
		return false, nil
	}
	if svc.store == nil {
		return false, errors.New("设置存储不可用")
	}
	generalSettings, err := svc.store.GetGeneralSettings()
	if err != nil {
		return false, err
	}
	if !generalSettings.DNSMasqTakeoverEnabled {
		return false, nil
	}
	dnsSettings, err := svc.store.GetDNSGlobalSettings()
	if err != nil {
		return false, err
	}
	return dnsSettings.Enabled, nil
}

func (svc *SingboxService) Stop() (*model.ActionResponse, error) {
	svc.lifecycleMu.Lock()
	defer svc.lifecycleMu.Unlock()
	return svc.stop("manual stop request")
}

func (svc *SingboxService) Shutdown() (*model.ActionResponse, error) {
	svc.lifecycleMu.Lock()
	defer svc.lifecycleMu.Unlock()
	return svc.stop("Ackwrap backend shutdown")
}

func (svc *SingboxService) stop(reason string) (*model.ActionResponse, error) {
	svc.mu.Lock()
	if !svc.isRunning() {
		svc.mu.Unlock()
		return nil, fmt.Errorf("sing-box is not running")
	}

	pid := svc.pid
	cmd := svc.cmd
	done := svc.done
	svc.stopping = true
	svc.stopReason = reason
	svc.mu.Unlock()

	logging.Info("core.stop", "stopping sing-box, pid=%d, reason=%s", pid, reason)

	svc.realtime.Broadcast("core.status", map[string]any{
		"status": "stopping",
		"pid":    pid,
	})

	if cmd == nil || cmd.Process == nil {
		return nil, fmt.Errorf("sing-box process handle is unavailable (pid=%d)", pid)
	}
	if err := requestProcessShutdown(cmd.Process); err != nil {
		logging.Error("core.stop", "graceful stop signal failed, force killing sing-box, pid=%d: %v", pid, err)
		if killErr := cmd.Process.Kill(); killErr != nil {
			return nil, fmt.Errorf("request sing-box shutdown: %v; force kill: %w", err, killErr)
		}
	}
	if done != nil {
		select {
		case cleanupErr := <-done:
			if cleanupErr != nil {
				return nil, fmt.Errorf("sing-box stopped but network cleanup failed: %w", cleanupErr)
			}
		case <-time.After(10 * time.Second):
			logging.Info("core.stop", "force killing sing-box, pid=%d", pid)
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			select {
			case cleanupErr := <-done:
				if cleanupErr != nil {
					return nil, fmt.Errorf("sing-box stopped but network cleanup failed: %w", cleanupErr)
				}
			case <-time.After(2 * time.Second):
				return nil, fmt.Errorf("sing-box process did not exit after force kill (pid=%d)", pid)
			}
		}
	}

	return &model.ActionResponse{Success: true, Message: "service stopped"}, nil
}

// RecoverStaleState removes sing-box-owned OpenWrt state left by an unclean
// process exit. It is safe to call on every Ackwrap startup.
func (svc *SingboxService) RecoverStaleState() error {
	svc.lifecycleMu.Lock()
	defer svc.lifecycleMu.Unlock()
	svc.networkLifecycleMu.Lock()
	defer svc.networkLifecycleMu.Unlock()
	releaseNetworkLock, err := acquireNetworkLifecycleFileLock(svc.paths.NetworkLifecycleLockPath())
	if err != nil {
		return err
	}
	defer releaseNetworkLock()

	svc.mu.Lock()
	running := svc.isRunning()
	svc.mu.Unlock()
	if running {
		return nil
	}
	return svc.recoverStoppedState(false, false)
}

func (svc *SingboxService) recoverStoppedState(forceDNS, rejectRunning bool) error {
	recovery, err := svc.recoverStoppedStateDetailed(forceDNS)
	if err != nil {
		return err
	}
	if recovery.ProcessRunning && rejectRunning {
		return fmt.Errorf("an unmanaged sing-box process is already running")
	}
	return nil
}

type stoppedStateRecovery struct {
	ProcessRunning  bool
	NetworkCleaned  bool
	DNSMasqRestored bool
}

func (svc *SingboxService) recoverStoppedStateDetailed(forceDNS bool) (stoppedStateRecovery, error) {
	result, cleanupErr := cleanupPlatformSingboxState(svc.routeTableStatePath())
	if result.ProcessRunning {
		logging.Info("core.cleanup", "skipping stale-state cleanup because a sing-box process is running")
		return stoppedStateRecovery{ProcessRunning: true}, nil
	}
	if result.Cleaned && cleanupErr == nil {
		logging.Info("core.cleanup", "removed stale sing-box OpenWrt network state")
	}
	var failures []error
	if cleanupErr != nil {
		failures = append(failures, fmt.Errorf("cleanup sing-box network state: %w", cleanupErr))
	}
	dnsmasqRestored := false
	if svc.dnsmasq != nil {
		var restoreErr error
		dnsmasqRestored, restoreErr = svc.dnsmasq.Restore()
		if restoreErr != nil {
			failures = append(failures, fmt.Errorf("restore OpenWrt dnsmasq: %w", restoreErr))
		}
	}
	if forceDNS || result.Cleaned || dnsmasqRestored {
		if err := flushSystemDNS(false); err != nil {
			failures = append(failures, err)
		}
	}
	return stoppedStateRecovery{
		NetworkCleaned:  result.Cleaned,
		DNSMasqRestored: dnsmasqRestored,
	}, errors.Join(failures...)
}

func (svc *SingboxService) routeTableStatePath() string {
	return filepath.Join(svc.paths.DataDir, ".singbox-route-tables.json")
}

func (svc *SingboxService) Restart() (*model.ActionResponse, error) {
	return svc.restartWithReason("manual restart request")
}

func (svc *SingboxService) ScheduledRestart() (*model.ActionResponse, error) {
	return svc.restartWithReason("scheduled restart")
}

func (svc *SingboxService) restartWithReason(reason string) (*model.ActionResponse, error) {
	svc.lifecycleMu.Lock()
	defer svc.lifecycleMu.Unlock()
	return svc.restart(reason)
}

func (svc *SingboxService) restart(reason string) (*model.ActionResponse, error) {
	if _, err := svc.validateActiveConfig(); err != nil {
		return nil, err
	}
	if _, err := svc.stop(reason); err != nil {
		return nil, err
	}

	return svc.start()
}

func (svc *SingboxService) validateActiveConfig() (string, error) {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	configPath, ok, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("config file not found")
	}
	if err := svc.migrateRuleSetAccessToken(configPath); err != nil {
		return "", err
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

func (svc *SingboxService) migrateRuleSetAccessToken(configPath string) error {
	if err := os.Chmod(configPath, 0600); err != nil {
		return fmt.Errorf("protect config before rule-set authentication migration: %w", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config for rule-set authentication migration: %w", err)
	}
	migratedData, migrated, err := migrateInternalRuleSetAccessTokenData(
		data,
		internalAPIBaseURL(),
		os.Getenv("ACKWRAP_API_TOKEN"),
	)
	if err != nil || migrated == 0 {
		return err
	}
	stagedFile, err := os.CreateTemp(filepath.Dir(configPath), ".ackwrap-rule-set-auth-*.tmp")
	if err != nil {
		return fmt.Errorf("create rule-set authentication migration file: %w", err)
	}
	stagedPath := stagedFile.Name()
	defer os.Remove(stagedPath)
	if err := stagedFile.Chmod(0600); err != nil {
		stagedFile.Close()
		return fmt.Errorf("protect rule-set authentication migration file: %w", err)
	}
	if _, err := stagedFile.Write(migratedData); err != nil {
		stagedFile.Close()
		return fmt.Errorf("write rule-set authentication migration file: %w", err)
	}
	if err := stagedFile.Close(); err != nil {
		return fmt.Errorf("close rule-set authentication migration file: %w", err)
	}
	if output, err := exec.Command(svc.paths.BinaryPath, "check", "-c", stagedPath).CombinedOutput(); err != nil {
		message := strings.TrimSpace(cleanLogLine(string(output)))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("rule-set authentication migration validation failed: %s", message)
	}
	if _, _, err := ensureDailyConfigBackup(svc.paths, svc.store, configPath, time.Now()); err != nil {
		return fmt.Errorf("backup config before rule-set authentication migration: %w", err)
	}
	if err := atomicReplaceFile(stagedPath, configPath); err != nil {
		return fmt.Errorf("apply rule-set authentication migration: %w", err)
	}
	logging.Info("config.migrate", "已更新 %d 个本机规则集认证 URL", migrated)
	return nil
}

func (svc *SingboxService) ReloadConfig() (*model.ActionResponse, error) {
	svc.lifecycleMu.Lock()
	defer svc.lifecycleMu.Unlock()
	logging.Info("core.reload_config", "reloading sing-box config")
	if !svc.IsRunning() {
		return &model.ActionResponse{Success: true, Message: "core is stopped; config will be used on next start"}, nil
	}
	return svc.restart("config reload")
}

func (svc *SingboxService) isRunning() bool {
	return svc.pid > 0 && svc.cmd != nil && svc.cmd.Process != nil
}

func (svc *SingboxService) IsRunning() bool {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return svc.isRunning()
}

func (svc *SingboxService) IsInstalledAndConfigured() bool {
	if _, err := os.Stat(svc.paths.BinaryPath); err != nil {
		return false
	}
	_, configured, err := svc.paths.ActiveConfigPath()
	return err == nil && configured
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
	line = ansiEscapePattern.ReplaceAllString(line, "")
	return redactAccessToken(line)
}

func redactAccessToken(value string) string {
	return logging.RedactAccessToken(value)
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
	if strings.Contains(lower, "auto-redirect") || strings.Contains(lower, "auto_redirect") || strings.Contains(lower, "nftables") {
		return "OpenWrt 透明代理启动失败：sing-box 无法配置 nftables/fw4，请确认以 root 或 CAP_NET_ADMIN 运行且系统使用 fw4/nftables；核心错误：" + lastError
	}
	if strings.Contains(lower, "configure tun interface") && (strings.Contains(lower, "operation not permitted") || strings.Contains(lower, "permission denied")) {
		return "TUN 模式启动失败：缺少创建 TUN 和修改路由的权限，请以 root 运行或授予 CAP_NET_ADMIN"
	}
	if lastError != "" {
		return lastError
	}
	return processErr.Error()
}

func coreExitState(processErr error, intentionalStop bool, lastError string) (string, model.RuntimeStatus, string) {
	if !intentionalStop {
		if processErr == nil {
			processErr = fmt.Errorf("sing-box process exited without an error status")
		}
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
	if err := svc.requestClashAPI(http.MethodDelete, "/connections"); err != nil {
		return nil, fmt.Errorf("failed to close connections: %w", err)
	}
	return &model.ActionResponse{Success: true, Message: "all connections closed"}, nil
}

func (svc *SingboxService) FlushCoreDNS() (*model.ActionResponse, error) {
	logging.Info("core.flush_core_dns", "flushing sing-box DNS cache")
	if !svc.IsRunning() {
		return nil, fmt.Errorf("sing-box is not running")
	}
	if err := svc.requestClashAPI(http.MethodPost, "/cache/dns/flush"); err != nil {
		return nil, fmt.Errorf("failed to flush sing-box DNS cache: %w", err)
	}
	return &model.ActionResponse{Success: true, Message: "sing-box DNS cache flushed"}, nil
}

func (svc *SingboxService) FlushFakeIP() (*model.ActionResponse, error) {
	logging.Info("core.flush_fakeip", "flushing sing-box FakeIP cache")
	if !svc.IsRunning() {
		return nil, fmt.Errorf("sing-box is not running")
	}
	if err := svc.requestClashAPI(http.MethodPost, "/cache/fakeip/flush"); err != nil {
		if isEmptyFakeIPCacheError(err) {
			logging.Info("core.flush_fakeip", "FakeIP cache is already empty")
			return &model.ActionResponse{Success: true, Message: "FakeIP cache already empty"}, nil
		}
		return nil, fmt.Errorf("failed to flush FakeIP cache: %w", err)
	}
	return &model.ActionResponse{Success: true, Message: "FakeIP cache flushed"}, nil
}

type clashAPIResponseError struct {
	statusCode int
	message    string
}

func (err *clashAPIResponseError) Error() string {
	if err.message != "" {
		return fmt.Sprintf("Clash API returned HTTP %d: %s", err.statusCode, err.message)
	}
	return fmt.Sprintf("Clash API returned HTTP %d", err.statusCode)
}

func isEmptyFakeIPCacheError(err error) bool {
	var responseErr *clashAPIResponseError
	return errors.As(err, &responseErr) &&
		responseErr.statusCode == http.StatusInternalServerError &&
		strings.EqualFold(strings.TrimSpace(responseErr.message), "bucket not found")
}

func (svc *SingboxService) requestClashAPI(method, path string) error {
	settings, err := svc.store.GetExperimentalSettings()
	if err != nil {
		return fmt.Errorf("read Clash API settings: %w", err)
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
		return fmt.Errorf("invalid Clash API port")
	}
	target := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(portNumber)) + path
	if err := requestClashAPI(method, target, secret); err != nil {
		return err
	}
	return nil
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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var payload struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &payload)
		return &clashAPIResponseError{statusCode: resp.StatusCode, message: strings.TrimSpace(payload.Message)}
	}
	return nil
}

func (svc *SingboxService) NetworkCheck() (*model.MaintenanceCheckResponse, error) {
	logging.Info("core.network_check", "running maintenance network checks")
	return svc.networkCheck(), nil
}

type activeTUNState struct {
	Enabled                  bool
	IPv4                     bool
	IPv6                     bool
	ManagesRoutes            bool
	AutoRedirect             bool
	RouteManagingInbounds    int
	ExpectedIPv4             bool
	ExpectedIPv6             bool
	AutoRouteWithoutRedirect bool
	CleanupIdentityError     string
	DNSMasqTakeover          bool
}

func readActiveTUNState(configPath string) (activeTUNState, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return activeTUNState{}, err
	}
	var config struct {
		DNS      *struct{} `json:"dns"`
		Inbounds []struct {
			Tag                           string          `json:"tag"`
			Type                          string          `json:"type"`
			Listen                        string          `json:"listen"`
			ListenPort                    int             `json:"listen_port"`
			Address                       []string        `json:"address"`
			AutoRoute                     bool            `json:"auto_route"`
			AutoRedirect                  bool            `json:"auto_redirect"`
			IPRoute2TableIndex            json.RawMessage `json:"iproute2_table_index"`
			IPRoute2RuleIndex             json.RawMessage `json:"iproute2_rule_index"`
			AutoRedirectFallbackRuleIndex json.RawMessage `json:"auto_redirect_iproute2_fallback_rule_index"`
			AutoRedirectInputMark         json.RawMessage `json:"auto_redirect_input_mark"`
			AutoRedirectOutputMark        json.RawMessage `json:"auto_redirect_output_mark"`
		} `json:"inbounds"`
		Route struct {
			Rules []map[string]json.RawMessage `json:"rules"`
		} `json:"route"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return activeTUNState{}, err
	}
	state := activeTUNState{}
	dnsInboundPresent := false
	for index, inbound := range config.Inbounds {
		if inbound.Tag == dnsInboundTag {
			if inbound.Type != "direct" || inbound.Listen != "127.0.0.1" || inbound.ListenPort != defaultDNSInboundPort {
				return activeTUNState{}, fmt.Errorf("Ackwrap DNS 入站配置无效")
			}
			dnsInboundPresent = true
		}
		if inbound.Type != "tun" {
			continue
		}
		state.Enabled = true
		managesRoutes := inbound.AutoRoute || inbound.AutoRedirect
		if managesRoutes {
			state.ManagesRoutes = true
			state.RouteManagingInbounds++
			state.AutoRedirect = state.AutoRedirect || inbound.AutoRedirect
			state.AutoRouteWithoutRedirect = state.AutoRouteWithoutRedirect || inbound.AutoRoute && !inbound.AutoRedirect
			identityFields := []struct {
				name string
				raw  json.RawMessage
				want uint64
			}{
				{name: "iproute2_table_index", raw: inbound.IPRoute2TableIndex, want: defaultIPRoute2TableIndex},
				{name: "iproute2_rule_index", raw: inbound.IPRoute2RuleIndex, want: defaultIPRoute2RuleIndex},
				{name: "auto_redirect_iproute2_fallback_rule_index", raw: inbound.AutoRedirectFallbackRuleIndex, want: defaultFallbackRuleIndex},
				{name: "auto_redirect_input_mark", raw: inbound.AutoRedirectInputMark, want: defaultAutoRedirectInputMark},
				{name: "auto_redirect_output_mark", raw: inbound.AutoRedirectOutputMark, want: defaultAutoRedirectMark},
			}
			for _, field := range identityFields {
				value, present, parseErr := parseOptionalTUNUint(field.raw)
				if parseErr != nil {
					return activeTUNState{}, fmt.Errorf("parse TUN inbound %d %s: %w", index, field.name, parseErr)
				}
				if present && value != 0 && value != field.want && state.CleanupIdentityError == "" {
					state.CleanupIdentityError = fmt.Sprintf("TUN inbound %d uses non-default %s=%d; cleanup identity requires %d", index, field.name, value, field.want)
				}
			}
		}
		for _, address := range inbound.Address {
			prefix, err := netip.ParsePrefix(address)
			if err != nil {
				continue
			}
			if prefix.Addr().Is4() {
				state.IPv4 = true
				state.ExpectedIPv4 = state.ExpectedIPv4 || managesRoutes
			} else if prefix.Addr().Is6() {
				state.IPv6 = true
				state.ExpectedIPv6 = state.ExpectedIPv6 || managesRoutes
			}
		}
	}
	dnsHijackRulePresent := false
	if len(config.Route.Rules) > 0 && len(config.Route.Rules[0]) == 2 {
		var inboundTag, action string
		inboundErr := json.Unmarshal(config.Route.Rules[0]["inbound"], &inboundTag)
		actionErr := json.Unmarshal(config.Route.Rules[0]["action"], &action)
		dnsHijackRulePresent = inboundErr == nil && actionErr == nil && inboundTag == dnsInboundTag && action == "hijack-dns"
	}
	state.DNSMasqTakeover = state.Enabled && config.DNS != nil && dnsInboundPresent && dnsHijackRulePresent
	return state, nil
}

func parseOptionalTUNUint(raw json.RawMessage) (uint64, bool, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return 0, false, nil
	}
	if strings.HasPrefix(value, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return 0, true, err
		}
		value = strings.TrimSpace(text)
	}
	parsed, err := strconv.ParseUint(value, 0, 32)
	if err != nil {
		return 0, true, fmt.Errorf("expected an unsigned 32-bit integer: %w", err)
	}
	return parsed, true, nil
}

func validateLinuxTUNCompatibility(state activeTUNState) error {
	if !state.ManagesRoutes {
		return nil
	}
	if state.RouteManagingInbounds != 1 {
		return fmt.Errorf("Linux route-managing TUN requires exactly one route-managing inbound, found %d", state.RouteManagingInbounds)
	}
	if !state.ExpectedIPv4 && !state.ExpectedIPv6 {
		return fmt.Errorf("Linux route-managing TUN requires at least one parseable IPv4 or IPv6 address family")
	}
	if state.AutoRouteWithoutRedirect {
		return fmt.Errorf("Linux auto_route without auto_redirect is not lifecycle-safe and cannot be started")
	}
	if state.CleanupIdentityError != "" {
		return fmt.Errorf("Linux TUN cleanup identity mismatch: %s", state.CleanupIdentityError)
	}
	return nil
}

func (svc *SingboxService) networkCheck() *model.MaintenanceCheckResponse {
	checks := make([]model.MaintenanceCheck, 0, 5)
	add := func(key, label, status, message string) {
		checks = append(checks, model.MaintenanceCheck{Key: key, Label: label, Status: status, Message: message})
	}

	binaryReady := false
	if _, err := os.Stat(svc.paths.BinaryPath); err != nil {
		add("binary", "核心程序", "fail", "sing-box 核心程序不存在")
	} else {
		binaryReady = true
		add("binary", "核心程序", "pass", "sing-box 核心程序可用")
	}

	configPath, configPresent, configPathErr := svc.paths.ActiveConfigPath()
	if configPathErr != nil {
		add("config", "配置文件", "fail", "无法读取当前配置状态")
	} else if !configPresent {
		add("config", "配置文件", "fail", "当前没有可用配置文件")
	} else if !binaryReady {
		add("config", "配置文件", "warn", "需要安装核心后才能校验配置")
	} else if _, err := svc.validateActiveConfig(); err != nil {
		add("config", "配置文件", "fail", "当前配置未通过 sing-box 校验")
	} else {
		add("config", "配置文件", "pass", "当前配置校验通过")
	}

	running := svc.IsRunning()
	if running {
		add("process", "核心进程", "pass", "sing-box 核心正在运行")
		if err := svc.requestClashAPI(http.MethodGet, "/version"); err != nil {
			add("clash_api", "Clash API 端口", "fail", "Clash API 不可访问，请检查端口和密钥设置")
		} else {
			add("clash_api", "Clash API 端口", "pass", "Clash API 可正常访问")
		}
	} else {
		add("process", "核心进程", "warn", "sing-box 核心当前未运行")
		add("clash_api", "Clash API 端口", "warn", "核心启动后才能检测 Clash API")
	}

	if goruntime.GOOS == "windows" {
		if err := exec.Command("net", "session").Run(); err != nil {
			add("administrator", "管理员权限", "warn", "当前进程可能没有管理员权限，TUN 和系统维护操作可能失败")
		} else {
			add("administrator", "管理员权限", "pass", "当前进程具有管理员权限")
		}
	} else if goruntime.GOOS == "linux" && configPathErr == nil && configPresent {
		tunState, err := readActiveTUNState(configPath)
		if err != nil {
			add("tun_config", "TUN 配置", "warn", "无法解析活动配置的 TUN 状态")
		} else if tunState.Enabled {
			if os.Geteuid() != 0 {
				add("tun_permission", "TUN 管理权限", "warn", "当前进程不是 root；OpenWrt 透明代理需要 CAP_NET_ADMIN 和 nftables 管理权限")
			} else {
				add("tun_permission", "TUN 管理权限", "pass", "当前进程以 root 运行")
			}
			forwarding, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
			if err != nil {
				add("ip_forward", "IPv4 转发", "warn", "无法读取 net.ipv4.ip_forward；请确认 OpenWrt LAN 转发已启用")
			} else if strings.TrimSpace(string(forwarding)) != "1" {
				add("ip_forward", "IPv4 转发", "warn", "net.ipv4.ip_forward 未启用，本机 TUN 可用但无法透明代理 LAN 设备")
			} else {
				add("ip_forward", "IPv4 转发", "pass", "IPv4 LAN 转发已启用")
			}
			if !tunState.IPv6 {
				add("ipv6_forward", "IPv6 转发", "warn", "活动 TUN 未配置 IPv6 地址，LAN IPv6 流量可能绕过代理")
			} else {
				forwarding, err := os.ReadFile("/proc/sys/net/ipv6/conf/all/forwarding")
				if err != nil {
					add("ipv6_forward", "IPv6 转发", "warn", "无法读取 IPv6 forwarding；请确认 OpenWrt IPv6 LAN 转发已启用")
				} else if strings.TrimSpace(string(forwarding)) != "1" {
					add("ipv6_forward", "IPv6 转发", "warn", "IPv6 forwarding 未启用，LAN IPv6 流量可能绕过代理")
				} else {
					add("ipv6_forward", "IPv6 转发", "pass", "IPv6 LAN 转发已启用")
				}
			}
		}
	}

	success := true
	for _, check := range checks {
		if check.Status == "fail" {
			success = false
			break
		}
	}
	return &model.MaintenanceCheckResponse{Success: success, Checks: checks}
}

func (svc *SingboxService) Diagnostics() (*model.CoreDiagnosticsResponse, error) {
	logging.Info("core.diagnostics", "building redacted diagnostics report")
	configPath, configPresent, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return nil, fmt.Errorf("read active config path: %w", err)
	}
	network := svc.networkCheck()
	configValid := false
	for _, check := range network.Checks {
		if check.Key == "config" {
			configValid = check.Status == "pass"
			break
		}
	}

	logSummary := model.CoreLogSummary{}
	if svc.coreLogs != nil {
		for _, entry := range svc.coreLogs.List(defaultCoreLogLimit) {
			logSummary.Total++
			switch entry.Source {
			case "stdout":
				logSummary.Stdout++
			case "stderr":
				logSummary.Stderr++
			}
			line := strings.ToLower(entry.Line)
			if strings.Contains(line, "error") || strings.Contains(line, "fatal") {
				logSummary.ErrorLines++
			}
		}
	}

	return &model.CoreDiagnosticsResponse{
		GeneratedAt:   time.Now().UnixMilli(),
		Platform:      goruntime.GOOS,
		Architecture:  goruntime.GOARCH,
		Version:       svc.getVersion(),
		Running:       svc.IsRunning(),
		PID:           svc.GetPID(),
		BinaryPath:    svc.paths.BinaryPath,
		ConfigPath:    configPath,
		ConfigPresent: configPresent,
		ConfigValid:   configValid,
		Network:       *network,
		Logs:          logSummary,
	}, nil
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

type systemDNSFlushCommand struct {
	name string
	path string
	args []string
}

func systemDNSFlushCommands(platform string, lookPath func(string) (string, error)) []systemDNSFlushCommand {
	if platform == "windows" {
		return []systemDNSFlushCommand{{name: "ipconfig", path: "ipconfig", args: []string{"/flushdns"}}}
	}
	if platform != "linux" {
		return nil
	}
	path, err := lookPath("ubus")
	if err != nil {
		return nil
	}
	return []systemDNSFlushCommand{{
		name: "OpenWrt dnsmasq",
		path: path,
		args: []string{"call", "service", "signal", `{"name":"dnsmasq","signal":1}`},
	}}
}

func flushSystemDNS(required bool) error {
	commands := systemDNSFlushCommands(goruntime.GOOS, exec.LookPath)
	if len(commands) == 0 {
		if required {
			return fmt.Errorf("当前系统未找到可用的 DNS 缓存清理工具")
		}
		return nil
	}
	failures := make([]string, 0, len(commands))
	for _, command := range commands {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		output, err := exec.CommandContext(ctx, command.path, command.args...).CombinedOutput()
		cancel()
		if err == nil {
			logging.Info("core.flush_dns", "DNS cache flushed successfully via %s", command.name)
			return nil
		}
		details := strings.Join(strings.Fields(string(output)), " ")
		if len(details) > 512 {
			details = details[:512]
		}
		logging.Error("core.flush_dns", "%s failed: %v, output: %s", command.name, err, details)
		failure := fmt.Sprintf("%s: %v", command.name, err)
		if details != "" {
			failure += ": " + details
		}
		failures = append(failures, failure)
	}
	return fmt.Errorf("清理系统 DNS 缓存失败: %s", strings.Join(failures, "; "))
}

// FlushDNS clears the operating system DNS cache.
func (svc *SingboxService) FlushDNS() (*model.ActionResponse, error) {
	logging.Info("core.flush_dns", "flushing DNS cache")
	if err := flushSystemDNS(true); err != nil {
		return nil, err
	}
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
