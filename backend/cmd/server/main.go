package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/api"
	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/service"
	"github.com/ackwrap/ackwrap/internal/store"
	"github.com/ackwrap/ackwrap/internal/webui"
)

const defaultListenAddr = "0.0.0.0:8080"

type serverConfig struct {
	ListenAddr string
	APIToken   string
}

type accessTokenRedactingWriter struct {
	io.Writer
}

func (writer accessTokenRedactingWriter) Write(value []byte) (int, error) {
	if _, err := writer.Writer.Write([]byte(logging.RedactAccessToken(string(value)))); err != nil {
		return 0, err
	}
	return len(value), nil
}

func loadServerConfig() (serverConfig, error) {
	config := serverConfig{
		ListenAddr: strings.TrimSpace(os.Getenv("ACKWRAP_LISTEN_ADDR")),
		APIToken:   strings.TrimSpace(os.Getenv("ACKWRAP_API_TOKEN")),
	}
	if config.ListenAddr == "" {
		config.ListenAddr = defaultListenAddr
	}

	host, port, err := net.SplitHostPort(config.ListenAddr)
	if err != nil || port == "" {
		return serverConfig{}, fmt.Errorf("ACKWRAP_LISTEN_ADDR 必须是 host:port 格式: %q", config.ListenAddr)
	}
	if host == "localhost" {
		return config, nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return serverConfig{}, fmt.Errorf("ACKWRAP_LISTEN_ADDR 主机必须是 IP 地址或 localhost: %q", host)
	}
	if !ip.IsLoopback() && config.APIToken == "" {
		return serverConfig{}, errors.New("非回环地址监听必须设置 ACKWRAP_API_TOKEN")
	}
	return config, nil
}

func registerWebUI(router *gin.Engine) error {
	dist, err := fs.Sub(webui.Files, "dist")
	if err != nil {
		return fmt.Errorf("open embedded UI: %w", err)
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return fmt.Errorf("read embedded UI index: %w", err)
	}
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "not found"}})
			return
		}
		fileName := strings.TrimPrefix(c.Request.URL.Path, "/")
		if fileName != "" && fs.ValidPath(fileName) {
			if data, readErr := fs.ReadFile(dist, fileName); readErr == nil {
				contentType := mime.TypeByExtension(path.Ext(fileName))
				if contentType == "" {
					contentType = "application/octet-stream"
				}
				if strings.HasPrefix(fileName, "assets/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				}
				c.Data(http.StatusOK, contentType, data)
				return
			}
		}
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
	return nil
}

func main() {
	serverCfg, err := loadServerConfig()
	if err != nil {
		log.Fatalf("server config: %v", err)
	}

	p := paths.Default()
	if err := p.EnsureDirs(); err != nil {
		log.Fatalf("ensure dirs: %v", err)
	}
	logging.Info("main", "data dir: %s", p.DataDir)

	db, err := store.Open(p.DBPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer db.Close()

	realtimeSvc := service.NewRealtimeService()
	coreLogSvc := service.NewCoreLogService()
	singboxSvc := service.NewSingboxService(p, realtimeSvc, coreLogSvc, db)
	runtimeSvc := service.NewRuntimeService(p, db, singboxSvc)
	installerSvc := service.NewInstallerService(db, p, realtimeSvc)
	configSvc := service.NewConfigService(p, db, realtimeSvc)
	if migrated, err := configSvc.MigrateCompatibility(""); err != nil {
		logging.Error("config.migrate", "启动时配置兼容迁移失败: %v", err)
	} else if migrated {
		logging.Info("config.migrate", "启动时配置兼容迁移完成")
	}
	if _, err := configSvc.ListBackups(); err != nil {
		logging.Error("config.backup", "启动时整理配置备份失败: %v", err)
	}
	installerSvc.SetPostInstallHook(func(version string) error {
		_, err := configSvc.MigrateCompatibility(version)
		return err
	})
	settingsSvc := service.NewSettingsService(db)

	// 初始化实验性功能默认配置（如果未设置）
	expSettings, _ := settingsSvc.GetExperimentalSettings()
	if expSettings == nil || expSettings.ClashAPIPort == "" {
		logging.Info("main", "初始化实验性功能默认配置")
		defaultSettings := &model.ExperimentalSettings{
			ClashAPIEnabled:   true,
			ClashAPIPort:      "9090",
			CacheFileEnabled:  true,
			CacheFileStoreDNS: true,
		}
		if err := settingsSvc.SetExperimentalSettings(defaultSettings); err != nil {
			logging.Info("main", "初始化实验性功能默认配置失败: %v", err)
		}
	}
	subscriptionSvc := service.NewSubscriptionService(db, realtimeSvc)
	nodeSvc := service.NewNodeService(db)
	nodeSvc.SetRealtimeService(realtimeSvc)
	routeRuleSvc := service.NewRouteRuleService(db, p, realtimeSvc)
	proxyCollectionSvc := service.NewProxyCollectionService(db, realtimeSvc)
	configGenSvc := service.NewConfigGeneratorService(db, p, singboxSvc)
	settingsSvc.SetModeDependencies(singboxSvc, configGenSvc)
	settingsSvc.SetConnectivitySettingsHook(proxyCollectionSvc.RefreshHealthCheckJobs)
	reconcileSvc := service.NewConfigReconcileService(configGenSvc, realtimeSvc)
	defer reconcileSvc.Close()
	coreRestartSvc := service.NewCoreRestartScheduler(db, singboxSvc, realtimeSvc)
	if err := coreRestartSvc.StartScheduler(); err != nil {
		logging.Error("core.restart_scheduler", "启动核心定时重启调度器失败: %v", err)
	}
	defer coreRestartSvc.StopScheduler()
	dnsSvc := service.NewDNSService(db)
	nodeGroupSvc := service.NewNodeGroupService(db)

	subscriptionSvc.SetConfigReconciler(reconcileSvc)

	subscriptionSvc.StartScheduler()
	routeRuleSvc.StartScheduler()
	proxyCollectionSvc.StartScheduler()
	defer subscriptionSvc.StopScheduler()
	defer routeRuleSvc.StopScheduler()
	defer proxyCollectionSvc.StopScheduler()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(
		gin.LoggerWithWriter(accessTokenRedactingWriter{Writer: gin.DefaultWriter}),
		gin.RecoveryWithWriter(accessTokenRedactingWriter{Writer: gin.DefaultErrorWriter}),
	)

	r.Use(api.SecurityMiddleware(serverCfg.APIToken))
	api.RegisterRoutes(r, runtimeSvc, installerSvc, singboxSvc, configSvc, settingsSvc, subscriptionSvc, nodeSvc, routeRuleSvc, proxyCollectionSvc, configGenSvc, realtimeSvc, coreLogSvc, dnsSvc, nodeGroupSvc, reconcileSvc, coreRestartSvc)
	if err := registerWebUI(r); err != nil {
		log.Fatalf("register embedded UI: %v", err)
	}

	server := &http.Server{
		Addr:              serverCfg.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logging.Info("main", "starting server on %s", serverCfg.ListenAddr)
		serverErrors <- server.ListenAndServe()
	}()

	shutdownSignals := make(chan os.Signal, 1)
	signal.Notify(shutdownSignals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdownSignals)
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
		return
	case shutdownSignal := <-shutdownSignals:
		logging.Info("main", "shutdown signal received: %s", shutdownSignal)
	}
	coreRestartSvc.StopScheduler()
	if singboxSvc.IsRunning() {
		if _, err := singboxSvc.Shutdown(); err != nil {
			logging.Error("main", "stop sing-box during shutdown: %v", err)
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}
}
