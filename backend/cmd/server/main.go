package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/api"
	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/service"
	"github.com/ackwrap/ackwrap/internal/store"
)

func main() {
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
	settingsSvc := service.NewSettingsService(db)

	// 初始化实验性功能默认配置（如果未设置）
	expSettings, _ := settingsSvc.GetExperimentalSettings()
	if expSettings == nil || expSettings.ClashAPIPort == "" {
		logging.Info("main", "初始化实验性功能默认配置")
		defaultSettings := &model.ExperimentalSettings{
			ClashAPIEnabled:  true,
			ClashAPIPort:     "9090",
			CacheFileEnabled: true,
		}
		if err := settingsSvc.SetExperimentalSettings(defaultSettings); err != nil {
			logging.Info("main", "初始化实验性功能默认配置失败: %v", err)
		}
	}

	subscriptionSvc := service.NewSubscriptionService(db, realtimeSvc)
	nodeSvc := service.NewNodeService(db)
	routeRuleSvc := service.NewRouteRuleService(db, p, realtimeSvc)
	proxyCollectionSvc := service.NewProxyCollectionService(db)
	configGenSvc := service.NewConfigGeneratorService(db, p, singboxSvc)
	dnsSvc := service.NewDNSService(db)
	nodeGroupSvc := service.NewNodeGroupService(db)

	// 延迟注入依赖（避免循环依赖）
	subscriptionSvc.SetConfigService(configSvc)
	subscriptionSvc.SetSingBoxService(singboxSvc)

	subscriptionSvc.StartScheduler()
	routeRuleSvc.StartScheduler()
	defer subscriptionSvc.StopScheduler()
	defer routeRuleSvc.StopScheduler()

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.Static("/assets", "./ui/assets")
	api.RegisterRoutes(r, runtimeSvc, installerSvc, singboxSvc, configSvc, settingsSvc, subscriptionSvc, nodeSvc, routeRuleSvc, proxyCollectionSvc, configGenSvc, realtimeSvc, coreLogSvc, dnsSvc, nodeGroupSvc)

	r.NoRoute(func(c *gin.Context) {
		if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
			c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "not found"}})
			return
		}
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.File("./ui/index.html")
	})

	logging.Info("main", "starting server on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
