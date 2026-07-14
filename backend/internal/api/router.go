package api

import (
	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/handler"
	"github.com/ackwrap/ackwrap/internal/service"
)

func RegisterRoutes(
	r *gin.Engine,
	runtimeSvc *service.RuntimeService,
	installerSvc *service.InstallerService,
	singboxSvc *service.SingboxService,
	configSvc *service.ConfigService,
	settingsSvc *service.SettingsService,
	subscriptionSvc *service.SubscriptionService,
	nodeSvc *service.NodeService,
	routeRuleSvc *service.RouteRuleService,
	proxyCollectionSvc *service.ProxyCollectionService,
	configGenSvc *service.ConfigGeneratorService,
	realtimeSvc *service.RealtimeService,
	coreLogSvc *service.CoreLogService,
	dnsSvc *service.DNSService,
	nodeGroupSvc *service.NodeGroupService,
	reconcileSvc *service.ConfigReconcileService,
) {
	runtimeH := handler.NewRuntimeHandler(runtimeSvc)
	installerH := handler.NewInstallerHandler(installerSvc)
	coreH := handler.NewCoreHandler(singboxSvc)
	configH := handler.NewConfigHandler(configSvc)
	settingsH := handler.NewSettingsHandler(settingsSvc)
	subscriptionH := handler.NewSubscriptionHandler(subscriptionSvc)
	nodeH := handler.NewNodeHandler(nodeSvc)
	routeRuleH := handler.NewRouteRuleHandler(routeRuleSvc)
	proxyCollectionH := handler.NewProxyCollectionHandler(proxyCollectionSvc)
	configGenH := handler.NewConfigGeneratorHandler(configGenSvc)
	realtimeH := handler.NewRealtimeHandler(realtimeSvc, runtimeSvc, installerSvc, configSvc, singboxSvc)
	logH := handler.NewLogHandler(coreLogSvc)
	dnsH := handler.NewDNSHandler(dnsSvc)
	nodeGroupH := handler.NewNodeGroupHandler(nodeGroupSvc)

	clashProxyH := handler.NewClashProxyHandler(settingsSvc)

	v1 := r.Group("/api/v1")
	v1.Use(configMutationMiddleware(reconcileSvc))
	{
		v1.GET("/runtime", runtimeH.GetStatus)

		v1.GET("/installer/sing-box", installerH.GetStatus)
		v1.POST("/installer/sing-box/install", installerH.Install)

		v1.GET("/config/status", configH.GetStatus)
		v1.GET("/config/files", configH.ListFiles)
		v1.POST("/config/default", configH.GenerateDefault)
		v1.POST("/config/validate", configH.Validate)
		v1.POST("/config/rules/update", configH.UpdateRules)
		v1.POST("/config/backup", configH.Backup)
		v1.POST("/config/restore", configH.Restore)

		v1.POST("/core/start", coreH.Start)
		v1.POST("/core/stop", coreH.Stop)
		v1.POST("/core/restart", coreH.Restart)
		v1.POST("/core/reload-config", coreH.ReloadConfig)
		v1.POST("/core/close-connections", coreH.CloseConnections)
		v1.POST("/core/flush-core-dns", coreH.FlushCoreDNS)
		v1.POST("/core/flush-fakeip", coreH.FlushFakeIP)
		v1.POST("/core/network-check", coreH.NetworkCheck)
		v1.GET("/core/diagnostics", coreH.Diagnostics)
		v1.POST("/core/reset-firewall", coreH.ResetFirewall)
		v1.POST("/core/flush-dns", coreH.FlushDNS)
		v1.POST("/core/check-update", coreH.CheckUpdate)
		v1.GET("/logs/core", logH.ListCore)
		v1.DELETE("/logs/core", logH.ClearCore)

		v1.GET("/settings/update", settingsH.GetUpdateSettings)
		v1.PUT("/settings/update", settingsH.SetUpdateSettings)
		v1.GET("/settings/log", settingsH.GetLogSettings)
		v1.PUT("/settings/log", settingsH.SetLogSettings)
		v1.GET("/settings/ntp", settingsH.GetNTPSettings)
		v1.PUT("/settings/ntp", settingsH.SetNTPSettings)
		v1.GET("/settings/dns", settingsH.GetDNSSettings)
		v1.PUT("/settings/dns", settingsH.SetDNSSettings)
		v1.GET("/settings/inbound-mode", settingsH.GetInboundMode)
		v1.PUT("/settings/inbound-mode", settingsH.SetInboundMode)
		v1.GET("/settings/proxy-mode", settingsH.GetProxyMode)
		v1.PUT("/settings/proxy-mode", settingsH.SetProxyMode)
		v1.GET("/settings/experimental", settingsH.GetExperimentalSettings)
		v1.PUT("/settings/experimental", settingsH.SetExperimentalSettings)
		v1.GET("/settings/node-filters", settingsH.ListNodeFilters)
		v1.POST("/settings/node-filters", settingsH.CreateNodeFilter)
		v1.PUT("/settings/node-filters/:id", settingsH.UpdateNodeFilter)
		v1.DELETE("/settings/node-filters/:id", settingsH.DeleteNodeFilter)

		v1.GET("/subscriptions", subscriptionH.List)
		v1.GET("/subscriptions/user-agents", subscriptionH.UserAgentOptions)
		v1.POST("/subscriptions", subscriptionH.Create)
		v1.POST("/subscriptions/sync", subscriptionH.SyncAll)
		v1.PUT("/subscriptions/:id", subscriptionH.Update)
		v1.DELETE("/subscriptions/:id", subscriptionH.Delete)
		v1.POST("/subscriptions/:id/sync", subscriptionH.Sync)

		v1.GET("/nodes/facets", nodeH.Facets)
		v1.GET("/nodes", nodeH.List)
		v1.POST("/nodes/import/preview", nodeH.ImportPreview)
		v1.POST("/nodes/import", nodeH.Import)
		v1.POST("/nodes/tcping", nodeH.TCPing)
		v1.POST("/nodes/add-emoji", nodeH.AddEmoji)
		v1.POST("/nodes/flag", nodeH.InferFlag)
		v1.POST("/nodes/flags", nodeH.InferFlags)
		v1.POST("/nodes/batch-rename", nodeH.BatchRename)
		v1.POST("/nodes/batch-delete", nodeH.BatchDelete)
		v1.PUT("/nodes/:uid/enabled", nodeH.SetEnabled)
		v1.PUT("/nodes/:uid/preferred", nodeH.SetPreferred)

		v1.GET("/collections", proxyCollectionH.List)
		v1.POST("/collections", proxyCollectionH.Create)
		v1.GET("/collections/:id", proxyCollectionH.Get)
		v1.PUT("/collections/:id", proxyCollectionH.Update)
		v1.DELETE("/collections/:id", proxyCollectionH.Delete)
		v1.PUT("/collections/:id/enabled", proxyCollectionH.ToggleEnabled)
		v1.POST("/collections/:id/test", proxyCollectionH.Test)

		v1.GET("/config/generate", configGenH.GetGenerateRequest)
		v1.POST("/config/generate", configGenH.Generate)
		v1.GET("/config/preview", configGenH.Preview)
		v1.POST("/config/apply", configGenH.Apply)

		v1.GET("/rules", routeRuleH.List)
		v1.POST("/rules", routeRuleH.Create)
		v1.GET("/rules/subscriptions", routeRuleH.ListSubscriptions)
		v1.POST("/rules/subscriptions", routeRuleH.CreateSubscription)
		v1.POST("/rules/subscriptions/sync", routeRuleH.SyncAllSubscriptions)
		v1.GET("/rules/subscriptions/:id/content", routeRuleH.SubscriptionContent)
		v1.POST("/rules/subscriptions/:id/sync", routeRuleH.SyncSubscription)
		v1.PUT("/rules/subscriptions/:id", routeRuleH.UpdateSubscription)
		v1.DELETE("/rules/subscriptions/:id", routeRuleH.DeleteSubscription)
		v1.GET("/rules/geo", routeRuleH.ListGeoAssets)
		v1.GET("/rules/geo/tags", routeRuleH.GeoTags)
		v1.GET("/rules/geo/domains", routeRuleH.GeoDomains)
		v1.GET("/rules/geo/lookup", routeRuleH.GeoLookup)
		v1.GET("/rules/geo/rule-sets/:tag/content", routeRuleH.GeneratedGeoRuleSetContent)
		v1.POST("/rules/geo/sync", routeRuleH.SyncAllGeoAssets)
		v1.PUT("/rules/geo/:id", routeRuleH.UpdateGeoAsset)
		v1.POST("/rules/geo/:id/sync", routeRuleH.SyncGeoAsset)
		v1.POST("/rules/reorder", routeRuleH.Reorder)
		v1.GET("/rules/preview", routeRuleH.Preview)
		v1.PUT("/rules/:id", routeRuleH.Update)
		v1.DELETE("/rules/:id", routeRuleH.Delete)

		v1.GET("/dns/servers", dnsH.ListDNSServers)
		v1.POST("/dns/servers", dnsH.CreateDNSServer)
		v1.GET("/dns/servers/:id", dnsH.GetDNSServer)
		v1.PUT("/dns/servers/:id", dnsH.UpdateDNSServer)
		v1.DELETE("/dns/servers/:id", dnsH.DeleteDNSServer)

		v1.GET("/dns/rules", dnsH.ListDNSRules)
		v1.POST("/dns/rules", dnsH.CreateDNSRule)
		v1.POST("/dns/rules/reorder", dnsH.ReorderDNSRules)
		v1.GET("/dns/rules/:id", dnsH.GetDNSRule)
		v1.PUT("/dns/rules/:id", dnsH.UpdateDNSRule)
		v1.DELETE("/dns/rules/:id", dnsH.DeleteDNSRule)

		v1.GET("/dns/global", dnsH.GetDNSGlobalSettings)
		v1.PUT("/dns/global", dnsH.SetDNSGlobalSettings)

		v1.GET("/node-groups", nodeGroupH.List)
		v1.POST("/node-groups", nodeGroupH.Create)
		v1.POST("/node-groups/reorder", nodeGroupH.Reorder)
		v1.POST("/node-groups/batch-delete", nodeGroupH.BatchDelete)
		v1.GET("/node-groups/preview", nodeGroupH.PreviewMatches)
		v1.POST("/node-groups/quick-setup", nodeGroupH.QuickSetup)
		v1.GET("/node-groups/:id", nodeGroupH.Get)
		v1.PUT("/node-groups/:id", nodeGroupH.Update)
		v1.DELETE("/node-groups/:id", nodeGroupH.Delete)

		v1.GET("/realtime/ws", realtimeH.HandleWS)

		// Clash API 代理路由（所有请求通过后端代理）
		v1.GET("/clash-status", clashProxyH.GetClashStatus)
		v1.Any("/clash/*path", clashProxyH.Proxy)
	}
}
