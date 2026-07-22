package application

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/api"
	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
)

var ErrClosed = errors.New("application is shutting down")

type Options struct {
	Paths    *paths.Paths
	APIToken string
}

// Application owns the shared backend services and their lifecycle.
type Application struct {
	handler                http.Handler
	store                  *store.Store
	realtime               *service.RealtimeService
	singbox                *service.SingboxService
	appUpdate              *service.AppUpdateService
	reconcile              *service.ConfigReconcileService
	coreRestart            *service.CoreRestartScheduler
	subscription           *service.SubscriptionService
	routeRule              *service.RouteRuleService
	proxyCollection        *service.ProxyCollectionService
	stopToolLogEvents      func()
	toolLogEventsCompleted chan struct{}

	lifecycleMu sync.Mutex
	started     bool
	closed      bool
	closeOnce   sync.Once
	closeErr    error
}

func New(options Options) (*Application, error) {
	if options.Paths == nil {
		return nil, errors.New("application paths are required")
	}
	if err := options.Paths.EnsureDirs(); err != nil {
		return nil, fmt.Errorf("ensure dirs: %w", err)
	}
	logging.Info("application.start", "data dir: %s", options.Paths.DataDir)

	db, err := store.Open(options.Paths.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	realtimeSvc := service.NewRealtimeService()
	toolLogEvents, stopToolLogEvents := logging.SubscribeToolLogs(256)
	toolLogEventsCompleted := make(chan struct{})
	go func() {
		defer close(toolLogEventsCompleted)
		for entry := range toolLogEvents {
			realtimeSvc.Broadcast("tool.log", entry)
		}
	}()

	coreLogSvc := service.NewCoreLogService()
	singboxSvc := service.NewSingboxService(options.Paths, realtimeSvc, coreLogSvc, db)
	if err := singboxSvc.RecoverStaleState(); err != nil {
		logging.Error("core.cleanup", "启动时清理 sing-box 网络残留失败: %v", err)
	}
	runtimeSvc := service.NewRuntimeService(options.Paths, db, singboxSvc)
	installerSvc := service.NewInstallerService(db, options.Paths, realtimeSvc)
	configSvc := service.NewConfigService(options.Paths, db, realtimeSvc)
	dnsSvc := service.NewDNSService(db, options.Paths)
	if migrated, migrateErr := configSvc.MigrateCompatibility(""); migrateErr != nil {
		logging.Error("config.migrate", "启动时配置兼容迁移失败: %v", migrateErr)
	} else if migrated {
		logging.Info("config.migrate", "启动时配置兼容迁移完成")
	}
	if _, migrateErr := dnsSvc.MigrateIndependentCache(""); migrateErr != nil {
		logging.Error("dns.global.migrate", "启动时 DNS 缓存配置迁移失败: %v", migrateErr)
	}
	if _, backupErr := configSvc.ListBackups(); backupErr != nil {
		logging.Error("config.backup", "启动时整理配置备份失败: %v", backupErr)
	}
	installerSvc.SetPostInstallHook(func(version string) error {
		_, configErr := configSvc.MigrateCompatibility(version)
		_, dnsErr := dnsSvc.MigrateIndependentCache(version)
		return errors.Join(configErr, dnsErr)
	})

	settingsSvc := service.NewSettingsService(db)
	settingsSvc.SetDashboardsDir(options.Paths.DashboardsDir)
	experimentalSettings, _ := settingsSvc.GetExperimentalSettings()
	if experimentalSettings == nil || experimentalSettings.ClashAPIPort == "" {
		logging.Info("application.start", "初始化实验性功能默认配置")
		defaultSettings := &model.ExperimentalSettings{
			ClashAPIEnabled:   true,
			ClashAPIPort:      "9090",
			CacheFileEnabled:  true,
			CacheFileStoreDNS: true,
		}
		if settingsErr := settingsSvc.SetExperimentalSettings(defaultSettings); settingsErr != nil {
			logging.Info("application.start", "初始化实验性功能默认配置失败: %v", settingsErr)
		}
	}

	subscriptionSvc := service.NewSubscriptionService(db, realtimeSvc)
	nodeSvc := service.NewNodeService(db)
	nodeSvc.SetRealtimeService(realtimeSvc)
	routeRuleSvc := service.NewRouteRuleService(db, options.Paths, realtimeSvc)
	proxyCollectionSvc := service.NewProxyCollectionService(db, realtimeSvc)
	configGenSvc := service.NewConfigGeneratorService(db, options.Paths, singboxSvc)
	settingsSvc.SetModeDependencies(singboxSvc, configGenSvc)
	settingsSvc.SetConnectivitySettingsHook(proxyCollectionSvc.RefreshHealthCheckJobs)
	reconcileSvc := service.NewConfigReconcileService(configGenSvc, realtimeSvc)
	coreRestartSvc := service.NewCoreRestartScheduler(db, singboxSvc, configGenSvc, realtimeSvc)
	appUpdateSvc := service.NewAppUpdateService(db, options.Paths, singboxSvc, realtimeSvc)
	dashboardSvc := service.NewDashboardService(db, options.Paths)
	nodeGroupSvc := service.NewNodeGroupService(db)
	subscriptionSvc.SetConfigReconciler(reconcileSvc)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(
		gin.LoggerWithWriter(accessTokenRedactingWriter{Writer: gin.DefaultWriter}),
		gin.RecoveryWithWriter(accessTokenRedactingWriter{Writer: gin.DefaultErrorWriter}),
	)
	router.Use(api.SecurityMiddleware(options.APIToken))
	api.RegisterRoutes(router, runtimeSvc, installerSvc, singboxSvc, configSvc, settingsSvc, subscriptionSvc, nodeSvc, routeRuleSvc, proxyCollectionSvc, configGenSvc, realtimeSvc, coreLogSvc, dnsSvc, nodeGroupSvc, reconcileSvc, coreRestartSvc, appUpdateSvc, dashboardSvc)
	if err := registerWebUI(router); err != nil {
		reconcileSvc.Close()
		stopToolLogEvents()
		<-toolLogEventsCompleted
		_ = db.Close()
		return nil, err
	}

	return &Application{
		handler:                router,
		store:                  db,
		realtime:               realtimeSvc,
		singbox:                singboxSvc,
		appUpdate:              appUpdateSvc,
		reconcile:              reconcileSvc,
		coreRestart:            coreRestartSvc,
		subscription:           subscriptionSvc,
		routeRule:              routeRuleSvc,
		proxyCollection:        proxyCollectionSvc,
		stopToolLogEvents:      stopToolLogEvents,
		toolLogEventsCompleted: toolLogEventsCompleted,
	}, nil
}

func (app *Application) Handler() http.Handler {
	return app.handler
}

func (app *Application) Start() error {
	app.lifecycleMu.Lock()
	defer app.lifecycleMu.Unlock()
	if app.closed {
		return ErrClosed
	}
	if app.started {
		return nil
	}
	if err := app.coreRestart.StartScheduler(); err != nil {
		logging.Error("core.restart_scheduler", "启动核心定时重启调度器失败: %v", err)
	}
	app.subscription.StartScheduler()
	app.routeRule.StartScheduler()
	app.proxyCollection.StartScheduler()
	app.started = true
	logging.Info("application.start", "backend schedulers started")
	return nil
}

func (app *Application) RestoreCoreAfterUpdate() {
	app.lifecycleMu.Lock()
	defer app.lifecycleMu.Unlock()
	if app.closed {
		return
	}
	app.appUpdate.RestoreCoreAfterUpdate()
}

func (app *Application) StartCoreIfConfigured() error {
	app.lifecycleMu.Lock()
	defer app.lifecycleMu.Unlock()
	if app.closed {
		return ErrClosed
	}
	settings, err := app.store.GetGeneralSettings()
	if err != nil {
		return fmt.Errorf("load general settings: %w", err)
	}
	if !settings.AutoStartCore {
		logging.Info("core.autostart", "核心自动启动已关闭，跳过启动")
		return nil
	}
	return app.singbox.StartIfConfigured()
}

// PrepareShutdown stops background work and the managed core without closing the store.
// The HTTP server can then drain in-flight requests before Close releases shared resources.
func (app *Application) PrepareShutdown() {
	app.lifecycleMu.Lock()
	defer app.lifecycleMu.Unlock()
	if app.closed {
		return
	}
	app.closed = true
	if app.started {
		app.coreRestart.StopScheduler()
		app.proxyCollection.StopScheduler()
		app.routeRule.StopScheduler()
		app.subscription.StopScheduler()
		app.started = false
	}
	if app.singbox.IsRunning() {
		if _, err := app.singbox.Shutdown(); err != nil {
			logging.Error("application.shutdown", "stop sing-box during shutdown: %v", err)
		}
	}
	logging.Info("application.shutdown", "backend background work stopped")
}

func (app *Application) Close() error {
	app.closeOnce.Do(func() {
		app.PrepareShutdown()
		app.reconcile.Close()
		app.stopToolLogEvents()
		<-app.toolLogEventsCompleted
		app.closeErr = app.store.Close()
		if app.closeErr != nil {
			logging.Error("application.shutdown", "close store failed: %v", app.closeErr)
		}
	})
	return app.closeErr
}
