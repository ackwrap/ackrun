package service

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

type RuntimeService struct {
	paths     *paths.Paths
	store     *store.Store
	singbox   *SingboxService
	startedAt time.Time
	traffic   *TrafficUsageService
}

func NewRuntimeService(p *paths.Paths, s *store.Store, sb *SingboxService) *RuntimeService {
	return &RuntimeService{paths: p, store: s, singbox: sb, startedAt: time.Now(), traffic: NewTrafficUsageService(s, sb)}
}

func (svc *RuntimeService) StartTrafficRecording() { svc.traffic.Start() }
func (svc *RuntimeService) StopTrafficRecording()  { svc.traffic.Close() }
func (svc *RuntimeService) SampleTraffic()         { svc.traffic.Sample() }

func (svc *RuntimeService) GetSummary() (*model.ControlSummary, error) {
	now := time.Now().In(svc.traffic.location)
	traffic, err := svc.store.GetMonthlyTraffic(now)
	if err != nil {
		return nil, err
	}
	subscriptions, err := svc.store.ListSubscriptions()
	if err != nil {
		return nil, err
	}
	result := summarizeSubscriptions(subscriptions, now)
	result.MonthlyTraffic = traffic
	result.TrafficError = svc.traffic.Error()
	return result, nil
}

func summarizeSubscriptions(subscriptions []model.Subscription, now time.Time) *model.ControlSummary {
	result := &model.ControlSummary{}
	for _, item := range subscriptions {
		if strings.TrimSpace(item.URL) == "manual://local" {
			continue
		}
		result.SubscriptionCount++
		if item.TrafficTotalBytes > 0 {
			result.SubscriptionTotalBytes += item.TrafficTotalBytes
			result.TrafficReportedCount++
		}
		if item.ExpireAt <= 0 {
			continue
		}
		if item.ExpireAt <= now.Unix() {
			result.ExpiredSubscriptionCount++
			continue
		}
		if result.NextExpiry == nil || item.ExpireAt < result.NextExpiry.ExpireAt {
			result.NextExpiry = &model.SubscriptionExpiry{Name: item.Name, ExpireAt: item.ExpireAt}
		}
	}
	return result
}

func (svc *RuntimeService) GetStatus() (*model.RuntimeResponse, error) {
	logging.Info("runtime.check", "checking runtime status")

	uptimeSeconds := int64(time.Since(svc.startedAt).Seconds())
	resp := &model.RuntimeResponse{Platform: runtime.GOOS, UptimeSeconds: &uptimeSeconds}

	if _, err := os.Stat(svc.paths.BinaryPath); os.IsNotExist(err) {
		logging.Info("runtime.check", "binary not found: %s", svc.paths.BinaryPath)
		resp.Status = model.RuntimeNotInstalled
		return resp, nil
	}

	resp.Version = svc.getVersion()

	if configPath, ok, err := svc.paths.ActiveConfigPath(); err != nil {
		return nil, err
	} else if !ok {
		logging.Info("runtime.check", "config not found in: %s", svc.paths.ConfigDir)
		resp.Status = model.RuntimeNoConfig
		return resp, nil
	} else {
		resp.ProxyPort = readMixedInboundPort(configPath)
	}

	pid, coreUptimeSeconds := svc.singbox.GetProcessState()
	if pid > 0 {
		resp.Status = model.RuntimeRunning
		resp.PID = pid
		resp.CoreUptimeSeconds = &coreUptimeSeconds
		return resp, nil
	}

	resp.Status = model.RuntimeStopped
	return resp, nil
}

func readMixedInboundPort(configPath string) int {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return 0
	}
	var config struct {
		Inbounds []struct {
			Type       string `json:"type"`
			ListenPort int    `json:"listen_port"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return 0
	}
	for _, inbound := range config.Inbounds {
		if inbound.Type == "mixed" && inbound.ListenPort > 0 {
			return inbound.ListenPort
		}
	}
	return 0
}

var cachedVersion string

func (svc *RuntimeService) getVersion() string {
	installState, err := svc.store.GetInstallState()
	if err == nil && isSingboxVersion(installState.Version) {
		return installState.Version
	}
	if cachedVersion != "" {
		return cachedVersion
	}

	cachedVersion = readSingboxVersion(svc.paths.BinaryPath)
	return cachedVersion
}
