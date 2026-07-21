package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	defaultServiceURL = "http://127.0.0.1:18080"
	serviceURLEnv     = "ACKWRAP_GUI_SERVICE_URL"
	connectTimeout    = 30 * time.Second
	probeTimeout      = 2 * time.Second
)

var validRuntimeStatuses = map[string]struct{}{
	"not_installed": {},
	"no_config":     {},
	"stopped":       {},
	"running":       {},
}

type ServiceStatus struct {
	State   string `json:"state"`
	Message string `json:"message"`
	URL     string `json:"url"`
	Attempt int    `json:"attempt"`
}

type App struct {
	serviceURL *url.URL
	httpClient *http.Client

	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	connecting bool
}

func NewApp() (*App, error) {
	serviceURL, err := loadServiceURL(os.Getenv(serviceURLEnv))
	if err != nil {
		return nil, err
	}
	return &App{
		serviceURL: serviceURL,
		httpClient: &http.Client{
			Timeout: probeTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("service probe must not redirect")
			},
		},
	}, nil
}

func loadServiceURL(configured string) (*url.URL, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		configured = defaultServiceURL
	}
	serviceURL, err := url.Parse(configured)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", serviceURLEnv, err)
	}
	if serviceURL.Scheme != "http" || serviceURL.User != nil || serviceURL.RawQuery != "" || serviceURL.Fragment != "" {
		return nil, fmt.Errorf("%s must be a plain loopback HTTP URL", serviceURLEnv)
	}
	host, port, err := net.SplitHostPort(serviceURL.Host)
	if err != nil || port == "" {
		return nil, fmt.Errorf("%s must include a loopback host and port", serviceURLEnv)
	}
	if !strings.EqualFold(host, "localhost") {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return nil, fmt.Errorf("%s must use a loopback address", serviceURLEnv)
		}
	}
	if serviceURL.Path != "" && serviceURL.Path != "/" {
		return nil, fmt.Errorf("%s must not include a path", serviceURLEnv)
	}
	serviceURL.Path = ""
	return serviceURL, nil
}

func (app *App) startup(ctx context.Context) {
	app.mu.Lock()
	app.ctx, app.cancel = context.WithCancel(ctx)
	app.mu.Unlock()
}

func (app *App) domReady(context.Context) {
	app.ConnectService()
}

func (app *App) shutdown(context.Context) {
	app.mu.Lock()
	if app.cancel != nil {
		app.cancel()
	}
	app.mu.Unlock()
}

func (app *App) secondInstanceLaunch(options.SecondInstanceData) {
	app.mu.Lock()
	ctx := app.ctx
	app.mu.Unlock()
	if ctx == nil {
		return
	}
	runtime.WindowUnminimise(ctx)
	runtime.WindowShow(ctx)
}

// ConnectService probes Ackwrap Service and navigates only after its runtime API is verified.
func (app *App) ConnectService() {
	app.mu.Lock()
	if app.connecting || app.ctx == nil {
		app.mu.Unlock()
		return
	}
	app.connecting = true
	ctx := app.ctx
	app.mu.Unlock()

	go func() {
		defer func() {
			app.mu.Lock()
			app.connecting = false
			app.mu.Unlock()
		}()

		connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
		defer cancel()
		for attempt := 1; ; attempt++ {
			err := app.probeService(connectCtx)
			if err == nil {
				app.emitStatus(ServiceStatus{
					State:   "ready",
					Message: "Ackwrap Service 已连接，正在打开管理页面",
					URL:     app.serviceURL.String(),
					Attempt: attempt,
				})
				encodedURL, _ := json.Marshal(app.serviceURL.String() + "/")
				runtime.WindowExecJS(ctx, "window.location.replace("+string(encodedURL)+")")
				return
			}
			if connectCtx.Err() != nil {
				app.emitStatus(ServiceStatus{
					State:   "failed",
					Message: "无法连接 Ackwrap Service，请确认服务已安装并正在运行",
					URL:     app.serviceURL.String(),
					Attempt: attempt,
				})
				return
			}
			app.emitStatus(ServiceStatus{
				State:   "waiting",
				Message: "正在等待 Ackwrap Service 启动",
				URL:     app.serviceURL.String(),
				Attempt: attempt,
			})
			timer := time.NewTimer(time.Second)
			select {
			case <-connectCtx.Done():
				timer.Stop()
			case <-timer.C:
				continue
			}
		}
	}()
}

func (app *App) probeService(ctx context.Context) error {
	endpoint := app.serviceURL.ResolveReference(&url.URL{Path: "/api/v1/runtime"})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	response, err := app.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("runtime endpoint returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&payload); err != nil {
		return fmt.Errorf("decode runtime response: %w", err)
	}
	if _, valid := validRuntimeStatuses[payload.Status]; !valid {
		return errors.New("runtime endpoint returned an unknown status")
	}
	return nil
}

func (app *App) emitStatus(status ServiceStatus) {
	app.mu.Lock()
	ctx := app.ctx
	app.mu.Unlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, "service.status", status)
	}
}
