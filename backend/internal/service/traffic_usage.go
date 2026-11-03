package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/store"
)

// TrafficUsageService records independently of any open administration page.
type TrafficUsageService struct {
	store     *store.Store
	core      *SingboxService
	client    *http.Client
	location  *time.Location
	sampleMu  sync.Mutex
	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	lastError string
}

func NewTrafficUsageService(db *store.Store, core *SingboxService) *TrafficUsageService {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	location, _ := schedulerLocation()
	return &TrafficUsageService{store: db, core: core, location: location,
		client: &http.Client{Transport: transport, Timeout: 3 * time.Second}}
}

func (svc *TrafficUsageService) Start() {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	svc.cancel, svc.done = cancel, make(chan struct{})
	go func() {
		defer close(svc.done)
		svc.sample(ctx)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				svc.sample(ctx)
			}
		}
	}()
	logging.Info("traffic.recording", "本月流量后台统计已启动，采样间隔 5 秒")
}

func (svc *TrafficUsageService) Close() {
	svc.mu.Lock()
	cancel, done := svc.cancel, svc.done
	svc.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (svc *TrafficUsageService) Sample() { svc.sample(context.Background()) }

func (svc *TrafficUsageService) Error() string {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return svc.lastError
}

func (svc *TrafficUsageService) sample(ctx context.Context) {
	svc.sampleMu.Lock()
	defer svc.sampleMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	err := svc.collect(ctx)
	if ctx.Err() != nil {
		return
	}
	svc.mu.Lock()
	previous := svc.lastError
	svc.lastError = ""
	if err != nil {
		svc.lastError = err.Error()
	}
	current := svc.lastError
	svc.mu.Unlock()
	if current != "" && current != previous {
		logging.Error("traffic.recording", "本月流量记录失败: %s", current)
	} else if current == "" && previous != "" {
		logging.Info("traffic.recording", "本月流量记录已恢复")
	}
}

func (svc *TrafficUsageService) session() (string, int64) {
	if svc.core == nil {
		return "", 0
	}
	svc.core.mu.Lock()
	defer svc.core.mu.Unlock()
	if !svc.core.isRunning() || svc.core.coreStartedAt.IsZero() {
		return "", 0
	}
	return fmt.Sprintf("%d:%d", svc.core.pid, svc.core.coreStartedAt.UnixNano()), svc.core.coreStartedAt.Unix()
}

func (svc *TrafficUsageService) collect(ctx context.Context) error {
	now := time.Now().In(svc.location)
	if err := svc.store.EnsureTrafficMonth(now); err != nil {
		return fmt.Errorf("初始化本月统计失败: %w", err)
	}
	session, startedAt := svc.session()
	if session == "" {
		return nil
	}
	settings, err := svc.store.GetExperimentalSettings()
	if err != nil {
		return fmt.Errorf("读取流量接口设置失败: %w", err)
	}
	if settings == nil || !settings.ClashAPIEnabled {
		return fmt.Errorf("核心流量接口未启用")
	}
	port, err := strconv.Atoi(settings.ClashAPIPort)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("核心流量接口端口无效")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/connections", nil)
	if err != nil {
		return err
	}
	if settings.ClashAPISecret != "" {
		req.Header.Set("Authorization", "Bearer "+settings.ClashAPISecret)
	}
	response, err := svc.client.Do(req)
	if err != nil {
		return fmt.Errorf("核心流量接口暂不可用")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("核心流量接口返回 HTTP %d", response.StatusCode)
	}
	var counters struct {
		Upload   *int64 `json:"uploadTotal"`
		Download *int64 `json:"downloadTotal"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&counters); err != nil || counters.Upload == nil || counters.Download == nil || *counters.Upload < 0 || *counters.Download < 0 {
		return fmt.Errorf("核心流量接口返回无效计数")
	}
	if current, _ := svc.session(); current != session {
		return nil // A restart happened while the request was in flight.
	}
	if err = svc.store.RecordTrafficSample(store.TrafficSample{Session: session, StartedAt: startedAt, UploadTotal: *counters.Upload, DownloadTotal: *counters.Download}, time.Now().In(svc.location)); err != nil {
		return fmt.Errorf("保存本月流量失败: %w", err)
	}
	return nil
}
